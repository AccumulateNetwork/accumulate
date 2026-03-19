// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/genesis"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// forceGC runs the garbage collector and waits for finalizers
func forceGC() {
	runtime.GC()
	runtime.GC()
}

// getAllocatedMemory returns the currently allocated heap memory
func getAllocatedMemory() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

func TestDataTx_MarshalUnmarshal(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	tx := NewDataTx(pub, []byte("hello world"), 1)
	tx.Sign(priv)

	data := tx.Marshal()
	tx2, err := UnmarshalTransaction(data)
	require.NoError(t, err)

	dataTx, ok := tx2.(*DataTx)
	require.True(t, ok)

	assert.Equal(t, tx.Hash(), dataTx.Hash())
	assert.True(t, dataTx.Verify())
}

func TestSetBlockTimeTx_MarshalUnmarshal(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	tx := NewSetBlockTimeTx(pub, 5*time.Second, 1)
	tx.Sign(priv)

	data := tx.Marshal()
	tx2, err := UnmarshalTransaction(data)
	require.NoError(t, err)

	blockTx, ok := tx2.(*SetBlockTimeTx)
	require.True(t, ok)

	assert.Equal(t, tx.Hash(), blockTx.Hash())
	assert.Equal(t, 5*time.Second, blockTx.BlockInterval)
	assert.True(t, blockTx.Verify())
}

func TestSetTxRateTx_MarshalUnmarshal(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	tx := NewSetTxRateTx(pub, 500, 1)
	tx.Sign(priv)

	data := tx.Marshal()
	tx2, err := UnmarshalTransaction(data)
	require.NoError(t, err)

	rateTx, ok := tx2.(*SetTxRateTx)
	require.True(t, ok)

	assert.Equal(t, tx.Hash(), rateTx.Hash())
	assert.Equal(t, uint32(500), rateTx.TxPerSecond)
	assert.True(t, rateTx.Verify())
}

func TestBlock_MarshalUnmarshal(t *testing.T) {
	block := &Block{
		Height:    42,
		PrevHash:  [32]byte{1, 2, 3},
		Timestamp: time.Now().Truncate(time.Nanosecond),
		TxnCount:  100,
		TxnsHash:  [32]byte{4, 5, 6},
		StateHash: [32]byte{7, 8, 9},
	}

	data := block.Marshal()
	block2, err := UnmarshalBlock(data)
	require.NoError(t, err)

	assert.Equal(t, block.Height, block2.Height)
	assert.Equal(t, block.PrevHash, block2.PrevHash)
	assert.Equal(t, block.Timestamp, block2.Timestamp)
	assert.Equal(t, block.TxnCount, block2.TxnCount)
	assert.Equal(t, block.TxnsHash, block2.TxnsHash)
	assert.Equal(t, block.StateHash, block2.StateHash)
}

func TestExecutor_ProcessTransaction(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	executor, err := NewExecutor(ExecutorConfig{
		Validators:    []ed25519.PublicKey{pub},
		BlockInterval: 1 * time.Second,
		TxRate:        100,
	})
	require.NoError(t, err)

	// Process a data transaction
	tx := NewDataTx(pub, []byte("test data"), 1)
	tx.Sign(priv)

	err = executor.ProcessTransaction(tx.Marshal())
	require.NoError(t, err)

	assert.Equal(t, uint64(1), executor.GetProcessedCount())
}

func TestExecutor_SetBlockTime(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	executor, err := NewExecutor(ExecutorConfig{
		Validators:    []ed25519.PublicKey{pub},
		BlockInterval: 1 * time.Second,
		TxRate:        100,
	})
	require.NoError(t, err)

	assert.Equal(t, 1*time.Second, executor.GetBlockInterval())

	// Submit SetBlockTime transaction
	tx := NewSetBlockTimeTx(pub, 5*time.Second, 1)
	tx.Sign(priv)

	err = executor.ProcessTransaction(tx.Marshal())
	require.NoError(t, err)

	assert.Equal(t, 5*time.Second, executor.GetBlockInterval())
	assert.Equal(t, uint64(1), executor.GetProcessedCount())
}

