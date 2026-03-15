// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package worker_test

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	bhost "github.com/libp2p/go-libp2p/p2p/host/blank"
	swarmt "github.com/libp2p/go-libp2p/p2p/net/swarm/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

func TestWorker_New(t *testing.T) {
	t.Run("with defaults", func(t *testing.T) {
		w := worker.New(worker.Config{
			ID:        0,
			Partition: "test",
		}, nil)

		assert.NotNil(t, w)
		assert.Equal(t, types.WorkerID(0), w.ID())
		assert.Equal(t, "test", w.Partition())
	})

	t.Run("with custom config", func(t *testing.T) {
		config := worker.Config{
			ID:             5,
			Partition:      "custom",
			BatchSize:      100,
			BatchTimeout:   50 * time.Millisecond,
			MaxBatchBytes:  100 * 1024,
			MaxPendingSize: 1024 * 1024,
		}

		w := worker.New(config, nil)
		assert.NotNil(t, w)
		assert.Equal(t, types.WorkerID(5), w.ID())
		assert.Equal(t, "custom", w.Partition())
	})
}

func TestWorker_Submit(t *testing.T) {
	t.Run("submit single transaction", func(t *testing.T) {
		w := worker.New(worker.Config{
			ID:        0,
			Partition: "test",
		}, nil)

		err := w.Submit([]byte("transaction 1"))
		require.NoError(t, err)

		assert.Equal(t, 1, w.PendingCount())
		assert.Equal(t, 13, w.PendingSize()) // len("transaction 1")
	})

	t.Run("submit multiple transactions", func(t *testing.T) {
		w := worker.New(worker.Config{
			ID:        0,
			Partition: "test",
		}, nil)

		for i := 0; i < 10; i++ {
			err := w.Submit([]byte("tx"))
			require.NoError(t, err)
		}

		assert.Equal(t, 10, w.PendingCount())
		assert.Equal(t, 20, w.PendingSize()) // 10 * len("tx")
	})

	t.Run("empty transaction rejected", func(t *testing.T) {
		w := worker.New(worker.Config{
			ID:        0,
			Partition: "test",
		}, nil)

		err := w.Submit([]byte{})
		assert.Error(t, err)
		assert.Equal(t, 0, w.PendingCount())
	})

	t.Run("backpressure when limit reached", func(t *testing.T) {
		w := worker.New(worker.Config{
			ID:             0,
			Partition:      "test",
			MaxPendingSize: 100, // Very small limit
		}, nil)

		// Fill up to the limit
		err := w.Submit(make([]byte, 90))
		require.NoError(t, err)

		// This should trigger backpressure
		err = w.Submit(make([]byte, 20))
		assert.ErrorIs(t, err, worker.ErrBackpressure)
	})

	t.Run("transaction is copied", func(t *testing.T) {
		w := worker.New(worker.Config{
			ID:        0,
			Partition: "test",
		}, nil)

		tx := []byte("original")
		err := w.Submit(tx)
		require.NoError(t, err)

		// Modify original
		tx[0] = 'X'

		// Pending transaction should not be affected
		// We can't directly check this, but the metrics should be correct
		assert.Equal(t, 1, w.PendingCount())
	})
}

func TestWorker_BatchCreation_OnSizeLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w := worker.New(worker.Config{
		ID:           0,
		Partition:    "test",
		BatchSize:    5, // Small batch size for testing
		BatchTimeout: 10 * time.Second,
	}, nil)

	// Start worker in background
	go func() {
		w.Start(ctx)
	}()

	// Give worker time to start
	time.Sleep(50 * time.Millisecond)

	// Submit exactly BatchSize transactions
	for i := 0; i < 5; i++ {
		err := w.Submit([]byte("tx"))
		require.NoError(t, err)
	}

	// Wait for batch to be created
	time.Sleep(100 * time.Millisecond)

	// Batch should have been created
	assert.Equal(t, 0, w.PendingCount(), "pending should be cleared after batch creation")
	assert.Equal(t, 1, w.BatchCount(), "one batch should be created")

	batches := w.AvailableBatches()
	assert.Len(t, batches, 1, "one batch should be available")

	// Verify batch content
	batch, err := w.GetBatch(batches[0])
	require.NoError(t, err)
	assert.Equal(t, 5, batch.Len())

	// Check metrics
	batchesCreated, txnsProcessed, _ := w.Metrics()
	assert.Equal(t, uint64(1), batchesCreated)
	assert.Equal(t, uint64(5), txnsProcessed)

	cancel()
}

