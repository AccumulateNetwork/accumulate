// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package dagbft provides the DAG-BFT consensus node that replaces CometBFT ABCI.
// It processes committed certificates from Bullshark consensus and executes
// their transactions through the Accumulate executor.
package dagbft

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/bullshark"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// DAGConsensusNode is the main consensus node that replaces CometBFT ABCI.
// It coordinates the DAG-BFT consensus components and executes committed
// certificates through the Accumulate executor.
type DAGConsensusNode struct {
	// Configuration
	config NodeConfig

	// Core components
	executor  execute.Executor
	bullshark *bullshark.Bullshark
	dag       *dag.DAG
	committee *types.Committee

	// Block production
	blockProducer *BlockProducer

	// Worker for transaction batching
	worker *worker.Worker

	// State tracking
	mu             sync.RWMutex
	lastBlockIndex uint64
	lastBlockHash  [32]byte
	startTime      time.Time
	ready          atomic.Bool

	// Event bus
	eventBus *events.Bus

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NodeConfig contains configuration for the DAG consensus node.
type NodeConfig struct {
	// Partition is the network partition this node operates on.
	Partition string

	// PrivateKey is the validator's ed25519 private key.
	PrivateKey ed25519.PrivateKey

	// Logger is the structured logger for this node.
	Logger *slog.Logger

	// MaxEnvelopesPerBlock limits envelopes processed per block (0 = unlimited).
	MaxEnvelopesPerBlock int
}

// NewDAGConsensusNode creates a new DAG-BFT consensus node.
func NewDAGConsensusNode(
	config NodeConfig,
	executor execute.Executor,
	eventBus *events.Bus,
	committee *types.Committee,
	d *dag.DAG,
	w *worker.Worker,
) (*DAGConsensusNode, error) {
	if executor == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	if committee == nil {
		return nil, fmt.Errorf("committee is nil")
	}
	if d == nil {
		return nil, fmt.Errorf("dag is nil")
	}

	// Get initial state from executor
	params, hash, err := executor.LastBlock()
	if err != nil {
		return nil, fmt.Errorf("get last block: %w", err)
	}

	var lastIndex uint64
	if params != nil {
		lastIndex = params.Index
	}

	// Create Bullshark consensus
	bs := bullshark.New(committee, d)

	node := &DAGConsensusNode{
		config:         config,
		executor:       executor,
		bullshark:      bs,
		dag:            d,
		committee:      committee,
		worker:         w,
		lastBlockIndex: lastIndex,
		lastBlockHash:  hash,
		eventBus:       eventBus,
		startTime:      time.Now(),
	}

	// Create block producer
	node.blockProducer = NewBlockProducer(BlockProducerConfig{
		Executor:             executor,
		EventBus:             eventBus,
		Logger:               config.Logger,
		Partition:            config.Partition,
		MaxEnvelopesPerBlock: config.MaxEnvelopesPerBlock,
	})

	// Subscribe to validator changes
	if eventBus != nil {
		events.SubscribeSync(eventBus, node.willChangeGlobals)
	}

	return node, nil
}

// Start begins the consensus node operation.
func (n *DAGConsensusNode) Start(ctx context.Context) error {
	n.ctx, n.cancel = context.WithCancel(ctx)
	n.ready.Store(true)

	n.wg.Add(1)
	go n.processLoop()

	if n.config.Logger != nil {
		n.config.Logger.Info("DAG consensus node started",
			"partition", n.config.Partition,
			"lastBlockIndex", n.lastBlockIndex)
	}

	return nil
}

// Stop stops the consensus node.
func (n *DAGConsensusNode) Stop() error {
	if n.cancel != nil {
		n.cancel()
	}
	n.wg.Wait()

	if n.config.Logger != nil {
		n.config.Logger.Info("DAG consensus node stopped",
			"partition", n.config.Partition)
	}

	return nil
}

// IsReady returns true if the node is ready to process transactions.
func (n *DAGConsensusNode) IsReady() bool {
	return n.ready.Load()
}

// LastBlock returns the last committed block parameters and hash.
func (n *DAGConsensusNode) LastBlock() (*execute.BlockParams, [32]byte, error) {
	return n.executor.LastBlock()
}

// Submit submits a transaction to the worker for batching.
// This is the DAG-BFT equivalent of CheckTx.
func (n *DAGConsensusNode) Submit(tx []byte) error {
	if !n.ready.Load() {
		return fmt.Errorf("node not ready")
	}

	// Validate the transaction before adding to worker queue
	if err := n.blockProducer.ValidateTransaction(tx); err != nil {
		return fmt.Errorf("transaction validation failed: %w", err)
	}

	// Submit to worker for batching
	if n.worker != nil {
		return n.worker.Submit(tx)
	}

	return nil
}

// ProcessCertificate processes a certificate that has been added to the DAG.
// This checks if any leaders can be committed and processes them.
func (n *DAGConsensusNode) ProcessCertificate(cert *types.Certificate) error {
	if cert == nil {
		return nil
	}

	// Let Bullshark determine if any leaders should be committed
	outputs := n.bullshark.ProcessCertificate(cert)
	if len(outputs) == 0 {
		return nil
	}

	// Process each committed certificate
	for _, output := range outputs {
		if err := n.processCommittedCertificate(output.Certificate); err != nil {
			return fmt.Errorf("process committed certificate: %w", err)
		}
	}

	return nil
}

// processCommittedCertificate processes a certificate that has been committed.
func (n *DAGConsensusNode) processCommittedCertificate(cert *types.Certificate) error {
	if cert == nil {
		return nil
	}

	// Collect batches referenced by the certificate
	batches := make(map[types.BatchDigest]*types.Batch)
	for batchDigest := range cert.Header.Payload {
		batch, err := n.getBatch(batchDigest)
		if err != nil {
			if n.config.Logger != nil {
				n.config.Logger.Warn("Failed to get batch",
					"digest", batchDigest.String(),
					"error", err)
			}
			continue
		}
		if batch != nil {
			batches[batchDigest] = batch
		}
	}

	// Determine if this node is the leader (certificate author)
	isLeader := false
	if n.config.PrivateKey != nil {
		pubKey := n.config.PrivateKey.Public().(ed25519.PublicKey)
		isLeader = string(cert.Author()) == string(pubKey)
	}

	// Get next block index
	n.mu.Lock()
	nextIndex := n.lastBlockIndex + 1
	n.mu.Unlock()

	// Produce block
	hash, err := n.blockProducer.ProduceBlock(n.ctx, BlockParams{
		Index:       nextIndex,
		Time:        time.Now(),
		IsLeader:    isLeader,
		LeaderRound: cert.Round(),
		Certificate: cert,
		Batches:     batches,
	})
	if err != nil {
		return fmt.Errorf("produce block: %w", err)
	}

	// Update state
	n.mu.Lock()
	n.lastBlockIndex = nextIndex
	n.lastBlockHash = hash
	n.mu.Unlock()

	// Mark committed in Bullshark
	n.bullshark.MarkCommitted(cert)

	// Prune committed batches from worker
	if n.worker != nil {
		committed := make([]types.BatchDigest, 0, len(batches))
		for digest := range batches {
			committed = append(committed, digest)
		}
		n.worker.PruneBatches(committed)
	}

	return nil
}

// getBatch retrieves a batch by its digest.
func (n *DAGConsensusNode) getBatch(digest types.BatchDigest) (*types.Batch, error) {
	if n.worker != nil {
		return n.worker.GetBatch(digest)
	}
	return nil, nil
}

// processLoop is the main processing loop that handles certificate events.
func (n *DAGConsensusNode) processLoop() {
	defer n.wg.Done()

	// In a full implementation, this would subscribe to certificate events
	// from the gossip layer. For now, certificates are processed via
	// ProcessCertificate being called directly.
	<-n.ctx.Done()
}

// willChangeGlobals handles validator set changes.
func (n *DAGConsensusNode) willChangeGlobals(e events.WillChangeGlobals) error {
	// In a full implementation, this would update the committee
	// when the validator set changes. For now, this is a placeholder.
	return nil
}

// UpdateCommittee updates the validator committee.
// This should be called when the epoch changes.
func (n *DAGConsensusNode) UpdateCommittee(committee *types.Committee) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.committee = committee
	n.bullshark.UpdateCommittee(committee)
}

// GetDAG returns the certificate DAG.
func (n *DAGConsensusNode) GetDAG() *dag.DAG {
	return n.dag
}

// GetBullshark returns the Bullshark consensus instance.
func (n *DAGConsensusNode) GetBullshark() *bullshark.Bullshark {
	return n.bullshark
}

// GetWorker returns the transaction worker.
func (n *DAGConsensusNode) GetWorker() *worker.Worker {
	return n.worker
}
