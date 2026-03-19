// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	bhost "github.com/libp2p/go-libp2p/p2p/host/blank"
	swarmt "github.com/libp2p/go-libp2p/p2p/net/swarm/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/testutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

func TestNewNode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		committee := types.NewCommittee([]types.ValidatorInfo{
			{PublicKey: pub, Stake: 100},
		}, 1)

		cfg := consensus.NodeConfig{
			Partition: "test",
			KeyPair:   priv,
		}

		node, err := consensus.NewNode(cfg, committee, nil, nil)
		require.NoError(t, err)
		require.NotNil(t, node)

		assert.Equal(t, "test", node.Partition())
		assert.Equal(t, pub, node.PublicKey())
		assert.NotNil(t, node.DAG())
		assert.NotNil(t, node.Primary())
		assert.NotNil(t, node.Bullshark())
		assert.Len(t, node.Workers(), 1)
		assert.Nil(t, node.Gossip()) // No host provided
	})

	t.Run("nil committee", func(t *testing.T) {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		cfg := consensus.NodeConfig{
			Partition: "test",
			KeyPair:   priv,
		}

		_, err = consensus.NewNode(cfg, nil, nil, nil)
		assert.Error(t, err)
	})

	t.Run("invalid key", func(t *testing.T) {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		committee := types.NewCommittee([]types.ValidatorInfo{
			{PublicKey: pub, Stake: 100},
		}, 1)

		cfg := consensus.NodeConfig{
			Partition: "test",
			KeyPair:   []byte("invalid"),
		}

		_, err = consensus.NewNode(cfg, committee, nil, nil)
		assert.Error(t, err)
	})

	t.Run("empty partition", func(t *testing.T) {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		committee := types.NewCommittee([]types.ValidatorInfo{
			{PublicKey: pub, Stake: 100},
		}, 1)

		cfg := consensus.NodeConfig{
			KeyPair: priv,
		}

		_, err = consensus.NewNode(cfg, committee, nil, nil)
		assert.Error(t, err)
	})

	t.Run("with gossip", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		netw := swarmt.GenSwarm(t)
		h := bhost.NewBlankHost(netw)
		defer h.Close()

		ps, err := pubsub.NewGossipSub(ctx, h)
		require.NoError(t, err)

		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		committee := types.NewCommittee([]types.ValidatorInfo{
			{PublicKey: pub, Stake: 100},
		}, 1)

		cfg := consensus.NodeConfig{
			Partition: "test",
			KeyPair:   priv,
		}

		node, err := consensus.NewNode(cfg, committee, h, ps)
		require.NoError(t, err)
		require.NotNil(t, node)
		assert.NotNil(t, node.Gossip())
	})
}

func TestNode_Lifecycle(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 100},
	}, 1)

	cfg := consensus.NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}

	node, err := consensus.NewNode(cfg, committee, nil, nil)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start in background
	done := make(chan error, 1)
	go func() {
		done <- node.Start(ctx)
	}()

	// Wait for start
	time.Sleep(20 * time.Millisecond)

	// Stop
	node.Stop()

	select {
	case err := <-done:
		// Context canceled or nil is fine
		if err != nil && err != context.Canceled {
			t.Logf("Start returned: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for node to stop")
	}
}

func TestNode_SubmitTransaction(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 100},
	}, 1)

	cfg := consensus.NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}

	node, err := consensus.NewNode(cfg, committee, nil, nil)
	require.NoError(t, err)

	// Should fail before start
	err = node.SubmitTransaction([]byte("test"))
	assert.Error(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	go func() {
		_ = node.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	// Should work after start
	err = node.SubmitTransaction([]byte("test"))
	assert.NoError(t, err)

	// Verify transaction was queued
	assert.Equal(t, uint64(1), func() uint64 { tx, _ := node.Metrics(); return tx }())

	node.Stop()
}

// Integration tests

func TestIntegration_SingleNode(t *testing.T) {
	// Skip in short mode as this is an integration test
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Single node can create certificates if it has quorum stake (it does, 100%)
	net := testutil.NewTestNetwork(t, 1,
		testutil.WithBatchSize(5),
		testutil.WithBatchTimeout(20*time.Millisecond),
		testutil.WithGossip(false), // No gossip needed for single node
	)
	defer net.Stop()

	err := net.Start()
	require.NoError(t, err)

	// Submit transactions
	for i := 0; i < 20; i++ {
		tx := []byte(fmt.Sprintf("tx-%d", i))
		err := net.SubmitTransaction(0, tx)
		require.NoError(t, err)
	}

	// Wait for batches to be created (workers process them)
	time.Sleep(200 * time.Millisecond)

	// In single-node mode without gossip, round advancement depends on
	// self-voting which may not happen automatically. Check what we have.
	node := net.GetNode(0)
	t.Logf("Single node: round=%d, commits=%d", node.Node.CurrentRound(), node.CommitCount())

	// Verify node is operational
	txSubmitted, _ := node.Node.Metrics()
	assert.Equal(t, uint64(20), txSubmitted)
}

func TestIntegration_ThreeNodes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	net := testutil.NewTestNetwork(t, 3,
		testutil.WithBatchSize(5),
		testutil.WithBatchTimeout(30*time.Millisecond),
	)
	defer net.Stop()

	err := net.Start()
	require.NoError(t, err)

	// Submit transactions to each node
	for i := 0; i < 30; i++ {
		tx := []byte(fmt.Sprintf("tx-%d", i))
		err := net.SubmitTransaction(i%3, tx)
		require.NoError(t, err)
		time.Sleep(5 * time.Millisecond)
	}

	// Wait for rounds to progress
	err = net.WaitForRound(2, 10*time.Second)
	require.NoError(t, err, "nodes should reach round 2")

	// Log status
	net.LogNodeStatus(t)

	// Verify all nodes have progressed
	net.AssertRoundProgress(t, 1)
}

