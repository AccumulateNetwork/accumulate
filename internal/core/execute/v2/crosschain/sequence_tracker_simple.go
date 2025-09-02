// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// SimpleSequenceGap represents a gap in sequence numbers
type SimpleSequenceGap struct {
	Start       uint64    // First missing sequence
	End         uint64    // Last missing sequence (inclusive)
	DetectedAt  time.Time // When gap was detected
	RecoveryAttempts int  // Number of recovery attempts
}

// SimplePartitionState tracks sequence state for a single source partition
// 
// IMPORTANT: Anchors and synthetic transactions are tracked separately because:
// - Synthetic transactions: Once processed, they can be pruned/discarded (temporary data)
// - Anchor transactions: Required for cryptographic proofs, must be kept permanently
// 
// This separation is critical for proper data lifecycle management and storage efficiency.
type SimplePartitionState struct {
	// Synthetic transaction tracking (can be pruned after processing)
	LastSyntheticDelivered uint64
	SyntheticGaps         map[uint64]*SimpleSequenceGap // Key is start of gap
	
	// Anchor tracking (permanent, required for cryptographic proofs)
	LastAnchorDelivered uint64
	AnchorGaps         map[uint64]*SimpleSequenceGap
	
	// Statistics
	TotalGapsDetected   int64
	TotalGapsRecovered  int64
	TotalDuplicates     int64
	TotalDropped        int64 // Out-of-order messages we dropped
	
	mu sync.RWMutex
}

// SimpleSequenceTracker manages sequence tracking without buffering
type SimpleSequenceTracker struct {
	states     map[string]*SimplePartitionState // Key is partition URL string
	conductor  *CrossChainConductor
	logger     logging.OptionalLogger
	mu         sync.RWMutex
	
	// Configuration
	maxGapSize int // Maximum gap size to track (larger = malicious)
}

// NewSimpleSequenceTracker creates a new simplified sequence tracker
func NewSimpleSequenceTracker(conductor *CrossChainConductor, logger logging.OptionalLogger) *SimpleSequenceTracker {
	return &SimpleSequenceTracker{
		states:     make(map[string]*SimplePartitionState),
		conductor:  conductor,
		logger:     logger.With("module", "sequence-tracker").(logging.OptionalLogger),
		maxGapSize: 100, // Gaps larger than 100 are considered attacks
	}
}

// ValidateAndTrackSynthetic validates a synthetic transaction's sequence
// Returns: valid, reason, should_request_recovery
func (st *SimpleSequenceTracker) ValidateAndTrackSynthetic(msg *messaging.SequencedMessage) (valid bool, reason string, requestRecovery bool) {
	st.logger.Debug("[HEALING-DEBUG] ValidateAndTrackSynthetic called",
		"source", msg.Source, "sequence", msg.Number)
	if msg.Number == 0 {
		return false, "missing sequence number", false
	}
	
	source := msg.Source.String()
	state := st.getOrCreateState(source)
	
	state.mu.Lock()
	defer state.mu.Unlock()
	
	expectedNext := state.LastSyntheticDelivered + 1
	st.logger.Debug("[HEALING-DEBUG] Synthetic sequence check",
		"source", source,
		"received_seq", msg.Number,
		"last_delivered", state.LastSyntheticDelivered,
		"expected_next", expectedNext,
		"total_gaps", len(state.SyntheticGaps))
	
	// Check for duplicate or old message
	if msg.Number <= state.LastSyntheticDelivered {
		state.TotalDuplicates++
		// With collection proofs, we might receive already-processed messages
		// Just skip them silently
		return false, fmt.Sprintf("already processed (seq %d, last delivered %d)", 
			msg.Number, state.LastSyntheticDelivered), false
	}
	
	// Perfect sequence - accept it
	if msg.Number == expectedNext {
		state.LastSyntheticDelivered = msg.Number
		st.logger.Debug("[HEALING-DEBUG] Perfect synthetic sequence - accepting",
			"source", source, "sequence", msg.Number)
		
		// Check if this closes any gaps
		st.checkGapClosure(state.SyntheticGaps, msg.Number)
		
		return true, "", false
	}
	
	// Gap detected - drop the message and request recovery
	if msg.Number > expectedNext {
		gapStart := expectedNext
		gapEnd := msg.Number - 1
		gapSize := gapEnd - gapStart + 1
		
		// Check for malicious gap size
		if gapSize > uint64(st.maxGapSize) {
			return false, fmt.Sprintf("gap too large (%d messages), possible attack", gapSize), false
		}
		
		// Track the gap if new
		shouldRequest := false
		if _, exists := state.SyntheticGaps[gapStart]; !exists {
			gap := &SimpleSequenceGap{
				Start:      gapStart,
				End:        gapEnd,
				DetectedAt: time.Now(),
			}
			state.SyntheticGaps[gapStart] = gap
			state.TotalGapsDetected++
			shouldRequest = true
			
			st.logger.Error("[HEALING-DEBUG] NEW synthetic gap detected - SENDING RECOVERY REQUEST",
				"source", source,
				"gap_start", gapStart,
				"gap_end", gapEnd,
				"gap_size", gapSize,
				"received", msg.Number,
				"last_delivered", state.LastSyntheticDelivered,
				"total_gaps_now", len(state.SyntheticGaps)+1)
			
			// Send recovery request immediately
			go st.SendRecoveryRequest(source, "synthetic", state.LastSyntheticDelivered)
		} else {
			st.logger.Debug("[HEALING-DEBUG] Gap already known, not sending duplicate recovery request",
				"source", source, "gap_start", gapStart)
		}
		
		// Drop the out-of-order message
		state.TotalDropped++
		return false, fmt.Sprintf("out of order, gap detected [%d-%d], dropping message %d", 
			gapStart, gapEnd, msg.Number), shouldRequest
	}
	
	// Should not reach here
	return false, "unexpected sequence validation state", false
}

