// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"sync/atomic"
	"time"
)

// GetMetrics returns current processing metrics
func (cc *CrossChainConductor) GetMetrics() (sent, errors, retried, transmissionErrors int64) {
	return atomic.LoadInt64(&cc.syntheticsSent),
		atomic.LoadInt64(&cc.syntheticsErrors),
		atomic.LoadInt64(&cc.syntheticsRetried),
		atomic.LoadInt64(&cc.transmissionErrors)
}

// CheckPartitionHealth returns health metrics for all partitions
func (cc *CrossChainConductor) CheckPartitionHealth() map[string]interface{} {
	health := make(map[string]interface{})

	// Get queue statistics
	cc.queuesMutex.RLock()
	queueStats := make(map[string]interface{})
	for key, queue := range cc.destinationQueues {
		queue.mu.RLock()
		stats := map[string]interface{}{
			"type":          cc.getMessageTypeName(key.Type),
			"is_blocked":    queue.IsBlocked,
			"pending_count": len(queue.PendingTx),
			"queued_count":  len(queue.QueuedRequests),
			"success_count": queue.SuccessCount,
			"failure_count": queue.FailureCount,
			"retry_count":   queue.RetryCount,
		}
		if queue.IsBlocked {
			stats["blocked_duration"] = time.Since(queue.BlockedSince).String()
		}
		if !queue.LastSuccess.IsZero() {
			stats["last_success"] = queue.LastSuccess.Format(time.RFC3339)
		}
		queue.mu.RUnlock()
		queueStats[key.Destination] = stats
	}
	cc.queuesMutex.RUnlock()

	health["queues"] = queueStats

	// Get global metrics
	sent, errors, retried, txErrors := cc.GetMetrics()
	health["global"] = map[string]interface{}{
		"synthetics_sent":      sent,
		"synthetics_errors":    errors,
		"synthetics_retried":   retried,
		"transmission_errors": txErrors,
	}

	// Get sequence tracker statistics if available
	if cc.sequenceTracker != nil {
		health["sequence_tracking"] = cc.sequenceTracker.GetStatistics()
	}

	// Get proof service metrics if available
	if cc.proofService != nil {
		health["proof_metrics"] = cc.GetProofMetrics()
	}

	// Get recovery manager status if available
	if cc.recoveryManager != nil {
		health["recovery_manager"] = map[string]interface{}{
			"status": "active",
			// Add more recovery manager metrics here
		}
	}

	// Get batch proof manager status if available
	if cc.batchProofManager != nil {
		health["batch_proof_manager"] = map[string]interface{}{
			"status": "active",
			// Add more batch proof manager metrics here
		}
	}

	return health
}

// GetProofMetrics returns proof-related metrics
func (cc *CrossChainConductor) GetProofMetrics() ProofMetrics {
	if cc.proofService == nil {
		return ProofMetrics{}
	}
	return cc.proofService.GetMetrics()
}

// getMessageTypeName returns a human-readable name for the message type
func (cc *CrossChainConductor) getMessageTypeName(t ConductorMessageType) string {
	switch t {
	case ConductorMessageTypeAnchor:
		return "anchor"
	case ConductorMessageTypeSynthetic:
		return "synthetic"
	case ConductorMessageTypeOther:
		return "other"
	default:
		return "unknown"
	}
}