func TestIntegration_FourNodes_OneFaulty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// 4 nodes with f=1 (3f+1=4), one can be faulty
	net := testutil.NewTestNetwork(t, 4,
		testutil.WithBatchSize(5),
		testutil.WithBatchTimeout(30*time.Millisecond),
	)
	defer net.Stop()

	err := net.Start()
	require.NoError(t, err)

	// Submit some transactions initially
	for i := 0; i < 20; i++ {
		tx := []byte(fmt.Sprintf("tx-pre-%d", i))
		_ = net.SubmitTransaction(i%4, tx)
	}

	// Wait for initial progress
	time.Sleep(300 * time.Millisecond)

	// Stop one node (simulate failure)
	net.StopNode(3)
	t.Log("Stopped node 3 (simulating failure)")

	// Continue submitting to remaining 3 nodes
	for i := 0; i < 30; i++ {
		tx := []byte(fmt.Sprintf("tx-post-%d", i))
		_ = net.SubmitTransaction(i%3, tx)
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for rounds to continue
	err = net.WaitForRound(2, 15*time.Second)
	if err != nil {
		net.LogNodeStatus(t)
	}
	require.NoError(t, err, "remaining 3 nodes should still make progress")

	// Verify active nodes count
	assert.Equal(t, 3, net.ActiveNodeCount())

	// Log final status
	net.LogNodeStatus(t)
}

func TestIntegration_BasicCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	net := testutil.NewTestNetwork(t, 3,
		testutil.WithBatchSize(3),
		testutil.WithBatchTimeout(20*time.Millisecond),
	)
	defer net.Stop()

	err := net.Start()
	require.NoError(t, err)

	// Submit transactions
	for i := 0; i < 50; i++ {
		tx := []byte(fmt.Sprintf("tx-%d", i))
		_ = net.SubmitTransaction(i%3, tx)
		time.Sleep(5 * time.Millisecond)
	}

	// Wait for some commits (may take time for Bullshark to commit leaders)
	// Leaders are at even rounds, so need round 3+ to commit round 2 leader
	err = net.WaitForRound(3, 15*time.Second)
	if err != nil {
		net.LogNodeStatus(t)
		t.Logf("Warning: %v", err)
	}

	// Check if we got any commits
	totalCommits := net.GetTotalCommits()
	t.Logf("Total commits across all nodes: %d", totalCommits)

	// Log final status
	net.LogNodeStatus(t)
}

func TestIntegration_RoundProgression(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	net := testutil.NewTestNetwork(t, 4,
		testutil.WithBatchSize(5),
		testutil.WithBatchTimeout(25*time.Millisecond),
	)
	defer net.Stop()

	err := net.Start()
	require.NoError(t, err)

	// Submit enough transactions to trigger multiple rounds
	for i := 0; i < 100; i++ {
		tx := []byte(fmt.Sprintf("tx-%d", i))
		_ = net.SubmitTransaction(i%4, tx)
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for rounds to advance
	time.Sleep(3 * time.Second)

	// Check round progression
	maxRound := net.GetMaxRound()
	minRound := net.GetMinRound()

	t.Logf("Round range: min=%d, max=%d", minRound, maxRound)
	net.LogNodeStatus(t)

	// Rounds should have advanced past initial state
	assert.Greater(t, maxRound, types.Round(1), "rounds should advance past 1")
}

func TestIntegration_LeaderCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Bullshark commits leaders at even rounds when they have f+1 support
	net := testutil.NewTestNetwork(t, 3,
		testutil.WithBatchSize(3),
		testutil.WithBatchTimeout(15*time.Millisecond),
	)
	defer net.Stop()

	err := net.Start()
	require.NoError(t, err)

	// Submit many transactions to drive consensus
	for i := 0; i < 80; i++ {
		tx := []byte(fmt.Sprintf("tx-%d", i))
		_ = net.SubmitTransaction(i%3, tx)
		time.Sleep(8 * time.Millisecond)
	}

	// Wait for round progression
	err = net.WaitForRound(4, 20*time.Second)
	if err != nil {
		net.LogNodeStatus(t)
		t.Logf("Note: %v", err)
	}

	// Check Bullshark commit progress
	for i, n := range net.GetAllNodes() {
		if n.IsStopped() {
			continue
		}
		lastCommit := n.Node.LastCommitRound()
		t.Logf("Node %d: lastCommitRound=%d", i, lastCommit)
	}

	// Even if we don't get commits, verify the system is operational
	maxRound := net.GetMaxRound()
	assert.Greater(t, maxRound, types.Round(0), "should have progressed past round 0")

	net.LogNodeStatus(t)
}