// ValidateAndTrackAnchor validates an anchor's sequence
func (st *SimpleSequenceTracker) ValidateAndTrackAnchor(msg *messaging.BlockAnchor, source *url.URL, sequence uint64) (valid bool, reason string, requestRecovery bool) {
	if sequence == 0 {
		return false, "missing anchor sequence number", false
	}
	
	sourceStr := source.String()
	state := st.getOrCreateState(sourceStr)
	
	state.mu.Lock()
	defer state.mu.Unlock()
	
	expectedNext := state.LastAnchorDelivered + 1
	
	// Check for duplicate or old anchor
	if sequence <= state.LastAnchorDelivered {
		state.TotalDuplicates++
		// Skip already processed anchors
		return false, fmt.Sprintf("anchor already processed (seq %d, last delivered %d)", 
			sequence, state.LastAnchorDelivered), false
	}
	
	// Perfect sequence - accept it
	if sequence == expectedNext {
		state.LastAnchorDelivered = sequence
		
		// Check if this closes any gaps
		st.checkGapClosure(state.AnchorGaps, sequence)
		
		return true, "", false
	}
	
	// Gap detected - drop and request recovery
	if sequence > expectedNext {
		gapStart := expectedNext
		gapEnd := sequence - 1
		gapSize := gapEnd - gapStart + 1
		
		// Check for malicious gap size
		if gapSize > uint64(st.maxGapSize) {
			return false, fmt.Sprintf("anchor gap too large (%d messages), possible attack", gapSize), false
		}
		
		// Track the gap if new
		shouldRequest := false
		if _, exists := state.AnchorGaps[gapStart]; !exists {
			gap := &SimpleSequenceGap{
				Start:      gapStart,
				End:        gapEnd,
				DetectedAt: time.Now(),
			}
			state.AnchorGaps[gapStart] = gap
			state.TotalGapsDetected++
			shouldRequest = true
			
			st.logger.Info("Sequence gap detected in anchors",
				"source", sourceStr,
				"gap_start", gapStart,
				"gap_end", gapEnd,
				"gap_size", gapSize,
				"received", sequence,
				"dropped", true)
		}
		
		// Drop the out-of-order anchor
		state.TotalDropped++
		return false, fmt.Sprintf("anchor out of order, gap detected [%d-%d], dropping anchor %d", 
			gapStart, gapEnd, sequence), shouldRequest
	}
	
	return false, "unexpected anchor sequence validation state", false
}

// ProcessCollectionProofMessages handles messages from a collection proof
// Collection proofs might include extras we don't need - just skip them
func (st *SimpleSequenceTracker) ProcessCollectionProofMessages(source string, sequences []uint64, msgType MessageType) {
	state := st.getOrCreateState(source)
	
	state.mu.Lock()
	defer state.mu.Unlock()
	
	if msgType == MessageTypeSynthetic {
		for _, seq := range sequences {
			// Skip if already processed
			if seq <= state.LastSyntheticDelivered {
				continue
			}
			
			// Only process if it's the next expected
			if seq == state.LastSyntheticDelivered + 1 {
				state.LastSyntheticDelivered = seq
				st.checkGapClosure(state.SyntheticGaps, seq)
			}
		}
	} else if msgType == MessageTypeAnchor {
		for _, seq := range sequences {
			// Skip if already processed
			if seq <= state.LastAnchorDelivered {
				continue
			}
			
			// Only process if it's the next expected
			if seq == state.LastAnchorDelivered + 1 {
				state.LastAnchorDelivered = seq
				st.checkGapClosure(state.AnchorGaps, seq)
			}
		}
	}
}

