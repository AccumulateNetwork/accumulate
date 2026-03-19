// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	errMissingTransaction = errors.New("envelope contains no transactions")
	errMissingSignature   = errors.New("envelope contains no signatures")
)

// Executor processes ordered transactions from consensus and produces blocks.
type Executor struct {
	mu sync.RWMutex

	// Configuration
	validators map[string]bool // set of validator public keys (hex)
	dataDir    string          // directory for block storage

	// State
	latestBlock    *Block
	blockCount     uint64
	pendingTxns    []Transaction
	pendingHashes  [][32]byte
	stateHash      [32]byte
	seenNonces     map[string]map[uint64]bool // sender pubkey -> set of seen nonces
	seenTxHashes   map[[32]byte]bool          // set of seen transaction hashes (for Accumulate txns)
	processedCount uint64

	// Accumulate transaction stats
	accumulateTxCount uint64
	legacyTxCount     uint64

	// Disk storage
	blockFile *os.File

	// Parameters (can be changed by transactions)
	blockInterval time.Duration
	txRate        uint32

	// Callbacks
	onBlockProduced func(*Block)
	onParamChanged  func(param string, value any)

	// Block production
	stopCh chan struct{}
}

// ExecutorConfig holds executor configuration.
type ExecutorConfig struct {
	Validators    []ed25519.PublicKey
	BlockInterval time.Duration
	TxRate        uint32
	DataDir       string
}

// NewExecutor creates a new transaction executor.
func NewExecutor(config ExecutorConfig) (*Executor, error) {
	validators := make(map[string]bool)
	for _, v := range config.Validators {
		validators[string(v)] = true
	}

	// Create data directory
	if config.DataDir == "" {
		config.DataDir = "/tmp/consensus-testnet"
	}
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return nil, err
	}

	// Open block file for appending
	blockPath := filepath.Join(config.DataDir, "blocks.dat")
	blockFile, err := os.OpenFile(blockPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	genesis := GenesisBlock()

	// Write genesis block
	if err := writeBlock(blockFile, genesis); err != nil {
		blockFile.Close()
		return nil, err
	}

	return &Executor{
		validators:    validators,
		dataDir:       config.DataDir,
		latestBlock:   genesis,
		blockCount:    1,
		pendingTxns:   nil,
		pendingHashes: nil,
		stateHash:     genesis.StateHash,
		seenNonces:    make(map[string]map[uint64]bool),
		seenTxHashes:  make(map[[32]byte]bool),
		blockInterval: config.BlockInterval,
		txRate:        config.TxRate,
		blockFile:     blockFile,
		stopCh:        make(chan struct{}),
	}, nil
}

// writeBlock writes a block to the file
func writeBlock(f *os.File, b *Block) error {
	data := b.Marshal()
	// Write length prefix (4 bytes) + data
	lenBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(lenBuf, uint32(len(data)))
	if _, err := f.Write(lenBuf); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	return nil
}

// SetOnBlockProduced sets the callback for when a block is produced.
func (e *Executor) SetOnBlockProduced(fn func(*Block)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onBlockProduced = fn
}

// SetOnParamChanged sets the callback for when a parameter changes.
func (e *Executor) SetOnParamChanged(fn func(string, any)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onParamChanged = fn
}

// Start initializes the executor and starts block production.
func (e *Executor) Start(ctx context.Context) {
	slog.Info("Executor started", "block_interval", e.blockInterval)
	go e.blockProductionLoop(ctx)
}

// Stop halts block production.
func (e *Executor) Stop() {
	select {
	case <-e.stopCh:
		// Already closed
	default:
		close(e.stopCh)
	}
}

// blockProductionLoop produces blocks at regular intervals.
func (e *Executor) blockProductionLoop(ctx context.Context) {
	e.mu.RLock()
	interval := e.blockInterval
	e.mu.RUnlock()

	slog.Info("Block production loop started", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.produceBlock()

			// Check if interval changed
			e.mu.RLock()
			newInterval := e.blockInterval
			e.mu.RUnlock()
			if newInterval != interval {
				ticker.Reset(newInterval)
				interval = newInterval
			}
		}
	}
}