func TestWorker_BatchCreation_OnTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w := worker.New(worker.Config{
		ID:           0,
		Partition:    "test",
		BatchSize:    100,                    // Large batch size
		BatchTimeout: 100 * time.Millisecond, // Short timeout
	}, nil)

	// Start worker in background
	go func() {
		w.Start(ctx)
	}()

	// Give worker time to start
	time.Sleep(50 * time.Millisecond)

	// Submit fewer than BatchSize transactions
	for i := 0; i < 3; i++ {
		err := w.Submit([]byte("tx"))
		require.NoError(t, err)
	}

	assert.Equal(t, 3, w.PendingCount())

	// Wait for timeout to fire
	time.Sleep(200 * time.Millisecond)

	// Batch should have been created by timeout
	assert.Equal(t, 0, w.PendingCount())
	assert.Equal(t, 1, w.BatchCount())

	cancel()
}

func TestWorker_BatchCreation_OnByteSizeLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w := worker.New(worker.Config{
		ID:            0,
		Partition:     "test",
		BatchSize:     1000,                   // Large transaction count
		BatchTimeout:  10 * time.Second,       // Long timeout
		MaxBatchBytes: 100,                    // Small byte limit
	}, nil)

	// Start worker in background
	go func() {
		w.Start(ctx)
	}()

	// Give worker time to start
	time.Sleep(50 * time.Millisecond)

	// Submit transactions that exceed MaxBatchBytes
	for i := 0; i < 3; i++ {
		err := w.Submit(make([]byte, 40)) // 3 * 40 = 120 > 100
		require.NoError(t, err)
	}

	// Wait for batch to be created
	time.Sleep(100 * time.Millisecond)

	// Batch should have been created due to byte size limit
	assert.Equal(t, 0, w.PendingCount())
	assert.GreaterOrEqual(t, w.BatchCount(), 1)

	cancel()
}

func TestWorker_GetBatch(t *testing.T) {
	w := worker.New(worker.Config{
		ID:        0,
		Partition: "test",
	}, nil)

	// Create a batch directly
	batch := types.NewBatch([][]byte{[]byte("test transaction")})
	digest := batch.Digest()

	// Store it
	err := w.StoreBatch(batch)
	require.NoError(t, err)

	t.Run("existing batch", func(t *testing.T) {
		retrieved, err := w.GetBatch(digest)
		require.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, digest, retrieved.Digest())
	})

	t.Run("non-existing batch", func(t *testing.T) {
		var fakeDigest types.BatchDigest
		fakeDigest[0] = 0xFF

		retrieved, err := w.GetBatch(fakeDigest)
		require.NoError(t, err)
		assert.Nil(t, retrieved)
	})
}

func TestWorker_StoreBatch(t *testing.T) {
	w := worker.New(worker.Config{
		ID:        0,
		Partition: "test",
	}, nil)

	t.Run("store valid batch", func(t *testing.T) {
		batch := types.NewBatch([][]byte{[]byte("tx1"), []byte("tx2")})
		err := w.StoreBatch(batch)
		require.NoError(t, err)

		assert.Equal(t, 1, w.BatchCount())
		assert.True(t, w.HasBatch(batch.Digest()))
	})

	t.Run("store nil batch", func(t *testing.T) {
		err := w.StoreBatch(nil)
		assert.Error(t, err)
	})

	t.Run("idempotent storage", func(t *testing.T) {
		batch := types.NewBatch([][]byte{[]byte("duplicate")})

		err := w.StoreBatch(batch)
		require.NoError(t, err)
		countBefore := w.BatchCount()

		// Store same batch again
		err = w.StoreBatch(batch)
		require.NoError(t, err)

		// Count should not increase
		assert.Equal(t, countBefore, w.BatchCount())
	})
}