// checkGapClosure checks if delivering a sequence closes any gaps
func (st *SimpleSequenceTracker) checkGapClosure(gaps map[uint64]*SimpleSequenceGap, delivered uint64) {
	for gapStart, gap := range gaps {
		// If we've delivered past the end of this gap, it's closed
		if delivered >= gap.End {
			delete(gaps, gapStart)
			st.logger.Info("Gap recovered",
				"gap_start", gap.Start,
				"gap_end", gap.End,
				"duration", time.Since(gap.DetectedAt))
		}
	}
}

// RequestMissingMessages immediately triggers recovery for detected gaps
func (st *SimpleSequenceTracker) RequestMissingMessages(ctx context.Context, source string, msgType MessageType, gapStart, gapEnd uint64) error {
	// Use the conductor's recovery manager immediately - no waiting
	// Simple gap recovery: ask the source partition to resend from gapStart
	req := &RecoveryRequest{
		Requester:   st.conductor.Describe().PartitionUrl().String(),
		FromNumber:  gapStart,
	}
	
	// For now, just log the gap - in a real system, this would send the request to the source
	// The source partition would call HandleRecoveryRequest when it receives this
	err := st.conductor.HandleRecoveryRequest(req)
	if err != nil {
		return errors.UnknownError.WithFormat("failed to handle gap recovery: %w", err)
	}
	
	st.logger.Info("Requested missing messages immediately",
		"source", source,
		"type", msgType,
		"gap", fmt.Sprintf("[%d-%d]", gapStart, gapEnd),
		"size", gapEnd - gapStart + 1)
	
	return nil
}

// getOrCreateState gets or creates partition state
func (st *SimpleSequenceTracker) getOrCreateState(partition string) *SimplePartitionState {
	st.mu.RLock()
	state, exists := st.states[partition]
	st.mu.RUnlock()
	
	if exists {
		return state
	}
	
	st.mu.Lock()
	defer st.mu.Unlock()
	
	// Double-check after acquiring write lock
	state, exists = st.states[partition]
	if exists {
		return state
	}
	
	// Create new state
	state = &SimplePartitionState{
		SyntheticGaps: make(map[uint64]*SimpleSequenceGap),
		AnchorGaps:    make(map[uint64]*SimpleSequenceGap),
	}
	st.states[partition] = state
	
	return state
}

// GetStatistics returns sequence tracking statistics
func (st *SimpleSequenceTracker) GetStatistics() map[string]interface{} {
	st.mu.RLock()
	defer st.mu.RUnlock()
	
	stats := make(map[string]interface{})
	
	for partition, state := range st.states {
		state.mu.RLock()
		partStats := map[string]interface{}{
			"synthetic_delivered":   state.LastSyntheticDelivered,
			"synthetic_gaps":        len(state.SyntheticGaps),
			"anchor_delivered":      state.LastAnchorDelivered,
			"anchor_gaps":          len(state.AnchorGaps),
			"total_gaps_detected":  state.TotalGapsDetected,
			"total_gaps_recovered": state.TotalGapsRecovered,
			"total_duplicates":     state.TotalDuplicates,
			"total_dropped":        state.TotalDropped,
		}
		state.mu.RUnlock()
		
		stats[partition] = partStats
	}
	
	return stats
}

// convertStringToMessageType converts string message type to MessageType enum
func convertStringToMessageType(messageType string) MessageType {
	switch messageType {
	case "synthetic":
		return MessageTypeSynthetic
	case "anchor":
		return MessageTypeAnchor
	default:
		return MessageTypeOther
	}
}

// SendRecoveryRequest sends a recovery request for missing messages
func (st *SimpleSequenceTracker) SendRecoveryRequest(source string, messageType string, lastKnownSeq uint64) {
	st.logger.Error("[HEALING-DEBUG] SendRecoveryRequest called - INITIATING RECOVERY",
		"to", source,
		"type", messageType,
		"last_seq", lastKnownSeq,
		"recovery_from_seq", lastKnownSeq+1)

	// Simple gap recovery: ask the source partition to resend from lastKnownSeq+1  
	req := &RecoveryRequest{
		Requester:   st.conductor.Describe().PartitionId,
		FromNumber:  lastKnownSeq + 1,
	}

	// For now, just log the gap healing request - in a real system, this would send to source
	err := st.conductor.HandleRecoveryRequest(req)
	if err != nil {
		st.logger.Error("[HEALING-DEBUG] Failed to handle gap healing request - RECOVERY FAILED",
			"to", source,
			"error", err,
			"recovery_from_seq", lastKnownSeq + 1)
	} else {
		st.logger.Error("[HEALING-DEBUG] Processed gap healing request - RECOVERY INITIATED",
			"to", source,
			"recovery_from_seq", lastKnownSeq + 1)
	}
}