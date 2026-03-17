// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
)

// BlockParams contains the parameters for block production.
type BlockParams struct {
	// Index is the block height/index.
	Index uint64

	// Time is the block timestamp.
	Time time.Time

	// IsLeader indicates if this node was the leader for this block.
	IsLeader bool

	// LeaderRound is the consensus round that committed this block.
	LeaderRound types.Round

	// Certificate is the committed certificate that triggered this block.
	Certificate *types.Certificate

	// Batches contains the transaction batches referenced by the certificate.
	// Map from batch digest to batch data.
	Batches map[types.BatchDigest]*types.Batch
}

// BlockProducerConfig contains configuration for the block producer.
type BlockProducerConfig struct {
	// Executor is the transaction executor.
	Executor execute.Executor

	// EventBus is the event bus for publishing block events.
	EventBus *events.Bus

	// Logger is the structured logger.
	Logger *slog.Logger

	// Partition is the network partition.
	Partition string

	// MaxEnvelopesPerBlock limits envelopes processed per block (0 = unlimited).
	MaxEnvelopesPerBlock int
}

// BlockProducer handles block production from committed certificates.
// It extracts transactions from batches, processes them through the executor,
// and commits the resulting block state.
type BlockProducer struct {
	config BlockProducerConfig

	mu             sync.RWMutex
	lastBlockIndex uint64
	lastBlockHash  [32]byte
	startTime      time.Time
}

// NewBlockProducer creates a new BlockProducer.
func NewBlockProducer(config BlockProducerConfig) *BlockProducer {
	return &BlockProducer{
		config:    config,
		startTime: time.Now(),
	}
}

// ProduceBlock processes a committed certificate and produces a block.
// It extracts transactions from batches, converts them to envelopes,
// and calls the executor to process them.
func (p *BlockProducer) ProduceBlock(ctx context.Context, params BlockParams) ([32]byte, error) {
	if p.config.Executor == nil {
		return [32]byte{}, fmt.Errorf("executor is nil")
	}

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
	block, err := p.config.Executor.Begin(execParams)
	if err != nil {
		return [32]byte{}, fmt.Errorf("begin block: %w", err)
	}

	// Process transactions from all batches
	txCount := 0
	envelopeCount := 0

	for digest, batch := range params.Batches {
		if batch == nil {
			if p.config.Logger != nil {
				p.config.Logger.Warn("Missing batch in certificate",
					"digest", digest.String(),
					"round", params.LeaderRound)
			}
			continue
		}

		for i, txBytes := range batch.Transactions {
			// Check envelope limit
			if p.config.MaxEnvelopesPerBlock > 0 && envelopeCount >= p.config.MaxEnvelopesPerBlock {
				if p.config.Logger != nil {
					p.config.Logger.Debug("Reached max envelopes per block",
						"limit", p.config.MaxEnvelopesPerBlock,
						"index", params.Index)
				}
				break
			}

			// Unmarshal transaction to envelope
			envelope := new(messaging.Envelope)
			if err := envelope.UnmarshalBinary(txBytes); err != nil {
				if p.config.Logger != nil {
					p.config.Logger.Debug("Failed to unmarshal transaction",
						"error", err,
						"batch", digest.String(),
						"index", i)
				}
				continue
			}

			// Process the envelope
			statuses, err := block.Process(envelope)
			if err != nil {
				if p.config.Logger != nil {
					p.config.Logger.Warn("Failed to process transaction",
						"error", err,
						"batch", digest.String(),
						"index", i)
				}
				continue
			}

			// Log any failed transactions
			for _, status := range statuses {
				if status.Error != nil {
					if p.config.Logger != nil {
						p.config.Logger.Debug("Transaction failed",
							"error", status.Error,
							"code", status.Code)
					}
				}
			}

			txCount++
			envelopeCount++
		}
	}

	// Close block
	state, err := block.Close()
	if err != nil {
		return [32]byte{}, fmt.Errorf("close block: %w", err)
	}

	// Handle empty blocks
	if state.IsEmpty() {
		state.Discard()
		// Return the previous block hash for empty blocks
		p.mu.RLock()
		hash := p.lastBlockHash
		p.mu.RUnlock()

		if p.config.Logger != nil {
			p.config.Logger.Debug("Discarded empty block",
				"index", params.Index,
				"round", params.LeaderRound)
		}
		return hash, nil
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
	p.mu.Lock()
	p.lastBlockIndex = params.Index
	p.lastBlockHash = hash
	p.mu.Unlock()

	// Publish block committed event
	if p.config.EventBus != nil {
		major, blockTime, _ := state.DidCompleteMajorBlock()
		if blockTime.IsZero() {
			blockTime = params.Time
		}
		err = p.config.EventBus.Publish(events.DidCommitBlock{
			Index: params.Index,
			Time:  blockTime,
			Major: major,
		})
		if err != nil {
			if p.config.Logger != nil {
				p.config.Logger.Warn("Failed to publish block event",
					"error", err,
					"index", params.Index)
			}
		}
	}

	if p.config.Logger != nil {
		p.config.Logger.Debug("Produced block",
			"index", params.Index,
			"round", params.LeaderRound,
			"transactions", txCount,
			"hash", fmt.Sprintf("%x", hash[:8]))
	}

	return hash, nil
}

// ValidateTransaction validates a transaction before it is added to a batch.
// This is the DAG-BFT equivalent of ABCI CheckTx.
func (p *BlockProducer) ValidateTransaction(tx []byte) error {
	if p.config.Executor == nil {
		return fmt.Errorf("executor is nil")
	}

	// Unmarshal to envelope
	envelope := new(messaging.Envelope)
	if err := envelope.UnmarshalBinary(tx); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}

	// Validate using executor
	statuses, err := p.config.Executor.Validate(envelope, false)
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
func (p *BlockProducer) LastBlock() (uint64, [32]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastBlockIndex, p.lastBlockHash, nil
}

// StateHash returns the current state hash (same as last block hash).
func (p *BlockProducer) StateHash() [32]byte {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastBlockHash
}
