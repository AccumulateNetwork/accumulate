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

	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
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
	validatorVersion       uint64
	validatorChangeHandler func(validators []ValidatorInfo, version uint64)
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

	version := globals.Network.Version

	b.mu.Lock()
	oldValidators := b.validators
	oldVersion := b.validatorVersion
	b.validators = validators
	b.validatorVersion = version
	handler := b.validatorChangeHandler
	b.mu.Unlock()

	// Notify on a version change even when the validator set is unchanged:
	// the committee epoch tracks the network definition version, and every
	// node must bump it at the same block or a node that joins at a later
	// version can never match the epoch of nodes that skipped the bump.
	if handler != nil && (version != oldVersion || !validatorSetsEqual(oldValidators, validators)) {
		slog.Info("Validator set changed",
			"partition", b.partitionID,
			"oldCount", len(oldValidators),
			"newCount", len(validators),
			"version", version)
		handler(validators, version)
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
func (b *ExecutorBridge) SetValidators(validators []ValidatorInfo, version uint64) {
	b.mu.Lock()
	oldValidators := b.validators
	oldVersion := b.validatorVersion
	b.validators = make([]ValidatorInfo, len(validators))
	copy(b.validators, validators)
	b.validatorVersion = version
	handler := b.validatorChangeHandler
	b.mu.Unlock()

	// Notify handler if validators changed
	if handler != nil && (version != oldVersion || !validatorSetsEqual(oldValidators, validators)) {
		handler(validators, version)
	}
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

	// Process transactions from all batches. Batches arrive in the
	// certificate's canonical payload order and MUST be executed in that
	// order — executing in any node-local order (this used to iterate a map)
	// diverges chain entries and BPT roots across validators (#4054).
	txCount := 0
	for _, batch := range params.Batches {
		if batch == nil {
			slog.Warn("Missing batch in certificate",
				"round", params.LeaderRound)
			continue
		}
		digest := batch.Digest()

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

	// Validator changes are handled via the WillChangeGlobals event (which
	// fires during block execution with the FULL new set and the network
	// definition version), not via DidUpdateValidators: that returns a DIFF —
	// added and removed keys only — and building a committee from a diff
	// replaces the whole validator set with just the change.
	if _, changed := state.DidUpdateValidators(); changed {
		slog.Debug("Block updated validators", "index", params.Index)
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
func (b *ExecutorBridge) OnValidatorSetChange(callback func(validators []ValidatorInfo, version uint64)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.validatorChangeHandler = callback
}

// Ensure ExecutorBridge implements ConsensusAdapter
var _ ConsensusAdapter = (*ExecutorBridge)(nil)