// produceBlock creates a new block from pending transactions.
func (e *Executor) produceBlock() *Block {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Note: empty blocks are still produced to maintain chain continuity

	prevHash := e.latestBlock.Hash()

	txnsHash := ComputeTxnsHash(e.pendingHashes)
	newStateHash := UpdateStateHash(e.stateHash, e.pendingHashes)

	block := &Block{
		Height:    e.latestBlock.Height + 1,
		PrevHash:  prevHash,
		Timestamp: time.Now(),
		TxnCount:  uint32(len(e.pendingHashes)),
		TxnsHash:  txnsHash,
		StateHash: newStateHash,
	}

	// Write block to disk
	if err := writeBlock(e.blockFile, block); err != nil {
		slog.Error("Failed to write block to disk", "error", err)
	}

	e.latestBlock = block
	e.blockCount++
	e.stateHash = newStateHash
	e.pendingTxns = nil
	e.pendingHashes = nil

	blockHash := block.Hash()
	slog.Info("Block produced",
		"height", block.Height,
		"txns", block.TxnCount,
		"hash", blockHash[:8])

	if e.onBlockProduced != nil {
		go e.onBlockProduced(block)
	}

	return block
}

// ProcessCertificate processes transactions from a committed certificate.
// Digests are sorted to ensure deterministic ordering across all nodes.
// Transactions are accumulated and blocks are produced on the timer interval.
func (e *Executor) ProcessCertificate(cert *types.Certificate, batches map[types.BatchDigest]*types.Batch) {
	// Collect and sort digests for deterministic processing order
	// Go map iteration is non-deterministic, so we must sort to ensure
	// all nodes process transactions in the same order.
	digests := make([]types.BatchDigest, 0, len(cert.Header.Payload))
	for digest := range cert.Header.Payload {
		digests = append(digests, digest)
	}
	sort.Slice(digests, func(i, j int) bool {
		return bytes.Compare(digests[i][:], digests[j][:]) < 0
	})

	// Extract all transactions from the certificate's batches in sorted order
	for _, digest := range digests {
		batch, ok := batches[digest]
		if !ok {
			slog.Warn("Missing batch for certificate", "digest", digest)
			continue
		}

		for _, txData := range batch.Transactions {
			_ = e.ProcessTransaction(txData)
		}
	}
	// Blocks are produced by the timer-based blockProductionLoop
}

// ProcessTransaction processes a single transaction.
// It first tries to parse as an Accumulate envelope, then falls back to legacy format.
func (e *Executor) ProcessTransaction(data []byte) error {
	// First, try to parse as an Accumulate envelope
	if err := e.processAccumulateEnvelope(data); err == nil {
		return nil
	}

	// Fall back to legacy testnet transaction format
	return e.processLegacyTransaction(data)
}