func TestExecutor_SetBlockTime_NonValidator(t *testing.T) {
	validatorPub, _, _ := ed25519.GenerateKey(rand.Reader)
	userPub, userPriv, _ := ed25519.GenerateKey(rand.Reader)

	executor, err := NewExecutor(ExecutorConfig{
		Validators:    []ed25519.PublicKey{validatorPub},
		BlockInterval: 1 * time.Second,
		TxRate:        100,
	})
	require.NoError(t, err)

	// Non-validator tries to change block time
	tx := NewSetBlockTimeTx(userPub, 10*time.Second, 1)
	tx.Sign(userPriv)

	err = executor.ProcessTransaction(tx.Marshal())
	require.NoError(t, err) // No error, but change is rejected

	// Block time should be unchanged
	assert.Equal(t, 1*time.Second, executor.GetBlockInterval())
	assert.Equal(t, uint64(0), executor.GetProcessedCount())
}

func TestExecutor_BlockProduction(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	executor, err := NewExecutor(ExecutorConfig{
		Validators:    []ed25519.PublicKey{pub},
		BlockInterval: 100 * time.Millisecond,
		TxRate:        100,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var blocksProduced atomic.Int32
	executor.SetOnBlockProduced(func(b *Block) {
		blocksProduced.Add(1)
	})

	// Submit some transactions
	for i := 0; i < 10; i++ {
		tx := NewDataTx(pub, []byte("test"), uint64(i+1))
		tx.Sign(priv)
		_ = executor.ProcessTransaction(tx.Marshal())
	}

	executor.Start(ctx)
	time.Sleep(500 * time.Millisecond)
	executor.Stop()

	// Should have produced at least a few blocks
	assert.Greater(t, blocksProduced.Load(), int32(0))
	assert.Greater(t, executor.GetBlockCount(), uint64(1)) // genesis + at least one
}

func TestExecutor_ReplayProtection(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	executor, err := NewExecutor(ExecutorConfig{
		Validators:    []ed25519.PublicKey{pub},
		BlockInterval: 1 * time.Second,
		TxRate:        100,
	})
	require.NoError(t, err)

	// Submit transaction with nonce 1
	tx1 := NewDataTx(pub, []byte("first"), 1)
	tx1.Sign(priv)
	err = executor.ProcessTransaction(tx1.Marshal())
	require.NoError(t, err)
	assert.Equal(t, uint64(1), executor.GetProcessedCount())

	// Try to replay the same transaction
	err = executor.ProcessTransaction(tx1.Marshal())
	require.NoError(t, err)
	assert.Equal(t, uint64(1), executor.GetProcessedCount()) // Still 1

	// Submit with higher nonce should work
	tx2 := NewDataTx(pub, []byte("second"), 2)
	tx2.Sign(priv)
	err = executor.ProcessTransaction(tx2.Marshal())
	require.NoError(t, err)
	assert.Equal(t, uint64(2), executor.GetProcessedCount())
}

func TestGenesisBlock(t *testing.T) {
	genesis := GenesisBlock()

	assert.Equal(t, uint64(0), genesis.Height)
	assert.Equal(t, [32]byte{}, genesis.PrevHash)
	assert.Equal(t, uint32(0), genesis.TxnCount)
}

// TestSingleNodeBlockProduction_ADI tests that a single node produces blocks with full consensus flow.
// This is part of DAG-BFT Deploy Step 2: verifying single node block production.
// The test uses a complete consensus node to validate that:
// 1. Transactions are submitted to the consensus node
// 2. Consensus produces certificates and commits them
// 3. Committed certificates trigger block production
func TestSingleNodeBlockProduction_ADI(t *testing.T) {
	const testDuration = 10 * time.Second
	const minExpectedBlocks = 5 // With single node consensus, expect fewer blocks

	ctx, cancel := context.WithTimeout(context.Background(), testDuration+5*time.Second)
	defer cancel()

	// Generate validator key
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Create committee with single validator
	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 100},
	}, 1)

	// Create consensus node (no libp2p for single node test)
	nodeCfg := consensus.NodeConfig{
		Partition:  "test",
		KeyPair:    priv,
		NumWorkers: 1,
	}
	node, err := consensus.NewNode(nodeCfg, committee, nil, nil)
	require.NoError(t, err)

	// Initialize genesis
	err = node.InitGenesis([]genesis.ValidatorInfo{
		{PublicKey: pub, PrivateKey: priv, Stake: 100},
	})
	require.NoError(t, err)

	// Create executor
	executor, err := NewExecutor(ExecutorConfig{
		Validators: []ed25519.PublicKey{pub},
		TxRate:     100,
		DataDir:    "/tmp/consensus-testnet-single-adi",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = executor.Cleanup() })

	// Track blocks produced and state hashes
	var blocksProduced atomic.Int32
	var stateHashes sync.Map

	executor.SetOnBlockProduced(func(b *Block) {
		blocksProduced.Add(1)
		stateHashes.Store(b.Height, b.StateHash)
	})

	// Start executor
	executor.Start(ctx)

	// Start consensus node
	err = node.Start(ctx)
	require.NoError(t, err)

	// Process committed certificates in background
	go func() {
		committed := node.Committed()
		workers := node.Workers()
		for {
			select {
			case <-ctx.Done():
				return
			case cert := <-committed:
				if cert == nil {
					continue
				}
				// Get batches for this certificate from workers
				batches := make(map[types.BatchDigest]*types.Batch)
				digests := make([]types.BatchDigest, 0, len(cert.Header.Payload))
				for digest := range cert.Header.Payload {
					digests = append(digests, digest)
					for _, w := range workers {
						if batch, err := w.GetBatch(digest); err == nil && batch != nil {
							batches[digest] = batch
							break
						}
					}
				}
				executor.ProcessCertificate(cert, batches)
				// Prune batches after processing
				for _, w := range workers {
					w.PruneBatches(digests)
				}
			}
		}
	}()

	// Submit transactions through the consensus node
	go func() {
		nonce := uint64(0)
		ticker := time.NewTicker(50 * time.Millisecond) // Fast for single node
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				nonce++
				tx := NewDataTx(pub, []byte("test-data"), nonce)
				tx.Sign(priv)
				_ = node.SubmitTransaction(tx.Marshal())
			}
		}
	}()

	// Let it run for the test duration
	time.Sleep(testDuration)
	node.Stop()
	executor.Stop()

	// Verify acceptance criteria
	finalBlockCount := blocksProduced.Load()
	t.Logf("Blocks produced: %d (expected >= %d)", finalBlockCount, minExpectedBlocks)
	t.Logf("Current round: %d", node.CurrentRound())
	t.Logf("Last commit round: %d", node.LastCommitRound())
	txSubmitted, certsCommitted := node.Metrics()
	t.Logf("Transactions submitted: %d, Certificates committed: %d", txSubmitted, certsCommitted)

	// 1. Blocks produced from consensus commits
	assert.GreaterOrEqual(t, finalBlockCount, int32(minExpectedBlocks),
		"Should have produced at least %d blocks in %v", minExpectedBlocks, testDuration)

	// 2. Logs show block height increasing
	latestBlock := executor.GetLatestBlock()
	assert.Greater(t, latestBlock.Height, uint64(0), "Block height should be greater than 0")
	t.Logf("Latest block height: %d", latestBlock.Height)

	// 3. State hash changes each block (when transactions are included)
	uniqueHashes := make(map[[32]byte]bool)
	stateHashes.Range(func(key, value interface{}) bool {
		hash := value.([32]byte)
		uniqueHashes[hash] = true
		return true
	})
	t.Logf("Unique state hashes: %d", len(uniqueHashes))
	assert.Greater(t, len(uniqueHashes), 1, "State hash should change as transactions are processed")

	// Verify processed transactions
	processedCount := executor.GetProcessedCount()
	t.Logf("Processed transactions: %d", processedCount)
	assert.Greater(t, processedCount, uint64(0), "Should have processed some transactions")
}

