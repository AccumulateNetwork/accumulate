// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// periodicHealthCheck performs periodic health checks on partitions
func (rm *RecoveryManager) periodicHealthCheck() {
	ticker := time.NewTicker(rm.checkInterval)
	defer ticker.Stop()

	for range ticker.C {
		rm.checkPartitionHealth()
	}
}

// checkPartitionHealth checks the health of all known partitions
func (rm *RecoveryManager) checkPartitionHealth() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get network information
	networkInfo, err := rm.getNetworkInfo(ctx)
	if err != nil {
		rm.logger.Error("Failed to get network info", "error", err)
		return
	}

	// Begin a database batch for checking
	batch := rm.db.Begin(false)
	defer batch.Discard()

	// Check each partition pair
	for srcID, srcInfo := range networkInfo.Partitions {
		if !srcInfo.IsHealthy {
			rm.logger.Debug("Skipping unhealthy partition", "partition", srcID)
			continue
		}

		for dstID, dstInfo := range networkInfo.Partitions {
			if srcID == dstID || !dstInfo.IsHealthy {
				continue
			}

			// Check for missing anchors
			rm.checkMissingAnchors(batch, srcInfo, dstInfo)

			// Check for missing synthetics
			rm.checkMissingSynthetics(batch, srcInfo, dstInfo)
		}
	}

	rm.logger.Debug("Health check completed",
		"partitions", len(networkInfo.Partitions))
}

// checkMissingAnchors checks for missing anchors between partitions
func (rm *RecoveryManager) checkMissingAnchors(batch *database.Batch, src, dst *PartitionInfo) {
	// Get the anchor ledger for the source partition
	sourceUrl := protocol.PartitionUrl(src.ID)
	anchorAccount := batch.Account(sourceUrl.JoinPath(protocol.AnchorPool))

	// Check if we have the expected anchors
	ledger := new(protocol.AnchorLedger)
	err := anchorAccount.Main().GetAs(ledger)
	if err != nil {
		rm.logger.Debug("Failed to get anchor ledger",
			"partition", src.ID,
			"error", err)
		return
	}

	// Check sequence continuity
	currentSequence := ledger.MinorBlockSequenceNumber
	src.LastAnchor = currentSequence

	// If we detect gaps, trigger recovery
	if dst.LastAnchor > 0 && currentSequence > dst.LastAnchor+10 {
		rm.logger.Info("Detected missing anchors",
			"source", src.ID,
			"destination", dst.ID,
			"expected", dst.LastAnchor,
			"current", currentSequence)

		// Create recovery request
		req := &RecoveryRequest{
			Type:        ConductorMessageTypeAnchor,
			Source:      src.ID,
			Destination: dst.ID,
			FromNumber:  dst.LastAnchor + 1,
			ToNumber:    currentSequence,
			Requester:   dst.ID,
			Priority:    1,
			RequestedAt: time.Now(),
		}

		// Submit recovery request
		select {
		case rm.recoveryQueue <- req:
			rm.logger.Info("Submitted anchor recovery request",
				"source", src.ID,
				"destination", dst.ID,
				"range", [2]uint64{req.FromNumber, req.ToNumber})
		default:
			rm.logger.Debug("Recovery queue full, skipping anchor recovery",
				"source", src.ID,
				"destination", dst.ID)
		}
	}
}

// checkMissingSynthetics checks for missing synthetic transactions between partitions
func (rm *RecoveryManager) checkMissingSynthetics(batch *database.Batch, src, dst *PartitionInfo) {
	// Get the synthetic ledger for the source partition
	sourceUrl := protocol.PartitionUrl(src.ID)
	synthAccount := batch.Account(sourceUrl.JoinPath(protocol.Synthetic))

	// Check if we have the expected synthetics
	ledger := new(protocol.SyntheticLedger)
	err := synthAccount.Main().GetAs(ledger)
	if err != nil {
		rm.logger.Debug("Failed to get synthetic ledger",
			"partition", src.ID,
			"error", err)
		return
	}

	// Get the sequence for the destination partition
	destUrl := protocol.PartitionUrl(dst.ID)
	destLedger := ledger.Partition(destUrl)
	if destLedger == nil {
		return
	}

	currentSequence := destLedger.Produced
	src.LastSynthetic = currentSequence

	// If we detect gaps, trigger recovery
	if dst.LastSynthetic > 0 && currentSequence > dst.LastSynthetic+10 {
		rm.logger.Info("Detected missing synthetics",
			"source", src.ID,
			"destination", dst.ID,
			"expected", dst.LastSynthetic,
			"current", currentSequence)

		// Create recovery request
		req := &RecoveryRequest{
			Type:        ConductorMessageTypeSynthetic,
			Source:      src.ID,
			Destination: dst.ID,
			FromNumber:  dst.LastSynthetic + 1,
			ToNumber:    currentSequence,
			Requester:   dst.ID,
			Priority:    1,
			RequestedAt: time.Now(),
		}

		// Submit recovery request
		select {
		case rm.recoveryQueue <- req:
			rm.logger.Info("Submitted synthetic recovery request",
				"source", src.ID,
				"destination", dst.ID,
				"range", [2]uint64{req.FromNumber, req.ToNumber})
		default:
			rm.logger.Debug("Recovery queue full, skipping synthetic recovery",
				"source", src.ID,
				"destination", dst.ID)
		}
	}
}

// GetHealthStatus returns the current health status of the recovery manager
func (rm *RecoveryManager) GetHealthStatus() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	status := map[string]interface{}{
		"active_sessions": len(rm.activeRecovery),
		"queue_size":      len(rm.recoveryQueue),
		"max_concurrent":  rm.maxConcurrentRecovery,
		"check_interval":  rm.checkInterval.String(),
	}

	// Add session details
	sessions := make([]map[string]interface{}, 0, len(rm.activeRecovery))
	for key, session := range rm.activeRecovery {
		sessions = append(sessions, map[string]interface{}{
			"key":      key,
			"status":   session.Status,
			"progress": session.Progress,
			"recovered": session.Recovered,
			"total":    session.Total,
			"duration": time.Since(session.StartedAt).String(),
		})
	}
	status["sessions"] = sessions

	return status
}

// CleanupStaleSessions removes stale recovery sessions
func (rm *RecoveryManager) CleanupStaleSessions() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	now := time.Now()
	staleTimeout := 30 * time.Minute

	for key, session := range rm.activeRecovery {
		if now.Sub(session.StartedAt) > staleTimeout {
			rm.logger.Info("Removing stale recovery session",
				"key", key,
				"age", now.Sub(session.StartedAt))
			delete(rm.activeRecovery, key)
		}
	}
}