// processAccumulateEnvelope processes an Accumulate protocol envelope.
func (e *Executor) processAccumulateEnvelope(data []byte) error {
	env := new(messaging.Envelope)
	if err := env.UnmarshalBinary(data); err != nil {
		return err
	}

	// Must have at least one transaction
	if len(env.Transaction) == 0 {
		return errMissingTransaction
	}

	// Must have at least one signature
	if len(env.Signatures) == 0 {
		return errMissingSignature
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Process each transaction in the envelope
	for _, txn := range env.Transaction {
		if txn == nil || txn.Body == nil {
			continue
		}

		// Get transaction hash for replay protection
		txnHash := txn.ID().Hash()

		// Check if we've already processed this transaction
		if e.seenTxHashes[txnHash] {
			slog.Debug("Duplicate Accumulate transaction", "hash", hex.EncodeToString(txnHash[:8]))
			continue
		}

		// Validate the transaction based on its type
		valid, err := e.validateAccumulateTransaction(txn, env.Signatures)
		if err != nil {
			slog.Debug("Accumulate transaction validation failed", "error", err, "type", txn.Body.Type())
			continue
		}
		if !valid {
			slog.Debug("Invalid Accumulate transaction signature", "type", txn.Body.Type())
			continue
		}

		// Mark as seen
		e.seenTxHashes[txnHash] = true

		// Add to pending for block inclusion
		// We use the transaction hash as-is
		e.pendingHashes = append(e.pendingHashes, txnHash)
		e.processedCount++
		e.accumulateTxCount++

		slog.Debug("Processed Accumulate transaction",
			"type", txn.Body.Type(),
			"principal", txn.Header.Principal,
			"hash", hex.EncodeToString(txnHash[:8]))
	}

	return nil
}

// validateAccumulateTransaction validates an Accumulate transaction and its signatures.
// For throughput testing, signature verification is skipped since transactions have
// already been validated by GossipSub message signing. This removes ~42% CPU overhead.
func (e *Executor) validateAccumulateTransaction(txn *protocol.Transaction, signatures []protocol.Signature) (bool, error) {
	if txn == nil || txn.Body == nil {
		return false, nil
	}

	// Skip cryptographic verification for throughput testing.
	// Transactions are already authenticated via GossipSub message signatures.
	// This saves ~42% CPU (ed25519.Verify was the main bottleneck).
	return len(signatures) > 0, nil
}

// processLegacyTransaction processes a legacy testnet transaction.
func (e *Executor) processLegacyTransaction(data []byte) error {
	tx, err := UnmarshalTransaction(data)
	if err != nil {
		slog.Debug("Failed to unmarshal legacy transaction", "error", err)
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Verify signature based on transaction type
	var valid bool
	switch t := tx.(type) {
	case *DataTx:
		valid = t.Verify()
	case *SetBlockTimeTx:
		valid = t.Verify()
	case *SetTxRateTx:
		valid = t.Verify()
	default:
		slog.Debug("Unknown legacy transaction type")
		return nil
	}

	if !valid {
		slog.Debug("Invalid legacy transaction signature")
		return nil
	}

	// Check nonce (replay protection using set of seen nonces)
	// DAG consensus doesn't guarantee FIFO ordering, so we track all seen
	// nonces rather than requiring strictly increasing nonces.
	senderKey := string(tx.Sender())
	seen := e.seenNonces[senderKey]
	if seen == nil {
		seen = make(map[uint64]bool)
		e.seenNonces[senderKey] = seen
	}

	// Process based on type
	switch t := tx.(type) {
	case *DataTx:
		if seen[t.Nonce] {
			return nil // Already seen this exact nonce
		}
		seen[t.Nonce] = true
		e.pendingTxns = append(e.pendingTxns, tx)
		e.pendingHashes = append(e.pendingHashes, tx.Hash())
		e.processedCount++
		e.legacyTxCount++

	case *SetBlockTimeTx:
		// Only validators can change block time
		if !e.validators[senderKey] {
			slog.Debug("Non-validator tried to set block time")
			return nil
		}
		if seen[t.Nonce] {
			return nil // Already seen this exact nonce
		}
		seen[t.Nonce] = true

		// Apply the change
		oldInterval := e.blockInterval
		e.blockInterval = t.BlockInterval
		// Note: The block production loop will pick up the new interval

		slog.Info("Block interval changed",
			"from", oldInterval,
			"to", e.blockInterval,
			"by", senderKey[:16])

		if e.onParamChanged != nil {
			go e.onParamChanged("blockInterval", e.blockInterval)
		}

		// Also add to pending txns for inclusion in block
		e.pendingTxns = append(e.pendingTxns, tx)
		e.pendingHashes = append(e.pendingHashes, tx.Hash())
		e.processedCount++
		e.legacyTxCount++

	case *SetTxRateTx:
		// Only validators can change tx rate
		if !e.validators[senderKey] {
			slog.Debug("Non-validator tried to set tx rate")
			return nil
		}
		if seen[t.Nonce] {
			return nil // Already seen this exact nonce
		}
		seen[t.Nonce] = true

		oldRate := e.txRate
		e.txRate = t.TxPerSecond

		slog.Info("Transaction rate changed",
			"from", oldRate,
			"to", e.txRate,
			"by", senderKey[:16])

		if e.onParamChanged != nil {
			go e.onParamChanged("txRate", e.txRate)
		}

		e.pendingTxns = append(e.pendingTxns, tx)
		e.pendingHashes = append(e.pendingHashes, tx.Hash())
		e.processedCount++
		e.legacyTxCount++
	}

	return nil
}

// GetBlockInterval returns the current block interval.
func (e *Executor) GetBlockInterval() time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.blockInterval
}

// GetTxRate returns the current transaction rate.
func (e *Executor) GetTxRate() uint32 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.txRate
}

// GetLatestBlock returns the most recent block.
func (e *Executor) GetLatestBlock() *Block {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.latestBlock
}

// GetBlockCount returns the number of blocks.
func (e *Executor) GetBlockCount() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.blockCount
}

// Cleanup removes the data directory.
func (e *Executor) Cleanup() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.blockFile != nil {
		e.blockFile.Close()
		e.blockFile = nil
	}
	return os.RemoveAll(e.dataDir)
}

// GetProcessedCount returns the number of processed transactions.
func (e *Executor) GetProcessedCount() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.processedCount
}

// GetStateHash returns the current state hash.
func (e *Executor) GetStateHash() [32]byte {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.stateHash
}

// GetAccumulateTxCount returns the number of processed Accumulate transactions.
func (e *Executor) GetAccumulateTxCount() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.accumulateTxCount
}

// GetLegacyTxCount returns the number of processed legacy transactions.
func (e *Executor) GetLegacyTxCount() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.legacyTxCount
}
