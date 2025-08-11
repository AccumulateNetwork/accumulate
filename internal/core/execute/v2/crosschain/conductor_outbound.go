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

// SubmitSynthetic submits synthetic transactions for async processing
func (cc *CrossChainConductor) SubmitSynthetic(ctx context.Context, messages []messaging.Message, destination *url.URL) error {
	responseChan := make(chan error, 1)
	req := &SyntheticRequest{
		Messages:     messages,
		Destination:  destination,
		Context:      ctx,
		SubmittedAt:  time.Now(),
		ResponseChan: responseChan,
	}

	// Try to send to processing queue with timeout
	select {
	case cc.syntheticChan <- req:
		// Successfully queued
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return errors.InternalError.With("timeout queueing synthetic transaction")
	}

	// Wait for response with timeout
	select {
	case err := <-responseChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Second):
		return errors.InternalError.With("timeout waiting for synthetic transaction processing")
	}
}

// processSynthetics is the main goroutine for processing synthetic transactions
func (cc *CrossChainConductor) processSynthetics() {
	defer cc.wg.Done()

	// Periodic cleanup of old transmissions
	cleanupTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-cc.stopChan:
			cc.logger.Info("Stopping synthetic processor")
			return

		case req := <-cc.syntheticChan:
			cc.processSyntheticRequest(req)

		case <-cleanupTicker.C:
			cc.cleanupOldTransmissions()
		}
	}
}

// generateTxID generates a unique transaction ID
func (cc *CrossChainConductor) generateTxID() string {
	id := atomic.AddInt64(&cc.txIDCounter, 1)
	return fmt.Sprintf("tx-%d-%d", time.Now().Unix(), id)
}

// processSyntheticRequest processes a single synthetic transaction request
func (cc *CrossChainConductor) processSyntheticRequest(req *SyntheticRequest) {
	msgType := cc.getMessageType(req.Messages)
	destKey := cc.createDestinationKey(msgType, req.Destination)
	queue := cc.getOrCreateDestinationQueue(destKey)

	queue.mu.Lock()
	defer queue.mu.Unlock()

	// Check if destination is blocked
	if queue.IsBlocked {
		// Check if we should unblock
		if time.Since(queue.BlockedSince) > cc.retryDelay {
			cc.unblockDestinationQueue(queue)
		} else {
			// Still blocked, queue the request
			queue.QueuedRequests = append(queue.QueuedRequests, req)
			cc.logger.Debug("Destination blocked, queueing request",
				"destination", req.Destination,
				"type", msgType,
				"queue_size", len(queue.QueuedRequests))
			return
		}
	}

	// Process immediately
	cc.processRequestImmediately(req, queue, destKey)
}

// processRequestImmediately processes a request immediately
func (cc *CrossChainConductor) processRequestImmediately(req *SyntheticRequest, queue *DestinationQueue, destKey DestinationKey) {
	// Generate unique TX ID
	txID := cc.generateTxID()

	// Create pending transmission record
	pending := &PendingTransmission{
		ID:          txID,
		Messages:    req.Messages,
		Destination: req.Destination,
		DestKey:     destKey,
		Context:     req.Context,
		SubmittedAt: time.Now(),
		AttemptNum:  1,
		Callback:    req.ResponseChan,
	}

	// Track pending transmission
	queue.PendingTx[txID] = pending

	// Submit messages
	cc.logger.Debug("Submitting synthetic transaction",
		"tx_id", txID,
		"destination", req.Destination,
		"message_count", len(req.Messages))

	// Use a goroutine to avoid blocking the processor
	go func() {
		// Submit through dispatcher
		envelope := &messaging.Envelope{Messages: req.Messages}
		err := cc.dispatcher.Submit(req.Context, req.Destination, envelope)

		// Handle result
		if err != nil {
			cc.handleTransmissionError(err)
			atomic.AddInt64(&cc.syntheticsErrors, 1)
			atomic.AddInt64(&cc.transmissionErrors, 1)

			// Mark destination as blocked
			queue.mu.Lock()
			queue.IsBlocked = true
			queue.BlockedSince = time.Now()
			queue.FailureCount++
			queue.mu.Unlock()

			// Schedule retry
			if pending.AttemptNum < cc.maxRetries {
				pending.RetryAfter = time.Now().Add(cc.retryDelay)
				select {
				case cc.retryChan <- pending:
				default:
					cc.logger.Error("Retry channel full, dropping retry", "tx_id", txID)
				}
			}
		} else {
			atomic.AddInt64(&cc.syntheticsSent, 1)

			// Update queue stats
			queue.mu.Lock()
			queue.SuccessCount++
			queue.LastSuccess = time.Now()
			delete(queue.PendingTx, txID)
			queue.mu.Unlock()

			cc.logger.Debug("Successfully sent synthetic transaction",
				"tx_id", txID,
				"destination", req.Destination)
		}

		// Send response
		if req.ResponseChan != nil {
			select {
			case req.ResponseChan <- err:
			default:
				// Channel might be closed
			}
		}
	}()
}

