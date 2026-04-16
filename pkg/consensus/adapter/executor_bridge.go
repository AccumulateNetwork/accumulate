// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package adapter

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
)

// isNotFoundError checks if an error indicates a not-found condition.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Check error message for "notFound" which is what pkg/errors returns
	return strings.Contains(err.Error(), "notFound")
}

// ExecutorBridge bridges the DAG-BFT consensus to the Accumulate Executor.
// It implements the ConsensusAdapter interface.
type ExecutorBridge struct {
	executor    execute.Executor
	partitionID string

	mu                     sync.RWMutex
	lastBlockIndex         uint64
	lastBlockHash          [32]byte
	validators             []ValidatorInfo
	validatorChangeHandler func(validators []ValidatorInfo)

	// Open block state (for multi-certificate blocks)
	openBlock execute.Block

	// Pipelined commit: at most one commit in flight
	pendingCommit chan error // nil when no commit in flight
}

// ExecutorBridgeConfig holds configuration for creating an ExecutorBridge.
type ExecutorBridgeConfig struct {
	// Executor is the Accumulate executor.
	Executor execute.Executor

	// PartitionID is the partition this bridge operates on.
	PartitionID string

	// EventBus is used to subscribe to global value changes.
	EventBus *events.Bus
}

// NewExecutorBridge creates a new ExecutorBridge with the given configuration.
func NewExecutorBridge(config ExecutorBridgeConfig) (*ExecutorBridge, error) {
	if config.Executor == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	if config.PartitionID == "" {
		return nil, fmt.Errorf("partition ID is required")
	}

	bridge := &ExecutorBridge{
		executor:    config.Executor,
		partitionID: config.PartitionID,
		validators:  make([]ValidatorInfo, 0),
	}

	// Get initial state (handle empty/fresh databases)
	params, hash, err := config.Executor.LastBlock()
	if err != nil {
		// NotFound is expected for fresh databases before genesis is loaded
		if !isNotFoundError(err) {
			return nil, fmt.Errorf("get last block: %w", err)
		}
		// Fresh database - start from block 0 with empty hash
		bridge.lastBlockIndex = 0
		bridge.lastBlockHash = [32]byte{}
	} else {
		if params != nil {
			bridge.lastBlockIndex = params.Index
		}
		bridge.lastBlockHash = hash
	}

	// Subscribe to global value changes if event bus is provided
	if config.EventBus != nil {
		events.SubscribeSync(config.EventBus, func(e events.WillChangeGlobals) error {
			bridge.updateValidatorsFromGlobals(e.New)
			return nil
		})
	}

	return bridge, nil
}

// updateValidatorsFromGlobals extracts validators from global values and updates the stored set.
func (b *ExecutorBridge) updateValidatorsFromGlobals(globals *network.GlobalValues) {
	if globals == nil || globals.Network == nil {
		return
	}

	validators := make([]ValidatorInfo, 0)
	for _, v := range globals.Network.Validators {
		if !v.IsActiveOn(b.partitionID) {
			continue
		}

		if len(v.PublicKey) != 32 {
			slog.Warn("Validator has invalid public key size",
				"expected", 32,
				"actual", len(v.PublicKey))
			continue
		}

		var pubKey [32]byte
		copy(pubKey[:], v.PublicKey)

		validators = append(validators, ValidatorInfo{
			PublicKey: pubKey,
			Stake:     1, // Default stake
			Active:    true,
		})
	}

	b.mu.Lock()
	oldValidators := b.validators
	b.validators = validators
	handler := b.validatorChangeHandler
	b.mu.Unlock()

	// Notify handler if validators changed
	if handler != nil && !validatorSetsEqual(oldValidators, validators) {
		slog.Info("Validator set changed",
			"partition", b.partitionID,
			"oldCount", len(oldValidators),
			"newCount", len(validators))
		handler(validators)
	}
}

// validatorSetsEqual compares two validator sets for equality.
func validatorSetsEqual(a, b []ValidatorInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].PublicKey != b[i].PublicKey ||
			a[i].Stake != b[i].Stake ||
			a[i].Active != b[i].Active {
			return false
		}
	}
	return true
}

// SetValidators explicitly sets the validator set.
// This can be used for initial setup or testing.
func (b *ExecutorBridge) SetValidators(validators []ValidatorInfo) {
	b.mu.Lock()
	oldValidators := b.validators
	b.validators = make([]ValidatorInfo, len(validators))
	copy(b.validators, validators)
	handler := b.validatorChangeHandler
	b.mu.Unlock()

	// Notify handler if validators changed
	if handler != nil && !validatorSetsEqual(oldValidators, validators) {
		handler(validators)
	}
}

