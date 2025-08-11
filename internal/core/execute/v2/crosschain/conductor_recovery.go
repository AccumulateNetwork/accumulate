// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"fmt"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// InitRecoveryManager initializes the recovery manager with database and client
func (cc *CrossChainConductor) InitRecoveryManager(db database.Beginner, client api.Querier) {
	if cc.recoveryManager != nil {
		cc.logger.Info("Recovery manager already initialized")
		return
	}

	cc.recoveryManager = NewRecoveryManager(cc, db, client)
	cc.recoveryManager.Start()
	cc.logger.Info("Recovery manager initialized and started")
}

// monitorTransmissionErrors monitors for transmission errors
func (cc *CrossChainConductor) monitorTransmissionErrors() {
	defer cc.wg.Done()

	for {
		select {
		case <-cc.stopChan:
			cc.logger.Info("Stopping transmission error monitor")
			return

		case <-time.After(10 * time.Second):
			// Check for stuck destinations
			cc.checkStuckDestinations()
		}
	}
}

// checkStuckDestinations checks for destinations that have been blocked too long
func (cc *CrossChainConductor) checkStuckDestinations() {
	cc.queuesMutex.RLock()
	queues := make([]*DestinationQueue, 0, len(cc.destinationQueues))
	for _, queue := range cc.destinationQueues {
		queues = append(queues, queue)
	}
	cc.queuesMutex.RUnlock()

	for _, queue := range queues {
		queue.mu.RLock()
		if queue.IsBlocked && time.Since(queue.BlockedSince) > 5*time.Minute {
			cc.logger.Info("Long-blocked destination detected",
				"type", queue.Key.Type,
				"destination", queue.Key.Destination,
				"blocked_duration", time.Since(queue.BlockedSince),
				"queued", len(queue.QueuedRequests))
		}
		queue.mu.RUnlock()
	}
}

// handleTransmissionError handles transmission errors
func (cc *CrossChainConductor) handleTransmissionError(err error) {
	if err == nil {
		return
	}

	// Log the error
	cc.logger.Error("Transmission error", "error", err)

	// TODO: Implement error-specific handling
	// - Network errors: retry with backoff
	// - Invalid message errors: drop and log
	// - Partition unavailable: block destination
}

// handleQueueTransmissionError handles errors for a specific queue
func (cc *CrossChainConductor) handleQueueTransmissionError(queue *DestinationQueue, err error) {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	queue.FailureCount++
	queue.IsBlocked = true
	queue.BlockedSince = time.Now()

	cc.logger.Error("Queue transmission failed",
		"type", queue.Key.Type,
		"destination", queue.Key.Destination,
		"failures", queue.FailureCount,
		"error", err)
}

// processRetries processes retry requests
func (cc *CrossChainConductor) processRetries() {
	defer cc.wg.Done()

	for {
		select {
		case <-cc.stopChan:
			cc.logger.Info("Stopping retry processor")
			return

		case pending := <-cc.retryChan:
			// Wait until retry time
			waitTime := time.Until(pending.RetryAfter)
			if waitTime > 0 {
				time.Sleep(waitTime)
			}

			// Retry the transmission
			cc.retryTransmission(pending)
		}
	}
}

// retryTransmission retries a failed transmission
func (cc *CrossChainConductor) retryTransmission(pending *PendingTransmission) {
	pending.AttemptNum++
	atomic.AddInt64(&cc.syntheticsRetried, 1)

	cc.logger.Info("Retrying transmission",
		"tx_id", pending.ID,
		"destination", pending.Destination,
		"attempt", pending.AttemptNum)

	// Get the destination queue
	queue := cc.getOrCreateDestinationQueue(pending.DestKey)

	// Submit messages again
	envelope := &messaging.Envelope{Messages: pending.Messages}
	err := cc.dispatcher.Submit(pending.Context, pending.Destination, envelope)

	if err != nil {
		cc.logger.Error("Retry failed",
			"tx_id", pending.ID,
			"attempt", pending.AttemptNum,
			"error", err)

		// Schedule another retry if under limit
		if pending.AttemptNum < cc.maxRetries {
			pending.RetryAfter = time.Now().Add(cc.retryDelay * time.Duration(pending.AttemptNum))
			select {
			case cc.retryChan <- pending:
			default:
				cc.logger.Error("Retry channel full, dropping retry", "tx_id", pending.ID)
			}
		} else {
			// Max retries reached, clean up
			queue.mu.Lock()
			delete(queue.PendingTx, pending.ID)
			queue.mu.Unlock()

			// Send final error
			if pending.Callback != nil {
				select {
				case pending.Callback <- errors.InternalError.WithFormat("max retries reached: %w", err):
				default:
				}
			}
		}
	} else {
		// Success!
		atomic.AddInt64(&cc.syntheticsSent, 1)

		queue.mu.Lock()
		delete(queue.PendingTx, pending.ID)
		queue.SuccessCount++
		queue.LastSuccess = time.Now()
		queue.mu.Unlock()

		// Send success response
		if pending.Callback != nil {
			select {
			case pending.Callback <- nil:
			default:
			}
		}
	}
}

// RequestMissingTransactions requests missing anchors or synthetic transactions
func (cc *CrossChainConductor) RequestMissingTransactions(
	msgType ConductorMessageType,
	source, destination string,
	fromNum, toNum uint64,
) (*RecoveryResponse, error) {
	if cc.recoveryManager == nil {
		return nil, errors.NotReady.With("recovery manager not initialized")
	}

	req := &RecoveryRequest{
		Type:        msgType,
		Source:      source,
		Destination: destination,
		FromNumber:  fromNum,
		ToNumber:    toNum,
		Requester:   destination,
		Priority:    1,
	}

	return cc.recoveryManager.RequestMissingTransactions(req)
}