// SubmitAnchor submits an anchor for cross-partition synchronization
func (cc *CrossChainConductor) SubmitAnchor(req *AnchorRequest) error {
	// Use unified transport if available
	if cc.unifiedTransport != nil {
		// Create a unified message for the anchor
		// Note: SourceChain and RootChain conversion would be handled by higher-level code
		anchorMsg := &UnifiedMessage{
			Type:        MessageTypeAnchor,
			Source:      req.Source,
			Destination: req.Destination,
			Sequence:    req.Sequence,
			Payload:     nil, // Would contain the wrapped anchor message
			SourceChain: nil, // Would be resolved from database context
			RootChain:   nil, // Would be resolved from database context
			BlockIndex:  req.BlockIndex,
		}

		ctx := context.Background()
		return cc.unifiedTransport.Send(ctx, []CrossChainMessage{anchorMsg})
	}

	// Fallback to direct dispatcher
	envelope := &messaging.Envelope{
		Messages: []messaging.Message{
			&messaging.BlockAnchor{
				// Anchor field needs proper wrapping
				// This is a simplified version
			},
		},
	}

	err := cc.dispatcher.Submit(context.Background(), req.Destination, envelope)
	if err != nil {
		return errors.UnknownError.WithFormat("failed to submit anchor: %w", err)
	}

	return nil
}

// SendCrossChainMessages sends a batch of crosschain messages
func (cc *CrossChainConductor) SendCrossChainMessages(
	ctx context.Context,
	messages []CrossChainMessage,
) error {
	// Use unified transport if available
	if cc.unifiedTransport != nil {
		return cc.unifiedTransport.Send(ctx, messages)
	}

	// Fallback to individual message sending
	for _, msg := range messages {
		switch msg.GetType() {
		case MessageTypeSynthetic:
			if err := cc.SubmitSynthetic(ctx, []messaging.Message{msg.GetPayload()}, msg.GetDestination()); err != nil {
				return err
			}
		case MessageTypeAnchor:
			// Handle anchor submission
			// This would need proper anchor request construction
		default:
			cc.logger.Debug("Unsupported message type for fallback", "type", msg.GetType())
		}
	}

	return nil
}

// createDestinationKey creates a key for destination+type combination
func (cc *CrossChainConductor) createDestinationKey(msgType MessageType, destination *url.URL) DestinationKey {
	return DestinationKey{
		Type:        msgType,
		Destination: destination.String(),
	}
}

// getOrCreateDestinationQueue gets or creates a destination queue
func (cc *CrossChainConductor) getOrCreateDestinationQueue(key DestinationKey) *DestinationQueue {
	cc.queuesMutex.Lock()
	defer cc.queuesMutex.Unlock()

	queue, exists := cc.destinationQueues[key]
	if !exists {
		queue = &DestinationQueue{
			Key:            key,
			IsBlocked:      false,
			PendingTx:      make(map[string]*PendingTransmission),
			QueuedRequests: make([]*SyntheticRequest, 0),
			LastSuccess:    time.Now(),
		}
		cc.destinationQueues[key] = queue
		cc.logger.Debug("Created new destination queue", "type", key.Type, "destination", key.Destination)
	}
	return queue
}

// cleanupOldTransmissions removes old pending transmissions
func (cc *CrossChainConductor) cleanupOldTransmissions() {
	cutoff := time.Now().Add(-30 * time.Minute)
	cleaned := 0

	cc.queuesMutex.RLock()
	queues := make([]*DestinationQueue, 0, len(cc.destinationQueues))
	for _, queue := range cc.destinationQueues {
		queues = append(queues, queue)
	}
	cc.queuesMutex.RUnlock()

	for _, queue := range queues {
		cleaned += cc.cleanupQueueTransmissions(queue, cutoff)
	}

	if cleaned > 0 {
		cc.logger.Info("Cleaned up old transmissions", "count", cleaned)
	}
}

// cleanupQueueTransmissions cleans up old transmissions in a queue
func (cc *CrossChainConductor) cleanupQueueTransmissions(queue *DestinationQueue, cutoff time.Time) int {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	cleaned := 0
	for txID, pending := range queue.PendingTx {
		if pending.SubmittedAt.Before(cutoff) {
			delete(queue.PendingTx, txID)
			cleaned++
		}
	}

	return cleaned
}

// unblockDestinationQueue unblocks a destination queue and processes queued requests
func (cc *CrossChainConductor) unblockDestinationQueue(queue *DestinationQueue) {
	queue.IsBlocked = false
	queue.BlockedSince = time.Time{}

	// Process queued requests
	queued := queue.QueuedRequests
	queue.QueuedRequests = nil

	cc.logger.Info("Unblocking destination queue",
		"type", queue.Key.Type,
		"destination", queue.Key.Destination,
		"queued_requests", len(queued))

	// Process each queued request
	for _, req := range queued {
		cc.processRequestImmediately(req, queue, queue.Key)
	}
}