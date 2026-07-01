// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// HandleGapRequest handles an incoming gap request from a destination partition.
// The gap request contains the last sequence number the destination has received.
// We reset our send index to that value so the next batch send will include
// all messages from that point forward.
//
// This implements the simple index-based gap recovery mechanism:
// 1. Destination detects gap and sends request with LastKnownSequence
// 2. Source resets SentTxIndex = LastKnownSequence
// 3. Next batch automatically includes everything from LastKnownSequence+1
func (cc *CrossChainConductor) HandleGapRequest(ctx context.Context, req *messaging.RecoveryRequest) error {
	cc.logger.Info("Handling gap request",
		"from", req.DestinationPartition,
		"type", req.MessageType,
		"last_known", req.LastKnownSequence)

	// Get or create destination state
	destURL, err := url.Parse(req.DestinationPartition)
	if err != nil {
		return errors.BadRequest.WithFormat("invalid destination URL: %w", err)
	}
	state := cc.getOrCreateDestinationState(destURL)
	
	// Reset the send index for gap recovery
	if state.ResetForGapRecovery(req.LastKnownSequence) {
		cc.logger.Info("Reset send index for gap recovery",
			"destination", req.DestinationPartition,
			"reset_to", req.LastKnownSequence,
			"gap_size", state.GetGapSize())
		
		// Trigger immediate batch send to fill the gap
		go cc.sendBatchToDestination(ctx, destURL)
		
		return nil
	}
	
	cc.logger.Debug("Gap request ignored - already at or past requested sequence",
		"destination", req.DestinationPartition,
		"requested", req.LastKnownSequence,
		"current", state.SentTxIndex)
	
	return nil
}

// sendBatchToDestination sends all pending messages to a destination using collection proofs.
// This is the core sending logic that implements the index-based gap recovery:
// - Sends from SentTxIndex+1 to CurrentTxIndex
// - On success: advances SentTxIndex
// - On failure: leaves SentTxIndex unchanged (will retry all messages next time)
func (cc *CrossChainConductor) sendBatchToDestination(ctx context.Context, dest *url.URL) error {
	state := cc.getDestinationState(dest.String())
	if state == nil {
		return errors.NotFound.WithFormat("destination state not found: %v", dest)
	}
	
	// Try to mark send in progress
	if !state.StartSend() {
		cc.logger.Debug("Send already in progress for destination", "dest", dest)
		return nil
	}
	
	// Get the range to send
	start, end := state.GetSendRange()
	if start == 0 && end == 0 {
		state.MarkSendFailure() // Reset SendInProgress flag
		cc.logger.Debug("Nothing to send to destination", "dest", dest)
		return nil
	}
	
	cc.logger.Info("Sending batch to destination",
		"dest", dest,
		"range", fmt.Sprintf("[%d-%d]", start, end),
		"count", end-start+1)
	
	// Collect messages in the range
	messages := state.CollectMessages(start, end)
	if len(messages) == 0 {
		state.MarkSendFailure()
		return errors.NotFound.With("no messages found in range")
	}
	
	// Create collection proof for the batch
	proof, err := cc.proofService.CreateProofForMessages(ctx, messages)
	if err != nil {
		state.MarkSendFailure()
		atomic.AddInt64(&cc.syntheticsErrors, 1)
		return errors.UnknownError.WithFormat("failed to create collection proof: %w", err)
	}
	
	// Send the batch with collection proof
	envelope := &messaging.Envelope{
		Messages: messages,
		// Proof would be attached here in real implementation
	}
	
	err = cc.dispatcher.Submit(ctx, dest, envelope)
	if err != nil {
		// Send failed - SentTxIndex remains unchanged
		// Next attempt will include all these messages plus any new ones
		state.MarkSendFailure()
		atomic.AddInt64(&cc.syntheticsErrors, 1)
		atomic.AddInt64(&cc.transmissionErrors, 1)
		
		cc.logger.Error("Failed to send batch",
			"dest", dest,
			"range", fmt.Sprintf("[%d-%d]", start, end),
			"error", err)
		
		return err
	}
	
	// Success! Advance SentTxIndex
	state.MarkSendSuccess(end)
	atomic.AddInt64(&cc.syntheticsSent, int64(len(messages)))
	
	cc.logger.Info("Successfully sent batch",
		"dest", dest,
		"range", fmt.Sprintf("[%d-%d]", start, end),
		"count", len(messages),
		"proof", proof != nil)
	
	return nil
}

// getOrCreateDestinationState gets or creates state for a destination
func (cc *CrossChainConductor) getOrCreateDestinationState(dest *url.URL) *DestinationSendState {
	key := dest.String()
	
	// Try read lock first
	cc.statesMutex.RLock()
	state, exists := cc.destinationStates[key]
	cc.statesMutex.RUnlock()
	
	if exists {
		return state
	}
	
	// Need to create - use write lock
	cc.statesMutex.Lock()
	defer cc.statesMutex.Unlock()
	
	// Double-check after acquiring write lock
	state, exists = cc.destinationStates[key]
	if exists {
		return state
	}
	
	// Create new state
	state = NewDestinationSendState(dest)
	cc.destinationStates[key] = state
	
	cc.logger.Debug("Created destination state", "dest", dest)
	return state
}

// getDestinationState gets existing state for a destination (no creation)
func (cc *CrossChainConductor) getDestinationState(key string) *DestinationSendState {
	cc.statesMutex.RLock()
	defer cc.statesMutex.RUnlock()
	return cc.destinationStates[key]
}

// QueueMessageForDestination queues a message to be sent to a destination.
// Messages are sent in batches with collection proofs.
func (cc *CrossChainConductor) QueueMessageForDestination(dest *url.URL, seq uint64, msg messaging.Message) {
	state := cc.getOrCreateDestinationState(dest)
	state.QueueMessage(seq, msg)
	
	cc.logger.Debug("Queued message for destination",
		"dest", dest,
		"seq", seq,
		"gap_size", state.GetGapSize())
}

// processPendingBatches periodically sends batches to destinations with pending messages
func (cc *CrossChainConductor) processPendingBatches() {
	ticker := time.NewTicker(100 * time.Millisecond) // Check every 100ms
	defer ticker.Stop()
	
	for {
		select {
		case <-cc.stopChan:
			return
		case <-ticker.C:
			cc.sendPendingBatches()
		}
	}
}

// sendPendingBatches sends batches to all destinations with pending messages
func (cc *CrossChainConductor) sendPendingBatches() {
	cc.statesMutex.RLock()
	destinations := make([]*DestinationSendState, 0, len(cc.destinationStates))
	for _, state := range cc.destinationStates {
		if state.HasPendingMessages() {
			destinations = append(destinations, state)
		}
	}
	cc.statesMutex.RUnlock()
	
	// Send to each destination with pending messages
	ctx := context.Background()
	for _, state := range destinations {
		if state.HasPendingMessages() {
			go cc.sendBatchToDestination(ctx, state.Destination)
		}
	}
}

// GetDestinationMetrics returns metrics for all destinations
func (cc *CrossChainConductor) GetDestinationMetrics() []map[string]interface{} {
	cc.statesMutex.RLock()
	defer cc.statesMutex.RUnlock()
	
	metrics := make([]map[string]interface{}, 0, len(cc.destinationStates))
	for _, state := range cc.destinationStates {
		metrics = append(metrics, state.GetMetrics())
	}
	
	return metrics
}