// waitForPendingCommit waits for any in-flight background commit to finish.
// Must NOT hold b.mu when calling this.
func (b *ExecutorBridge) waitForPendingCommit() error {
	b.mu.RLock()
	ch := b.pendingCommit
	b.mu.RUnlock()

	if ch == nil {
		return nil
	}

	err := <-ch

	b.mu.Lock()
	b.pendingCommit = nil
	b.mu.Unlock()

	return err
}

// BeginBlock opens a new block at the given index and time.
// If a prior block is still committing in the background, waits for it first.
func (b *ExecutorBridge) BeginBlock(ctx context.Context, index uint64, blockTime time.Time) error {
	// Wait for any prior commit to finish before starting a new block
	if err := b.waitForPendingCommit(); err != nil {
		return fmt.Errorf("prior commit failed: %w", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.openBlock != nil {
		return fmt.Errorf("block already open")
	}

	execParams := execute.BlockParams{
		Context:  ctx,
		IsLeader: true,
		Index:    index,
		Time:     blockTime,
	}

	block, err := b.executor.Begin(execParams)
	if err != nil {
		return fmt.Errorf("begin block: %w", err)
	}

	b.openBlock = block
	return nil
}

// ProcessCertificate processes a committed certificate's transactions into
// the currently open block.
func (b *ExecutorBridge) ProcessCertificate(ctx context.Context, params CertificateParams) error {
	b.mu.RLock()
	block := b.openBlock
	b.mu.RUnlock()

	if block == nil {
		return fmt.Errorf("no open block")
	}

	return b.processTransactions(block, params.Batches, params.LeaderRound)
}

// CommitBlock closes the currently open block and commits it in the background.
// Close() and Hash() happen synchronously. The disk-flushing Commit() runs in
// a goroutine so the next block can begin immediately. The next BeginBlock will
// wait for this commit to finish if it hasn't already.
func (b *ExecutorBridge) CommitBlock(ctx context.Context) ([32]byte, error) {
	b.mu.Lock()
	block := b.openBlock
	b.openBlock = nil
	b.mu.Unlock()

	if block == nil {
		return [32]byte{}, fmt.Errorf("no open block")
	}

	// Close is synchronous — computes final state
	state, err := block.Close()
	if err != nil {
		return [32]byte{}, fmt.Errorf("close block: %w", err)
	}

	// Hash is synchronous — needed before we return
	hash, err := state.Hash()
	if err != nil {
		state.Discard()
		return [32]byte{}, fmt.Errorf("get block hash: %w", err)
	}

	b.mu.Lock()
	b.lastBlockHash = hash
	b.mu.Unlock()

	// Commit runs in background — disk flush happens while next block processes
	ch := make(chan error, 1)
	b.mu.Lock()
	b.pendingCommit = ch
	b.mu.Unlock()

	go func() {
		commitErr := state.Commit()
		if commitErr != nil {
			slog.Error("Background block commit failed", "error", commitErr)
		}

		// Check for validator updates after commit
		if updates, changed := state.DidUpdateValidators(); changed {
			b.handleValidatorUpdates(updates)
		}

		ch <- commitErr
	}()

	return hash, nil
}

// processTransactions processes all transactions from batches into a block.
func (b *ExecutorBridge) processTransactions(block execute.Block, batches map[types.BatchDigest]*types.Batch, round types.Round) error {
	for digest, batch := range batches {
		if batch == nil {
			slog.Warn("Missing batch in certificate",
				"digest", digest.String(),
				"round", round)
			continue
		}

		for i, txBytes := range batch.Transactions {
			envelope := new(messaging.Envelope)
			if err := envelope.UnmarshalBinary(txBytes); err != nil {
				slog.Debug("Failed to unmarshal transaction",
					"error", err,
					"batch", digest.String(),
					"index", i)
				continue
			}

			statuses, err := block.Process(envelope)
			if err != nil {
				slog.Warn("Failed to process transaction",
					"error", err,
					"batch", digest.String(),
					"index", i)
				continue
			}

			for _, status := range statuses {
				if status.Error != nil {
					slog.Debug("Transaction failed",
						"error", status.Error,
						"code", status.Code)
				}
			}
		}
	}
	return nil
}

// ProduceBlock is the legacy single-shot method. It opens, processes, and
// commits a block in one call. Kept for backward compatibility.
func (b *ExecutorBridge) ProduceBlock(ctx context.Context, params BlockParams) ([32]byte, error) {
	// Create executor block params
	// Note: We don't have CometBFT CommitInfo/Evidence, so we leave them nil
	execParams := execute.BlockParams{
		Context:  ctx,
		IsLeader: params.IsLeader,
		Index:    params.Index,
		Time:     params.Time,
		// CommitInfo and Evidence are CometBFT-specific, not needed for DAG-BFT
	}

	// Begin block
	block, err := b.executor.Begin(execParams)
	if err != nil {
		return [32]byte{}, fmt.Errorf("begin block: %w", err)
	}

	// Process transactions from all batches
	txCount := 0
	for digest, batch := range params.Batches {
		if batch == nil {
			slog.Warn("Missing batch in certificate",
				"digest", digest.String(),
				"round", params.LeaderRound)
			continue
		}

		for i, txBytes := range batch.Transactions {
			// Unmarshal transaction to envelope
			envelope := new(messaging.Envelope)
			if err := envelope.UnmarshalBinary(txBytes); err != nil {
				slog.Debug("Failed to unmarshal transaction",
					"error", err,
					"batch", digest.String(),
					"index", i)
				continue
			}

			// Process the envelope
			statuses, err := block.Process(envelope)
			if err != nil {
				slog.Warn("Failed to process transaction",
					"error", err,
					"batch", digest.String(),
					"index", i)
				continue
			}

			// Log any failed transactions
			for _, status := range statuses {
				if status.Error != nil {
					slog.Debug("Transaction failed",
						"error", status.Error,
						"code", status.Code)
				}
			}

			txCount++
		}
	}

	// Close block
	state, err := block.Close()
	if err != nil {
		return [32]byte{}, fmt.Errorf("close block: %w", err)
	}

	// Get block hash
	hash, err := state.Hash()
	if err != nil {
		state.Discard()
		return [32]byte{}, fmt.Errorf("get block hash: %w", err)
	}

	// Commit changes
	if err := state.Commit(); err != nil {
		return [32]byte{}, fmt.Errorf("commit block: %w", err)
	}

	// Update our tracking
	b.mu.Lock()
	b.lastBlockIndex = params.Index
	b.lastBlockHash = hash
	b.mu.Unlock()

	slog.Debug("Produced block",
		"index", params.Index,
		"round", params.LeaderRound,
		"transactions", txCount,
		"hash", fmt.Sprintf("%x", hash[:8]))

	// Check for validator updates
	if updates, changed := state.DidUpdateValidators(); changed {
		b.handleValidatorUpdates(updates)
	}

	return hash, nil
}

// ValidateTransaction validates a transaction before it is added to a batch.
func (b *ExecutorBridge) ValidateTransaction(tx []byte) error {
	// Unmarshal to envelope
	envelope := new(messaging.Envelope)
	if err := envelope.UnmarshalBinary(tx); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}

	// Validate using executor
	statuses, err := b.executor.Validate(envelope, false)
	if err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	// Check for validation errors
	for _, status := range statuses {
		if status.Error != nil {
			return status.Error
		}
	}

	return nil
}

// LastBlock returns the last committed block index and hash.
func (b *ExecutorBridge) LastBlock() (uint64, [32]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastBlockIndex, b.lastBlockHash, nil
}

// StateHash returns the current state hash (same as last block hash).
func (b *ExecutorBridge) StateHash() [32]byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastBlockHash
}