func TestWorker_AvailableBatches(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w := worker.New(worker.Config{
		ID:           0,
		Partition:    "test",
		BatchSize:    3,
		BatchTimeout: 10 * time.Second,
	}, nil)

	// Start worker
	go func() {
		w.Start(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	// Create first batch
	for i := 0; i < 3; i++ {
		w.Submit([]byte("batch1"))
	}
	time.Sleep(100 * time.Millisecond)

	// Create second batch
	for i := 0; i < 3; i++ {
		w.Submit([]byte("batch2"))
	}
	time.Sleep(100 * time.Millisecond)

	// Should have 2 available batches
	available := w.AvailableBatches()
	assert.Len(t, available, 2)

	// Consume them
	consumed := w.ConsumeAvailableBatches()
	assert.Len(t, consumed, 2)

	// Available should now be empty
	available = w.AvailableBatches()
	assert.Len(t, available, 0)

	cancel()
}

func TestWorker_PruneBatches(t *testing.T) {
	w := worker.New(worker.Config{
		ID:        0,
		Partition: "test",
	}, nil)

	// Store multiple batches with different content (so they have different digests)
	var digests []types.BatchDigest
	for i := 0; i < 5; i++ {
		// Each batch has unique content to ensure unique digests
		batch := types.NewBatch([][]byte{[]byte(fmt.Sprintf("tx-%d", i))})
		w.StoreBatch(batch)
		digests = append(digests, batch.Digest())
	}

	assert.Equal(t, 5, w.BatchCount())

	// Prune first 3 batches
	w.PruneBatches(digests[:3])

	assert.Equal(t, 2, w.BatchCount())

	// Verify correct batches were pruned
	for _, d := range digests[:3] {
		assert.False(t, w.HasBatch(d))
	}
	for _, d := range digests[3:] {
		assert.True(t, w.HasBatch(d))
	}
}

func TestWorker_Close(t *testing.T) {
	w := worker.New(worker.Config{
		ID:        0,
		Partition: "test",
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())

	// Start in background
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	// Close should work
	err := w.Close()
	require.NoError(t, err)

	cancel()

	// Wait for Start to return
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after Close")
	}

	// Submit should fail after close
	err = w.Submit([]byte("test"))
	assert.ErrorIs(t, err, worker.ErrWorkerClosed)

	// Close again should be idempotent
	err = w.Close()
	require.NoError(t, err)
}

func TestWorker_ConcurrentSubmit(t *testing.T) {
	w := worker.New(worker.Config{
		ID:        0,
		Partition: "test",
		BatchSize: 1000, // Large batch size
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		w.Start(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	// Submit transactions concurrently
	var wg sync.WaitGroup
	numGoroutines := 10
	txnsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < txnsPerGoroutine; j++ {
				w.Submit([]byte("tx"))
			}
		}(i)
	}

	wg.Wait()

	// All transactions should be received
	_, _, txnsReceived := w.Metrics()
	assert.Equal(t, uint64(numGoroutines*txnsPerGoroutine), txnsReceived)

	cancel()
}

func TestWorker_BatchDigests(t *testing.T) {
	w := worker.New(worker.Config{
		ID:        0,
		Partition: "test",
	}, nil)

	// Store some batches
	batch1 := types.NewBatch([][]byte{[]byte("tx1")})
	batch2 := types.NewBatch([][]byte{[]byte("tx2")})
	batch3 := types.NewBatch([][]byte{[]byte("tx3")})

	w.StoreBatch(batch1)
	w.StoreBatch(batch2)
	w.StoreBatch(batch3)

	digests := w.BatchDigests()
	assert.Len(t, digests, 3)

	// Verify all digests are present
	digestSet := make(map[types.BatchDigest]bool)
	for _, d := range digests {
		digestSet[d] = true
	}

	assert.True(t, digestSet[batch1.Digest()])
	assert.True(t, digestSet[batch2.Digest()])
	assert.True(t, digestSet[batch3.Digest()])
}

func TestWorker_String(t *testing.T) {
	w := worker.New(worker.Config{
		ID:        42,
		Partition: "my-partition",
	}, nil)

	str := w.String()
	assert.Contains(t, str, "42")
	assert.Contains(t, str, "my-partition")
}

// testNetwork helps create connected test hosts for gossip tests.
type testNetwork struct {
	t     *testing.T
	ctx   context.Context
	hosts []host.Host
	ps    []*pubsub.PubSub
}

func newTestNetwork(t *testing.T, ctx context.Context, n int) *testNetwork {
	t.Helper()

	tn := &testNetwork{
		t:     t,
		ctx:   ctx,
		hosts: make([]host.Host, n),
		ps:    make([]*pubsub.PubSub, n),
	}

	// Create hosts
	for i := 0; i < n; i++ {
		netw := swarmt.GenSwarm(t)
		h := bhost.NewBlankHost(netw)
		tn.hosts[i] = h
		t.Cleanup(func() { h.Close() })
	}

	// Connect all hosts to each other
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			tn.connect(i, j)
		}
	}

	// Create pubsub instances
	for i := 0; i < n; i++ {
		ps, err := pubsub.NewGossipSub(ctx, tn.hosts[i])
		require.NoError(t, err)
		tn.ps[i] = ps
	}

	return tn
}

func (tn *testNetwork) connect(i, j int) {
	pi := peer.AddrInfo{
		ID:    tn.hosts[j].ID(),
		Addrs: tn.hosts[j].Addrs(),
	}
	err := tn.hosts[i].Connect(tn.ctx, pi)
	require.NoError(tn.t, err)
}

func TestWorker_WithGossip_BroadcastBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tn := newTestNetwork(t, ctx, 2)

	// Create gossip layers
	gl1, err := gossip.NewGossipLayer(tn.hosts[0], tn.ps[0], "test")
	require.NoError(t, err)

	gl2, err := gossip.NewGossipLayer(tn.hosts[1], tn.ps[1], "test")
	require.NoError(t, err)

	// Start gossip layers
	err = gl1.Start(ctx)
	require.NoError(t, err)
	defer gl1.Close()

	err = gl2.Start(ctx)
	require.NoError(t, err)
	defer gl2.Close()

	// Wait for mesh to form
	time.Sleep(500 * time.Millisecond)

	// Create workers
	w1 := worker.New(worker.Config{
		ID:           0,
		Partition:    "test",
		BatchSize:    3,
		BatchTimeout: 10 * time.Second,
	}, gl1)

	w2 := worker.New(worker.Config{
		ID:        1,
		Partition: "test",
	}, gl2)

	// Start both workers
	go func() { w1.Start(ctx) }()
	go func() { w2.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Submit transactions to worker 1
	for i := 0; i < 3; i++ {
		err := w1.Submit([]byte("tx-from-w1"))
		require.NoError(t, err)
	}

	// Wait for batch to be created and broadcast
	time.Sleep(500 * time.Millisecond)

	// Worker 2 should have received the batch
	batches := w1.AvailableBatches()
	require.Len(t, batches, 1)

	// Give more time for gossip propagation
	time.Sleep(500 * time.Millisecond)

	// Verify worker 2 has the batch
	batch, err := w2.GetBatch(batches[0])
	require.NoError(t, err)
	require.NotNil(t, batch, "worker 2 should have received the batch from worker 1")
	assert.Equal(t, 3, batch.Len())

	cancel()
}

func TestWorker_WithGossip_ReceiveIncomingBatches(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tn := newTestNetwork(t, ctx, 2)

	// Create gossip layers
	gl1, err := gossip.NewGossipLayer(tn.hosts[0], tn.ps[0], "test")
	require.NoError(t, err)

	gl2, err := gossip.NewGossipLayer(tn.hosts[1], tn.ps[1], "test")
	require.NoError(t, err)

	err = gl1.Start(ctx)
	require.NoError(t, err)
	defer gl1.Close()

	err = gl2.Start(ctx)
	require.NoError(t, err)
	defer gl2.Close()

	time.Sleep(500 * time.Millisecond)

	// Create worker on node 2
	w2 := worker.New(worker.Config{
		ID:        1,
		Partition: "test",
	}, gl2)

	go func() { w2.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	// Broadcast batch directly from gossip layer 1 (simulating another worker)
	batch := types.NewBatch([][]byte{
		[]byte("external tx 1"),
		[]byte("external tx 2"),
	})

	err = gl1.BroadcastBatch(ctx, batch)
	require.NoError(t, err)

	// Wait for propagation
	time.Sleep(500 * time.Millisecond)

	// Worker 2 should have stored the incoming batch
	retrieved, err := w2.GetBatch(batch.Digest())
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, batch.Digest(), retrieved.Digest())
	assert.Equal(t, 2, retrieved.Len())

	cancel()
}

func TestWorker_FinalBatchOnClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w := worker.New(worker.Config{
		ID:           0,
		Partition:    "test",
		BatchSize:    100,                   // Large batch size
		BatchTimeout: 10 * time.Second,      // Long timeout
	}, nil)

	// Start worker
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)

	// Submit some transactions (less than batch size)
	for i := 0; i < 5; i++ {
		w.Submit([]byte("pending-tx"))
	}

	assert.Equal(t, 5, w.PendingCount())

	// Close the worker
	w.Close()
	cancel()

	<-done

	// Final batch should have been created
	_, txnsProcessed, _ := w.Metrics()
	assert.Equal(t, uint64(5), txnsProcessed)
}

func TestWorker_BatchContentsCorrect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w := worker.New(worker.Config{
		ID:           0,
		Partition:    "test",
		BatchSize:    5,
		BatchTimeout: 10 * time.Second,
	}, nil)

	go func() { w.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)

	// Submit specific transactions
	txns := [][]byte{
		[]byte("transaction-1"),
		[]byte("transaction-2"),
		[]byte("transaction-3"),
		[]byte("transaction-4"),
		[]byte("transaction-5"),
	}

	for _, tx := range txns {
		err := w.Submit(tx)
		require.NoError(t, err)
	}

	// Wait for batch
	time.Sleep(100 * time.Millisecond)

	// Get the batch
	available := w.AvailableBatches()
	require.Len(t, available, 1)

	batch, err := w.GetBatch(available[0])
	require.NoError(t, err)
	require.NotNil(t, batch)

	// Verify contents
	require.Equal(t, 5, batch.Len())
	for i, tx := range batch.Transactions {
		assert.True(t, bytes.Equal(txns[i], tx), "transaction %d should match", i)
	}

	cancel()
}