// TestSingleNodeBlockProduction_MemoryStability tests that a single node doesn't leak memory
// over multiple block production cycles. This is part of DAG-BFT Deploy Step 2.
func TestSingleNodeBlockProduction_MemoryStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory stability test in short mode")
	}

	const testDuration = 30 * time.Second
	const memoryGrowthThreshold = 50 * 1024 * 1024 // 50MB growth allowed

	ctx, cancel := context.WithTimeout(context.Background(), testDuration+10*time.Second)
	defer cancel()

	// Generate validator key
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Create committee with single validator
	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 100},
	}, 1)

	// Create consensus node
	nodeCfg := consensus.NodeConfig{
		Partition:  "test-memory",
		KeyPair:    priv,
		NumWorkers: 1,
	}
	node, err := consensus.NewNode(nodeCfg, committee, nil, nil)
	require.NoError(t, err)

	// Initialize genesis
	err = node.InitGenesis([]genesis.ValidatorInfo{
		{PublicKey: pub, PrivateKey: priv, Stake: 100},
	})
	require.NoError(t, err)

	// Create executor
	executor, err := NewExecutor(ExecutorConfig{
		Validators: []ed25519.PublicKey{pub},
		TxRate:     100,
		DataDir:    "/tmp/consensus-testnet-memory",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = executor.Cleanup() })

	// Start executor and consensus node
	executor.Start(ctx)
	err = node.Start(ctx)
	require.NoError(t, err)

	// Process committed certificates in background
	go func() {
		committed := node.Committed()
		workers := node.Workers()
		for {
			select {
			case <-ctx.Done():
				return
			case cert := <-committed:
				if cert == nil {
					continue
				}
				batches := make(map[types.BatchDigest]*types.Batch)
				digests := make([]types.BatchDigest, 0, len(cert.Header.Payload))
				for digest := range cert.Header.Payload {
					digests = append(digests, digest)
					for _, w := range workers {
						if batch, err := w.GetBatch(digest); err == nil && batch != nil {
							batches[digest] = batch
							break
						}
					}
				}
				executor.ProcessCertificate(cert, batches)
				for _, w := range workers {
					w.PruneBatches(digests)
				}
			}
		}
	}()

	// Force GC and get baseline memory
	forceGC()
	baselineMemory := getAllocatedMemory()
	t.Logf("Baseline memory: %d bytes", baselineMemory)

	// Submit transactions at a steady rate
	var submitted atomic.Uint64
	go func() {
		payload := make([]byte, 256)
		_, _ = rand.Read(payload)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				nonce := submitted.Add(1)
				tx := NewDataTx(pub, payload, nonce)
				tx.Sign(priv)
				_ = node.SubmitTransaction(tx.Marshal())
			}
		}
	}()

	// Sample memory periodically
	var memSamples []uint64
	sampleTicker := time.NewTicker(5 * time.Second)
	defer sampleTicker.Stop()

	testTimer := time.NewTimer(testDuration)
	defer testTimer.Stop()

	for {
		select {
		case <-testTimer.C:
			goto done
		case <-sampleTicker.C:
			forceGC()
			currentMem := getAllocatedMemory()
			memSamples = append(memSamples, currentMem)
			t.Logf("Memory sample: %d bytes (blocks: %d, processed: %d)",
				currentMem, executor.GetBlockCount(), executor.GetProcessedCount())
		}
	}