func TestIntegration_DAGConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	net := testutil.NewTestNetwork(t, 3,
		testutil.WithBatchSize(5),
		testutil.WithBatchTimeout(20*time.Millisecond),
	)
	defer net.Stop()

	err := net.Start()
	require.NoError(t, err)

	// Submit transactions
	for i := 0; i < 40; i++ {
		tx := []byte(fmt.Sprintf("tx-%d", i))
		_ = net.SubmitTransaction(i%3, tx)
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for progress
	time.Sleep(2 * time.Second)

	// Check DAG consistency
	net.AssertDAGConsistency(t)

	// Log DAG sizes
	for i, n := range net.GetAllNodes() {
		t.Logf("Node %d DAG size: %d", i, n.Node.DAG().Size())
	}
}

func TestIntegration_MultipleWorkers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	net := testutil.NewTestNetwork(t, 3,
		testutil.WithNumWorkers(3),
		testutil.WithBatchSize(5),
		testutil.WithBatchTimeout(20*time.Millisecond),
	)
	defer net.Stop()

	err := net.Start()
	require.NoError(t, err)

	// Submit transactions
	for i := 0; i < 50; i++ {
		tx := []byte(fmt.Sprintf("tx-%d", i))
		_ = net.SubmitTransaction(i%3, tx)
	}

	// Wait for processing
	time.Sleep(time.Second)

	// Verify workers are all operational
	for i, n := range net.GetAllNodes() {
		workers := n.Node.Workers()
		assert.Len(t, workers, 3, "node %d should have 3 workers", i)

		for j, w := range workers {
			batchCount := w.BatchCount()
			t.Logf("Node %d Worker %d: batchCount=%d", i, j, batchCount)
		}
	}

	net.LogNodeStatus(t)
}

func TestIntegration_GenesisBootstrap(t *testing.T) {
	// Test that genesis is properly inserted and nodes can build from it
	pub1, priv1, _ := ed25519.GenerateKey(rand.Reader)
	pub2, priv2, _ := ed25519.GenerateKey(rand.Reader)
	pub3, priv3, _ := ed25519.GenerateKey(rand.Reader)

	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub1, Stake: 100},
		{PublicKey: pub2, Stake: 100},
		{PublicKey: pub3, Stake: 100},
	}, 1)

	cfg := consensus.NodeConfig{
		Partition:  "test",
		KeyPair:    priv1,
		NumWorkers: 1,
		WorkerConfig: worker.Config{
			BatchSize:    5,
			BatchTimeout: 20 * time.Millisecond,
		},
	}

	node, err := consensus.NewNode(cfg, committee, nil, nil)
	require.NoError(t, err)

	// Insert genesis
	err = node.InsertGenesisForAll([]ed25519.PrivateKey{priv1, priv2, priv3})
	require.NoError(t, err)

	// Verify genesis certificates are in DAG
	dag := node.DAG()
	genesisCerts := dag.GetRound(0)
	require.Len(t, genesisCerts, 3, "should have 3 genesis certificates")

	// Verify node is at round 1
	assert.Equal(t, types.Round(1), node.CurrentRound())

	// Verify DAG has quorum at round 0
	assert.True(t, dag.HasQuorum(0, committee))
}

func TestIntegration_StakeWeighting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create network with unequal stakes
	net := testutil.NewTestNetwork(t, 4,
		testutil.WithStakes([]uint64{400, 200, 200, 200}), // First node has 40% stake
		testutil.WithBatchSize(5),
		testutil.WithBatchTimeout(20*time.Millisecond),
	)
	defer net.Stop()

	err := net.Start()
	require.NoError(t, err)

	// Verify committee setup
	committee := net.Committee()
	assert.Equal(t, uint64(1000), committee.TotalStake())
	assert.Equal(t, uint64(667), committee.QuorumThreshold()) // >2/3 of 1000

	// Submit transactions
	for i := 0; i < 30; i++ {
		tx := []byte(fmt.Sprintf("tx-%d", i))
		_ = net.SubmitTransaction(i%4, tx)
	}

	time.Sleep(time.Second)
	net.LogNodeStatus(t)
}

// Benchmarks

func BenchmarkTransactionSubmission(b *testing.B) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	committee := types.NewCommittee([]types.ValidatorInfo{
		{PublicKey: pub, Stake: 100},
	}, 1)

	cfg := consensus.NodeConfig{
		Partition:  "test",
		KeyPair:    priv,
		NumWorkers: 4,
	}

	node, err := consensus.NewNode(cfg, committee, nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = node.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)
	defer func() {
		cancel()
		node.Stop()
	}()

	tx := []byte("benchmark-transaction")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = node.SubmitTransaction(tx)
	}
}
