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
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// ExecutorBridge bridges the DAG-BFT consensus to the Accumulate Executor.
// It implements the ConsensusAdapter interface.
type ExecutorBridge struct {
	executor execute.Executor

	mu                     sync.RWMutex
	lastBlockIndex         uint64
	lastBlockHash          [32]byte
	validatorChangeHandler func(validators []ValidatorInfo)
}

// NewExecutorBridge creates a new ExecutorBridge.
func NewExecutorBridge(executor execute.Executor) (*ExecutorBridge, error) {
	if executor == nil {
		return nil, fmt.Errorf("executor is nil")
	}

	bridge := &ExecutorBridge{
		executor: executor,
	}

	// Get initial state
	params, hash, err := executor.LastBlock()
	if err != nil {
		return nil, fmt.Errorf("get last block: %w", err)
	}
	if params != nil {
		bridge.lastBlockIndex = params.Index
	}
	bridge.lastBlockHash = hash

	return bridge, nil
}

// ProduceBlock processes a committed certificate and produces a block.
// It extracts transactions from batches, converts them to envelopes,
// and calls the executor to process them.
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

// Validators returns the current validator set.
// Note: This requires access to the protocol's validator set,
// which would need to be provided separately.
func (b *ExecutorBridge) Validators() []ValidatorInfo {
	// TODO: Implement by querying the protocol's validator set
	// This requires access to the database/network state
	return nil
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
