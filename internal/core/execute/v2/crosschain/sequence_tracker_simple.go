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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// SimpleSequenceGap represents a gap in sequence numbers
type SimpleSequenceGap struct {
	Start            uint64    // First missing sequence
	End              uint64    // Last missing sequence (inclusive)
	DetectedAt       time.Time // When gap was detected
	RecoveryAttempts int       // Number of recovery attempts
}

// SimplePartitionState tracks sequence state for a single source partition
type SimplePartitionState struct {
	// Synthetic transaction tracking
	LastSyntheticDelivered uint64
	SyntheticGaps          map[uint64]*SimpleSequenceGap // Key is start of gap

	// Anchor tracking
	LastAnchorDelivered uint64
	AnchorGaps          map[uint64]*SimpleSequenceGap

	// Statistics
	TotalGapsDetected  int64
	TotalGapsRecovered int64
	TotalDuplicates    int64
	TotalDropped       int64 // Out-of-order messages we dropped

	mu sync.RWMutex
}

// SimpleSequenceTracker manages sequence tracking without buffering
type SimpleSequenceTracker struct {
	states    map[string]*SimplePartitionState // Key is partition URL string
	conductor *CrossChainConductor
	logger    logging.OptionalLogger
	mu        sync.RWMutex

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
	if msg.Number == 0 {
		return false, "missing sequence number", false
	}

	source := msg.Source.String()
	state := st.getOrCreateState(source)

	state.mu.Lock()
	defer state.mu.Unlock()

	expectedNext := state.LastSyntheticDelivered + 1

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

			st.logger.Info("Sequence gap detected in synthetic transactions",
				"source", source,
				"gap_start", gapStart,
				"gap_end", gapEnd,
				"gap_size", gapSize,
				"received", msg.Number,
				"dropped", true)

			// Send recovery request immediately
			go st.SendRecoveryRequest(source, "synthetic", state.LastSyntheticDelivered)
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
			if seq == state.LastSyntheticDelivered+1 {
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
			if seq == state.LastAnchorDelivered+1 {
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
	if st.conductor.recoveryManager == nil {
		// If no recovery manager, try batch proof manager
		if st.conductor.batchProofManager != nil {
			// For batch proof recovery, we need to convert to the appropriate format
			missingSequences := make([]uint64, 0, gapEnd-gapStart+1)
			for seq := gapStart; seq <= gapEnd; seq++ {
				missingSequences = append(missingSequences, seq)
			}
			sourceURL, _ := url.Parse(source)
			return st.conductor.RequestMissingTransactionsWithBatchProof(source, msgType, missingSequences, sourceURL)
		}
		return errors.InternalError.With("no recovery mechanism available")
	}

	// Create recovery request for the gap
	req := &RecoveryRequest{
		Type:        msgType,
		Source:      source,
		Destination: st.conductor.Describe.PartitionUrl().String(),
		FromNumber:  gapStart,
		ToNumber:    gapEnd,
		Requester:   st.conductor.Describe.PartitionUrl().String(),
		RequestedAt: time.Now(),
	}

	// Submit recovery request immediately
	_, err := st.conductor.recoveryManager.RequestMissingTransactions(req)
	if err != nil {
		return errors.UnknownError.WithFormat("failed to request missing messages: %w", err)
	}

	st.logger.Info("Requested missing messages immediately",
		"source", source,
		"type", msgType,
		"gap", fmt.Sprintf("[%d-%d]", gapStart, gapEnd),
		"size", gapEnd-gapStart+1)

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
			"synthetic_delivered":  state.LastSyntheticDelivered,
			"synthetic_gaps":       len(state.SyntheticGaps),
			"anchor_delivered":     state.LastAnchorDelivered,
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

// SendRecoveryRequest sends a recovery request for missing messages
func (st *SimpleSequenceTracker) SendRecoveryRequest(source string, messageType string, lastKnownSeq uint64) {
	st.logger.Info("Sending recovery request",
		"to", source,
		"type", messageType,
		"last_seq", lastKnownSeq)

	// Create recovery request
	req := &messaging.RecoveryRequest{
		SourcePartition:      source,
		DestinationPartition: st.conductor.Describe.PartitionId,
		MessageType:          messageType,
		LastKnownSequence:    lastKnownSeq,
	}

	// Send via dispatcher
	envelope := &messaging.Envelope{
		Messages: []messaging.Message{req},
	}

	sourceURL := protocol.PartitionUrl(source)
	err := st.conductor.dispatcher.Submit(context.Background(), sourceURL, envelope)
	if err != nil {
		st.logger.Error("Failed to send recovery request",
			"to", source,
			"error", err)
	}
}