done:
	_ = memSamples // Samples logged during collection; slice kept for potential future analysis
	node.Stop()
	executor.Stop()

	// Force final GC and get final memory
	forceGC()
	finalMemory := getAllocatedMemory()
	memoryGrowth := int64(finalMemory) - int64(baselineMemory)

	t.Logf("Final memory: %d bytes", finalMemory)
	t.Logf("Memory growth: %d bytes", memoryGrowth)
	t.Logf("Blocks produced: %d", executor.GetBlockCount())
	t.Logf("Transactions submitted: %d", submitted.Load())
	t.Logf("Transactions processed: %d", executor.GetProcessedCount())

	// Check that memory growth is within acceptable bounds
	// Note: Some growth is expected due to test framework overhead
	assert.Less(t, memoryGrowth, int64(memoryGrowthThreshold),
		"Memory growth should be less than %d bytes, got %d bytes",
		memoryGrowthThreshold, memoryGrowth)

	// Verify the system was actually doing work
	assert.Greater(t, executor.GetBlockCount(), uint64(100),
		"Should have produced significant number of blocks")
	assert.Greater(t, executor.GetProcessedCount(), uint64(100),
		"Should have processed significant number of transactions")
}

func TestComputeTxnsHash(t *testing.T) {
	// Empty list
	hash1 := ComputeTxnsHash(nil)
	hash2 := ComputeTxnsHash([][32]byte{})
	assert.Equal(t, hash1, hash2)

	// Single transaction
	txHash := [32]byte{1, 2, 3}
	hash3 := ComputeTxnsHash([][32]byte{txHash})
	assert.NotEqual(t, hash1, hash3)

	// Order matters
	txHash2 := [32]byte{4, 5, 6}
	hash4 := ComputeTxnsHash([][32]byte{txHash, txHash2})
	hash5 := ComputeTxnsHash([][32]byte{txHash2, txHash})
	assert.NotEqual(t, hash4, hash5)
}
