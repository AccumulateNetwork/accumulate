// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
)

// Default shutdown configuration values.
const (
	// DefaultDrainTimeout is the default timeout for draining in-flight operations.
	DefaultDrainTimeout = 30 * time.Second

	// DefaultShutdownGracePeriod is the default grace period before forcing shutdown.
	DefaultShutdownGracePeriod = 5 * time.Second

	// DrainCheckInterval is how often to check if draining is complete.
	DrainCheckInterval = 10 * time.Millisecond
)

// ErrShutdownTimeout is returned when shutdown times out.
var ErrShutdownTimeout = errors.New("shutdown timed out")

// ErrShutdownInProgress is returned when shutdown is already in progress.
var ErrShutdownInProgress = errors.New("shutdown already in progress")

// ShutdownConfig holds configuration for graceful shutdown.
type ShutdownConfig struct {
	// DrainTimeout is the maximum time to wait for in-flight operations.
	DrainTimeout time.Duration

	// GracePeriod is the time to wait before forcing shutdown.
	GracePeriod time.Duration

	// PersistState enables state persistence during shutdown.
	PersistState bool

	// CheckpointStore is the store for persisting checkpoints.
	CheckpointStore *persist.Store
}

// applyDefaults fills in default values for unset configuration fields.
func (c *ShutdownConfig) applyDefaults() {
	if c.DrainTimeout <= 0 {
		c.DrainTimeout = DefaultDrainTimeout
	}
	if c.GracePeriod <= 0 {
		c.GracePeriod = DefaultShutdownGracePeriod
	}
}

// ShutdownManager handles graceful shutdown of a consensus node.
// It coordinates stopping transaction acceptance, draining in-flight
// operations, persisting state, and closing resources.
type ShutdownManager struct {
	node   *Node
	config ShutdownConfig

	// Shutdown state
	shutdownStarted  atomic.Bool
	acceptingTxns    atomic.Bool
	shutdownComplete atomic.Bool
}

// NewShutdownManager creates a new shutdown manager for the given node.
func NewShutdownManager(node *Node, config ShutdownConfig) *ShutdownManager {
	config.applyDefaults()
	return &ShutdownManager{
		node:   node,
		config: config,
	}
}

// Shutdown performs a graceful shutdown of the node.
// It follows these steps:
// 1. Stop accepting new transactions
// 2. Drain pending transaction queue
// 3. Wait for current round to complete (with timeout)
// 4. Persist state if configured
// 5. Close all resources
func (m *ShutdownManager) Shutdown(ctx context.Context) error {
	// Check if shutdown already in progress
	if !m.shutdownStarted.CompareAndSwap(false, true) {
		return ErrShutdownInProgress
	}
	defer m.shutdownComplete.Store(true)

	slog.Info("Starting graceful shutdown",
		"partition", m.node.Partition(),
		"drainTimeout", m.config.DrainTimeout)

	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, m.config.DrainTimeout+m.config.GracePeriod)
	defer cancel()

	// Step 1: Stop accepting new transactions
	m.stopAccepting()

	// Step 2: Drain pending transactions with timeout
	drainCtx, drainCancel := context.WithTimeout(ctx, m.config.DrainTimeout)
	err := m.drainPendingTransactions(drainCtx)
	drainCancel()
	if err != nil {
		slog.Warn("Drain timeout, proceeding with shutdown",
			"error", err)
	}

	// Step 3: Persist state if configured
	if m.config.PersistState && m.config.CheckpointStore != nil {
		if err := m.persistState(); err != nil {
			slog.Error("Failed to persist state during shutdown",
				"error", err)
			// Continue with shutdown despite persistence failure
		}
	}

	// Step 4: Close all resources
	m.closeResources()

	slog.Info("Graceful shutdown complete",
		"partition", m.node.Partition())

	return nil
}

// stopAccepting marks the node as no longer accepting transactions.
func (m *ShutdownManager) stopAccepting() {
	m.acceptingTxns.Store(false)
	m.node.closed.Store(true) // Mark node as closed to reject new transactions

	slog.Debug("Stopped accepting new transactions")
}

// IsAcceptingTransactions returns true if the node is accepting transactions.
func (m *ShutdownManager) IsAcceptingTransactions() bool {
	return m.acceptingTxns.Load() && !m.shutdownStarted.Load()
}