// Validators returns the current validator set for this partition.
func (b *ExecutorBridge) Validators() []ValidatorInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.validators) == 0 {
		return nil
	}

	// Return a copy to prevent modification
	result := make([]ValidatorInfo, len(b.validators))
	copy(result, b.validators)
	return result
}

// OnValidatorSetChange registers a callback for validator set changes.
func (b *ExecutorBridge) OnValidatorSetChange(callback func(validators []ValidatorInfo)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.validatorChangeHandler = callback
}

// handleValidatorUpdates converts executor validator updates to adapter format
// and calls the registered handler.
func (b *ExecutorBridge) handleValidatorUpdates(updates []*execute.ValidatorUpdate) {
	b.mu.RLock()
	handler := b.validatorChangeHandler
	b.mu.RUnlock()

	if handler == nil {
		return
	}

	validators := make([]ValidatorInfo, 0, len(updates))
	for _, u := range updates {
		if len(u.PublicKey) != 32 {
			continue
		}

		var pubKey [32]byte
		copy(pubKey[:], u.PublicKey)

		validators = append(validators, ValidatorInfo{
			PublicKey: pubKey,
			Stake:     uint64(u.Power),
			Active:    u.Power > 0,
		})
	}

	handler(validators)
}

// Ensure ExecutorBridge implements ConsensusAdapter
var _ ConsensusAdapter = (*ExecutorBridge)(nil)