// RequestBatchProofRecovery requests missing messages using collection proofs (simplified interface)
func (cc *CrossChainConductor) RequestBatchProofRecovery(source string, msgType ConductorMessageType, gapStart, gapEnd uint64) error {
	if cc.batchProofManager == nil {
		// Fallback to regular recovery manager if batch proof manager not available
		if cc.recoveryManager != nil {
			req := &RecoveryRequest{
				Type:        msgType,
				Source:      source,
				Destination: cc.Describe.PartitionUrl().String(),
				FromNumber:  gapStart,
				ToNumber:    gapEnd,
				Requester:   cc.Describe.PartitionUrl().String(),
				RequestedAt: time.Now(),
			}
			_, err := cc.recoveryManager.RequestMissingTransactions(req)
			return err
		}
		return errors.NotReady.With("no recovery mechanism available")
	}

	// Convert to batch proof request
	missingSeqs := make([]uint64, 0, gapEnd-gapStart+1)
	for seq := gapStart; seq <= gapEnd; seq++ {
		missingSeqs = append(missingSeqs, seq)
	}

	// Determine chain URL based on message type
	var chainURL *url.URL
	switch msgType {
	case ConductorMessageTypeSynthetic:
		chainURL = protocol.PartitionUrl(source).JoinPath(protocol.Synthetic)
	case ConductorMessageTypeAnchor:
		chainURL = protocol.PartitionUrl(source).JoinPath(protocol.AnchorPool)
	default:
		return errors.BadRequest.WithFormat("unsupported message type: %d", msgType)
	}

	return cc.RequestMissingTransactionsWithBatchProof(source, msgType, missingSeqs, chainURL)
}

// RequestMissingTransactionsWithBatchProof requests missing transactions using collection proofs for efficiency
func (cc *CrossChainConductor) RequestMissingTransactionsWithBatchProof(
	partitionID string,
	msgType ConductorMessageType,
	missingSequences []uint64,
	chainURL *url.URL,
) error {
	if cc.batchProofManager == nil {
		return errors.NotReady.With("batch proof recovery manager not initialized")
	}

	// Convert MessageType to RecoveryType
	var recoveryType RecoveryType
	switch msgType {
	case ConductorMessageTypeAnchor:
		recoveryType = RecoveryTypeAnchor
	case ConductorMessageTypeSynthetic:
		recoveryType = RecoveryTypeSynthetic
	default:
		return errors.BadRequest.WithFormat("unsupported message type for batch recovery: %d", msgType)
	}

	cc.logger.Info("Requesting batch proof recovery",
		"partition", partitionID,
		"type", recoveryType.String(),
		"sequences", len(missingSequences),
		"chain", chainURL)

	// Create batch recovery request
	req := &BatchRecoveryRequest{
		PartitionID:      partitionID,
		Type:             recoveryType,
		MissingSequences: missingSequences,
		ChainURL:         chainURL,
		RequestTime:      time.Now(),
		Callback: func(response *BatchRecoveryResponse) {
			cc.handleBatchRecoveryResponse(response)
		},
	}

	// Send to batch proof manager
	cc.batchProofManager.RequestBatchRecovery(req)
	return nil
}

// handleBatchRecoveryResponse processes the response from batch proof recovery
func (cc *CrossChainConductor) handleBatchRecoveryResponse(response *BatchRecoveryResponse) {
	if response.Error != nil {
		cc.logger.Error("Batch recovery failed",
			"partition", response.PartitionID,
			"type", response.Type,
			"error", response.Error)
		return
	}

	cc.logger.Info("Batch recovery successful",
		"partition", response.PartitionID,
		"type", response.Type,
		"batch_size", response.BatchSize,
		"proof_savings", response.ProofSavings,
		"transactions", len(response.Transactions))

	// Process recovered transactions
	for _, tx := range response.Transactions {
		cc.logger.Debug("Processing recovered transaction",
			"sequence", tx.SequenceNum,
			"hash", fmt.Sprintf("%x", tx.Hash[:8]),
			"type", tx.Type)

		// Here you would submit the recovered transaction back to the destination partition
		// This would integrate with the existing message processing pipeline
	}

	// Log collection proof efficiency metrics
	if response.CollectionProof != nil {
		cc.logger.Info("Collection proof metrics",
			"partition", response.PartitionID,
			"proof_elements", len(response.CollectionProof.Elements),
			"individual_proofs_saved", response.ProofSavings,
			"generation_time", response.ProofGenerated.Sub(time.Now().Add(-time.Since(response.ProofGenerated))))
	}
}

// HandleRecoveryRequest processes an incoming recovery request from another partition
func (cc *CrossChainConductor) HandleRecoveryRequest(req *RecoveryRequest) error {
	if cc.recoveryManager == nil {
		return errors.NotReady.With("recovery manager not initialized")
	}

	cc.logger.Info("Handling recovery request",
		"type", req.Type,
		"source", req.Source,
		"destination", req.Destination,
		"range", fmt.Sprintf("%d-%d", req.FromNumber, req.ToNumber))

	// Process the recovery request
	response, err := cc.recoveryManager.ProcessRecoveryRequest(req)
	if err != nil {
		return errors.UnknownError.WithFormat("failed to process recovery request: %w", err)
	}

	// Send the recovered transactions back
	if response.Error != nil {
		cc.logger.Error("Recovery request failed",
			"error", response.Error)
		return response.Error
	}

	cc.logger.Info("Recovery request completed",
		"transactions_recovered", response.TransactionCount,
		"proof_included", response.ProofIncluded)

	return nil
}