// drainPendingTransactions waits for workers to process pending transactions.
func (m *ShutdownManager) drainPendingTransactions(ctx context.Context) error {
	ticker := time.NewTicker(DrainCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ErrShutdownTimeout
		case <-ticker.C:
			// Check if all workers have drained their pending transactions
			allDrained := true
			for _, w := range m.node.workers {
				if w.PendingCount() > 0 {
					allDrained = false
					break
				}
			}

			if allDrained {
				slog.Debug("All pending transactions drained")
				return nil
			}
		}
	}
}

// persistState creates a checkpoint of the current node state.
func (m *ShutdownManager) persistState() error {
	slog.Debug("Persisting consensus state")

	// Capture current state
	snapshot := m.captureState()

	// Convert to checkpoint
	checkpoint := snapshot.ToCheckpoint()

	// Save checkpoint
	if err := m.config.CheckpointStore.Save(checkpoint); err != nil {
		return err
	}

	slog.Info("Consensus state persisted",
		"round", checkpoint.CurrentRound,
		"epoch", checkpoint.CurrentEpoch,
		"lastCommitRound", checkpoint.LastCommitRound)

	return nil
}

// captureState captures the current node state for checkpointing.
func (m *ShutdownManager) captureState() *persist.StateSnapshot {
	return &persist.StateSnapshot{
		Partition:       m.node.Partition(),
		CurrentRound:    m.node.CurrentRound(),
		CurrentEpoch:    m.node.primary.CurrentEpoch(),
		LastCommitRound: m.node.bullshark.LastCommitRound(),
		LastCommitted:   m.node.bullshark.GetLastCommitted(),
	}
}

// closeResources closes all node resources in the correct order.
func (m *ShutdownManager) closeResources() {
	slog.Debug("Closing resources")

	// Stop the node (this handles component shutdown order)
	m.node.Stop()
}

// IsShutdownComplete returns true if shutdown has completed.
func (m *ShutdownManager) IsShutdownComplete() bool {
	return m.shutdownComplete.Load()
}

// IsShutdownInProgress returns true if shutdown has started but not completed.
func (m *ShutdownManager) IsShutdownInProgress() bool {
	return m.shutdownStarted.Load() && !m.shutdownComplete.Load()
}

// StopAccepting stops the node from accepting new transactions.
// This can be called independently of Shutdown for scenarios where
// you want to stop accepting transactions but continue processing.
func (n *Node) StopAccepting() {
	n.closed.Store(true)
	slog.Info("Node stopped accepting transactions",
		"partition", n.Partition())
}

// DrainWithTimeout waits for in-flight operations to complete.
// Returns ErrShutdownTimeout if the timeout is exceeded.
func (n *Node) DrainWithTimeout(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(DrainCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ErrShutdownTimeout
		case <-ticker.C:
			// Check if all workers have drained
			allDrained := true
			totalPending := 0
			for _, w := range n.workers {
				pending := w.PendingCount()
				totalPending += pending
				if pending > 0 {
					allDrained = false
				}
			}

			if allDrained {
				slog.Debug("Node drained successfully")
				return nil
			}

			slog.Debug("Waiting for drain",
				"pendingTransactions", totalPending)
		}
	}
}

// PersistState persists the current node state to the given store.
func (n *Node) PersistState(store *persist.Store) error {
	if store == nil {
		return errors.New("store is nil")
	}

	snapshot := &persist.StateSnapshot{
		Partition:       n.Partition(),
		CurrentRound:    n.CurrentRound(),
		CurrentEpoch:    n.primary.CurrentEpoch(),
		LastCommitRound: n.bullshark.LastCommitRound(),
		LastCommitted:   n.bullshark.GetLastCommitted(),
	}

	checkpoint := snapshot.ToCheckpoint()

	if err := store.Save(checkpoint); err != nil {
		return err
	}

	slog.Info("Node state persisted",
		"partition", n.Partition(),
		"round", checkpoint.CurrentRound,
		"lastCommitRound", checkpoint.LastCommitRound)

	return nil
}

// GracefulShutdown performs a graceful shutdown using default configuration.
func (n *Node) GracefulShutdown(ctx context.Context) error {
	manager := NewShutdownManager(n, ShutdownConfig{})
	return manager.Shutdown(ctx)
}

// GracefulShutdownWithPersistence performs a graceful shutdown and persists state.
func (n *Node) GracefulShutdownWithPersistence(ctx context.Context, store *persist.Store) error {
	manager := NewShutdownManager(n, ShutdownConfig{
		PersistState:    true,
		CheckpointStore: store,
	})
	return manager.Shutdown(ctx)
}
