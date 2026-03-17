// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// Default recovery configuration values.
const (
	// DefaultCatchUpTimeout is the default timeout for catching up with peers.
	DefaultCatchUpTimeout = 60 * time.Second

	// DefaultPeerSyncInterval is how often to request missing certificates.
	DefaultPeerSyncInterval = 100 * time.Millisecond

	// DefaultReconnectTimeout is the default timeout for reconnecting to peers.
	DefaultReconnectTimeout = 30 * time.Second
)

// ErrRecoveryFailed is returned when recovery fails.
var ErrRecoveryFailed = errors.New("recovery failed")

// ErrInvalidCheckpoint is returned when the checkpoint is invalid.
var ErrInvalidCheckpoint = errors.New("invalid checkpoint")

// ErrPartitionMismatch is returned when the checkpoint partition doesn't match.
var ErrPartitionMismatch = errors.New("partition mismatch")

// RecoveryConfig holds configuration for node recovery.
type RecoveryConfig struct {
	// CatchUpTimeout is the maximum time to spend catching up.
	CatchUpTimeout time.Duration

	// PeerSyncInterval is how often to request missing certificates.
	PeerSyncInterval time.Duration

	// ReconnectTimeout is the maximum time to spend reconnecting to peers.
	ReconnectTimeout time.Duration

	// SkipCatchUp skips the catch-up phase (useful for testing).
	SkipCatchUp bool
}

// applyDefaults fills in default values for unset configuration fields.
func (c *RecoveryConfig) applyDefaults() {
	if c.CatchUpTimeout <= 0 {
		c.CatchUpTimeout = DefaultCatchUpTimeout
	}
	if c.PeerSyncInterval <= 0 {
		c.PeerSyncInterval = DefaultPeerSyncInterval
	}
	if c.ReconnectTimeout <= 0 {
		c.ReconnectTimeout = DefaultReconnectTimeout
	}
}

// RecoveryManager handles restart recovery for a consensus node.
// It coordinates loading persisted state, reconnecting to the network,
// and catching up with missed certificates.
type RecoveryManager struct {
	node   *Node
	config RecoveryConfig
	store  *persist.Store
}

// NewRecoveryManager creates a new recovery manager.
func NewRecoveryManager(node *Node, store *persist.Store, config RecoveryConfig) *RecoveryManager {
	config.applyDefaults()
	return &RecoveryManager{
		node:   node,
		config: config,
		store:  store,
	}
}

// RecoveryResult holds the result of a recovery operation.
type RecoveryResult struct {
	// Recovered indicates if recovery was performed.
	Recovered bool

	// FromCheckpoint indicates if state was loaded from a checkpoint.
	FromCheckpoint bool

	// RestoredRound is the round restored from checkpoint.
	RestoredRound types.Round

	// CurrentRound is the round after catch-up.
	CurrentRound types.Round

	// CertificatesCaughtUp is the number of certificates received during catch-up.
	CertificatesCaughtUp int
}

// Recover performs restart recovery.
// It follows these steps:
// 1. Load persisted state from checkpoint
// 2. Reconnect to the network
// 3. Request missing certificates from peers
// 4. Process committed certificates
// 5. Resume normal operation
func (m *RecoveryManager) Recover(ctx context.Context) (*RecoveryResult, error) {
	result := &RecoveryResult{}

	slog.Info("Starting recovery",
		"partition", m.node.Partition())

	// Step 1: Load persisted state
	checkpoint, err := m.loadCheckpoint()
	if err != nil {
		if errors.Is(err, persist.ErrNoCheckpoint) {
			slog.Info("No checkpoint found, starting fresh")
			return result, nil
		}
		return nil, fmt.Errorf("load checkpoint: %w", err)
	}

	result.FromCheckpoint = true
	result.RestoredRound = checkpoint.CurrentRound

	// Validate checkpoint
	if err := m.validateCheckpoint(checkpoint); err != nil {
		return nil, fmt.Errorf("validate checkpoint: %w", err)
	}

	// Step 2: Restore state from checkpoint
	if err := m.restoreState(checkpoint); err != nil {
		return nil, fmt.Errorf("restore state: %w", err)
	}

	result.Recovered = true

	// Step 3: Catch up with peers (if not skipped and gossip available)
	if !m.config.SkipCatchUp && m.node.gossip != nil {
		catchUpCtx, cancel := context.WithTimeout(ctx, m.config.CatchUpTimeout)
		caught, err := m.catchUp(catchUpCtx)
		cancel()

		if err != nil {
			slog.Warn("Catch-up encountered errors",
				"error", err)
			// Continue despite catch-up errors
		}

		result.CertificatesCaughtUp = caught
	}

	result.CurrentRound = m.node.CurrentRound()

	slog.Info("Recovery complete",
		"partition", m.node.Partition(),
		"restoredRound", result.RestoredRound,
		"currentRound", result.CurrentRound,
		"certificatesCaughtUp", result.CertificatesCaughtUp)

	return result, nil
}

// loadCheckpoint loads the checkpoint from storage.
func (m *RecoveryManager) loadCheckpoint() (*persist.Checkpoint, error) {
	if m.store == nil {
		return nil, persist.ErrNoCheckpoint
	}

	checkpoint, err := m.store.Load()
	if err != nil {
		return nil, err
	}

	slog.Debug("Loaded checkpoint",
		"round", checkpoint.CurrentRound,
		"epoch", checkpoint.CurrentEpoch,
		"lastCommitRound", checkpoint.LastCommitRound)

	return checkpoint, nil
}

// validateCheckpoint checks if the checkpoint is valid for this node.
func (m *RecoveryManager) validateCheckpoint(checkpoint *persist.Checkpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCheckpoint, err)
	}

	// Verify partition matches
	if checkpoint.Partition != m.node.Partition() {
		return fmt.Errorf("%w: checkpoint is for %s, node is %s",
			ErrPartitionMismatch, checkpoint.Partition, m.node.Partition())
	}

	return nil
}

// restoreState restores node state from a checkpoint.
func (m *RecoveryManager) restoreState(checkpoint *persist.Checkpoint) error {
	slog.Debug("Restoring state from checkpoint",
		"round", checkpoint.CurrentRound,
		"epoch", checkpoint.CurrentEpoch)

	// Restore primary state
	m.node.primary.SetRound(checkpoint.CurrentRound)

	// Restore Bullshark state
	m.node.bullshark.SetLastCommitRound(checkpoint.LastCommitRound)
	for author, round := range checkpoint.LastCommitted {
		// Use SetLastCommittedForAuthor to restore per-author commit tracking
		m.node.bullshark.SetLastCommittedForAuthor(author, round)
	}

	// Restore DAG commit round
	m.node.dag.SetLastCommitRound(checkpoint.LastCommitRound)

	slog.Info("State restored from checkpoint",
		"round", checkpoint.CurrentRound,
		"lastCommitRound", checkpoint.LastCommitRound)

	return nil
}

// catchUp requests and processes missing certificates from peers.
func (m *RecoveryManager) catchUp(ctx context.Context) (int, error) {
	slog.Debug("Starting catch-up")

	caughtUp := 0

	// Get current state
	currentRound := m.node.CurrentRound()
	lastCommitRound := m.node.bullshark.LastCommitRound()

	// If we're behind, request certificates from peers
	// The CertSyncer in the primary handles the actual sync
	// We just need to wait for it to catch up

	ticker := time.NewTicker(m.config.PeerSyncInterval)
	defer ticker.Stop()

	startRound := currentRound
	stableCount := 0
	lastRound := currentRound

	for {
		select {
		case <-ctx.Done():
			return caughtUp, ctx.Err()

		case <-ticker.C:
			newRound := m.node.CurrentRound()

			if newRound > lastRound {
				caughtUp += int(newRound - lastRound)
				lastRound = newRound
				stableCount = 0
			} else {
				stableCount++
			}

			// If round has been stable for a while, we've caught up
			// (10 intervals without progress = caught up or no peers)
			if stableCount >= 10 {
				slog.Debug("Catch-up complete",
					"startRound", startRound,
					"endRound", newRound,
					"lastCommitRound", lastCommitRound)
				return caughtUp, nil
			}
		}
	}
}

// RecoverFromCheckpoint is a convenience function to recover a node from a checkpoint.
func RecoverFromCheckpoint(ctx context.Context, node *Node, store *persist.Store) (*RecoveryResult, error) {
	manager := NewRecoveryManager(node, store, RecoveryConfig{})
	return manager.Recover(ctx)
}

// RecoverAndStart recovers a node and starts it.
func RecoverAndStart(ctx context.Context, node *Node, store *persist.Store) (*RecoveryResult, error) {
	// First recover state
	manager := NewRecoveryManager(node, store, RecoveryConfig{
		SkipCatchUp: true, // Catch-up will happen after start
	})

	result, err := manager.Recover(ctx)
	if err != nil {
		return nil, fmt.Errorf("recovery failed: %w", err)
	}

	// Start the node
	if err := node.Start(ctx); err != nil {
		return nil, fmt.Errorf("start after recovery failed: %w", err)
	}

	return result, nil
}

// NewNodeWithRecovery creates a new node and attempts to recover from a checkpoint.
// If no checkpoint exists, it returns a fresh node.
func NewNodeWithRecovery(config NodeConfig, committee *types.Committee, store *persist.Store) (*Node, *RecoveryResult, error) {
	// Create a basic node first (without host/pubsub for now)
	node, err := NewNode(config, committee, nil, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create node: %w", err)
	}

	// Try to recover
	result := &RecoveryResult{}

	if store != nil && store.Exists() {
		checkpoint, err := store.Load()
		if err == nil {
			// Validate partition
			if checkpoint.Partition == config.Partition {
				// Restore state
				node.primary.SetRound(checkpoint.CurrentRound)
				node.bullshark.SetLastCommitRound(checkpoint.LastCommitRound)
				node.dag.SetLastCommitRound(checkpoint.LastCommitRound)

				for author, round := range checkpoint.LastCommitted {
					node.bullshark.SetLastCommittedForAuthor(author, round)
				}

				result.Recovered = true
				result.FromCheckpoint = true
				result.RestoredRound = checkpoint.CurrentRound
			}
		}
	}

	return node, result, nil
}

// SetRound sets the current round on the primary.
// This is exported for recovery purposes.
func (n *Node) SetRound(round types.Round) {
	n.primary.SetRound(round)
}
