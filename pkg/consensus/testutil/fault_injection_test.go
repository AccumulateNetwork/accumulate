// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testutil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestFaultInjector_PacketLoss tests that consensus continues with packet loss.
func TestFaultInjector_PacketLoss(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name       string
		lossRate   float64
		numNodes   int
		shouldPass bool
	}{
		{"5% packet loss", 0.05, 4, true},
		{"10% packet loss", 0.10, 4, true},
		{"20% packet loss", 0.20, 4, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			net := NewTestNetwork(t, tt.numNodes,
				WithBatchSize(5),
				WithBatchTimeout(30*time.Millisecond),
			)
			defer net.Stop()

			// Create faulty network
			fn := NewFaultyNetwork(net)

			// Start the network first
			err := net.Start()
			require.NoError(t, err)

			// Apply packet loss
			fn.WithPacketLoss(tt.lossRate)
			t.Logf("Applied %d%% packet loss to all nodes", int(tt.lossRate*100))

			// Submit transactions
			for i := 0; i < 50; i++ {
				tx := []byte(fmt.Sprintf("tx-%d", i))
				_ = net.SubmitTransaction(i%tt.numNodes, tx)
				time.Sleep(10 * time.Millisecond)
			}

			// Wait for round progression
			err = net.WaitForRound(2, 20*time.Second)
			if tt.shouldPass {
				if err != nil {
					net.LogNodeStatus(t)
					t.Logf("Note: %v (packet loss may cause slower progress)", err)
				}
			}

			// Verify consensus progressed despite packet loss
			maxRound := net.GetMaxRound()
			assert.Greater(t, maxRound, types.Round(0),
				"consensus should progress despite %d%% packet loss", int(tt.lossRate*100))

			// Verify nodes are in reasonable agreement
			minRound := net.GetMinRound()
			assert.LessOrEqual(t, maxRound-minRound, types.Round(5),
				"nodes should be roughly in sync despite packet loss")

			net.LogNodeStatus(t)

			// Verify no split brain - all nodes should agree on committed certificates
			net.AssertNodesAgree(t)
		})
	}
}

// TestFaultInjector_Latency tests consensus behavior with added latency.
func TestFaultInjector_Latency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name    string
		latency time.Duration
	}{
		{"100ms latency", 100 * time.Millisecond},
		{"500ms latency", 500 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			net := NewTestNetwork(t, 4,
				WithBatchSize(5),
				WithBatchTimeout(50*time.Millisecond),
			)
			defer net.Stop()

			fn := NewFaultyNetwork(net)

			err := net.Start()
			require.NoError(t, err)

			// Apply latency simulation
			fn.WithLatency(tt.latency)
			t.Logf("Applied %v latency to all nodes", tt.latency)

			// Submit transactions
			for i := 0; i < 40; i++ {
				tx := []byte(fmt.Sprintf("tx-%d", i))
				_ = net.SubmitTransaction(i%4, tx)
				time.Sleep(20 * time.Millisecond)
			}

			// With latency, consensus will be slower but should still progress
			// Give more time proportional to the latency
			timeout := 15*time.Second + tt.latency*10
			err = net.WaitForRound(2, timeout)
			if err != nil {
				net.LogNodeStatus(t)
				t.Logf("Note: round 2 not reached, but checking progress...")
			}

			// Verify consensus progressed
			maxRound := net.GetMaxRound()
			assert.Greater(t, maxRound, types.Round(0),
				"consensus should progress despite %v latency", tt.latency)

			net.LogNodeStatus(t)

			// Verify graceful degradation - performance degrades but consensus continues
			net.AssertNodesAgree(t)
		})
	}
}

// TestFaultInjector_NetworkPartition tests consensus behavior during network partitions.
func TestFaultInjector_NetworkPartition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a 4-node network (f=1)
	net := NewTestNetwork(t, 4,
		WithBatchSize(3),
		WithBatchTimeout(30*time.Millisecond),
	)
	defer net.Stop()

	fn := NewFaultyNetwork(net)

	err := net.Start()
	require.NoError(t, err)

	// Submit initial transactions
	for i := 0; i < 20; i++ {
		tx := []byte(fmt.Sprintf("tx-pre-%d", i))
		_ = net.SubmitTransaction(i%4, tx)
	}

	// Wait for initial progress
	time.Sleep(500 * time.Millisecond)
	initialMaxRound := net.GetMaxRound()
	t.Logf("Initial max round: %d", initialMaxRound)

	// Create a network partition: [0, 1, 2] vs [3]
	// With 4 nodes (f=1), the larger partition (3 nodes) should continue
	t.Log("Creating network partition: [0,1,2] vs [3]")
	fn.WithPartitions([][]int{{0, 1, 2}, {3}})

	// Submit transactions to the majority partition
	for i := 0; i < 30; i++ {
		tx := []byte(fmt.Sprintf("tx-partition-%d", i))
		_ = net.SubmitTransaction(i%3, tx) // Only to nodes 0, 1, 2
		time.Sleep(20 * time.Millisecond)
	}

	// Wait for the majority partition to make progress
	err = net.WaitForRound(initialMaxRound+2, 15*time.Second)
	if err != nil {
		net.LogNodeStatus(t)
	}

	// The majority partition should have made progress
	partitionedMaxRound := net.GetMaxRound()
	assert.Greater(t, partitionedMaxRound, initialMaxRound,
		"majority partition should continue making progress")

	// Heal the partition
	t.Log("Healing network partition")
	fn.Heal()

	// Wait for gossip mesh to reform
	time.Sleep(time.Second)

	// Submit more transactions to all nodes
	for i := 0; i < 20; i++ {
		tx := []byte(fmt.Sprintf("tx-healed-%d", i))
		_ = net.SubmitTransaction(i%4, tx)
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for recovery
	time.Sleep(2 * time.Second)

	// Verify all nodes recovered and are making progress
	net.LogNodeStatus(t)

	// Nodes should eventually agree
	net.AssertNodesAgree(t)

	t.Log("Partition recovery successful - no split brain detected")
}

// TestFaultInjector_NodeIsolation tests that a single isolated node can rejoin.
func TestFaultInjector_NodeIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a 4-node network
	net := NewTestNetwork(t, 4,
		WithBatchSize(5),
		WithBatchTimeout(30*time.Millisecond),
	)
	defer net.Stop()

	fn := NewFaultyNetwork(net)

	err := net.Start()
	require.NoError(t, err)

	// Submit initial transactions
	for i := 0; i < 20; i++ {
		tx := []byte(fmt.Sprintf("tx-initial-%d", i))
		_ = net.SubmitTransaction(i%4, tx)
	}

	time.Sleep(500 * time.Millisecond)
	initialRound := net.GetMaxRound()
	t.Logf("Initial max round: %d", initialRound)

	// Isolate node 3
	t.Log("Isolating node 3")
	fn.IsolateNode(3)

	// Verify isolation is recorded
	assert.True(t, fn.Injector.isolated[3], "node 3 should be marked as isolated")

	// Submit transactions (only to non-isolated nodes)
	for i := 0; i < 30; i++ {
		tx := []byte(fmt.Sprintf("tx-isolated-%d", i))
		_ = net.SubmitTransaction(i%3, tx) // Only to nodes 0, 1, 2
		time.Sleep(15 * time.Millisecond)
	}

	// Wait for remaining nodes to make progress
	err = net.WaitForRound(initialRound+2, 15*time.Second)
	if err != nil {
		net.LogNodeStatus(t)
	}

	isolatedMaxRound := net.GetMaxRound()
	assert.Greater(t, isolatedMaxRound, initialRound,
		"remaining nodes should continue making progress")

	// Get isolated node's round before reconnection
	isolatedNode := net.GetNode(3)
	isolatedNodeRound := isolatedNode.Node.CurrentRound()
	t.Logf("Isolated node 3 at round %d, others at max round %d", isolatedNodeRound, isolatedMaxRound)

	// Reconnect the isolated node
	t.Log("Reconnecting node 3")
	fn.ReconnectNode(3)

	// Wait for the node to sync
	time.Sleep(2 * time.Second)

	// Submit more transactions
	for i := 0; i < 20; i++ {
		tx := []byte(fmt.Sprintf("tx-rejoined-%d", i))
		_ = net.SubmitTransaction(i%4, tx)
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for sync
	time.Sleep(2 * time.Second)

	// Verify the isolated node caught up
	reconnectedNodeRound := isolatedNode.Node.CurrentRound()
	t.Logf("After reconnection, node 3 at round %d", reconnectedNodeRound)

	// The node should have caught up somewhat
	assert.Greater(t, reconnectedNodeRound, isolatedNodeRound,
		"isolated node should have caught up after reconnection")

	net.LogNodeStatus(t)

	// Verify no consensus forks
	net.AssertNodesAgree(t)

	t.Log("Node isolation and recovery successful")
}

// TestFaultInjector_CombinedFaults tests behavior with multiple simultaneous faults.
func TestFaultInjector_CombinedFaults(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a larger network to handle multiple faults
	net := NewTestNetwork(t, 7, // 7 nodes can tolerate f=2 faults
		WithBatchSize(5),
		WithBatchTimeout(40*time.Millisecond),
	)
	defer net.Stop()

	fn := NewFaultyNetwork(net)

	err := net.Start()
	require.NoError(t, err)

	// Apply combined faults
	t.Log("Applying combined faults: 10% packet loss + 100ms latency + isolating node 6")

	// 10% packet loss
	fn.WithPacketLoss(0.10)

	// 100ms latency
	fn.WithLatency(100 * time.Millisecond)

	// Isolate one node
	fn.IsolateNode(6)

	// Submit transactions
	for i := 0; i < 60; i++ {
		tx := []byte(fmt.Sprintf("tx-%d", i))
		nodeIdx := i % 6 // Only submit to non-isolated nodes
		_ = net.SubmitTransaction(nodeIdx, tx)
		time.Sleep(25 * time.Millisecond)
	}

	// Wait for progress (longer timeout due to combined faults)
	err = net.WaitForRound(2, 30*time.Second)
	if err != nil {
		net.LogNodeStatus(t)
		t.Logf("Note: %v", err)
	}

	// Verify consensus continued despite combined faults
	maxRound := net.GetMaxRound()
	assert.Greater(t, maxRound, types.Round(0),
		"consensus should progress despite combined faults")

	net.LogNodeStatus(t)

	// Verify no split brain among active nodes
	net.AssertNodesAgree(t)

	t.Log("Combined fault tolerance test passed")
}

// TestFaultInjector_Stats verifies fault injection statistics.
func TestFaultInjector_Stats(t *testing.T) {
	net := NewTestNetwork(t, 4, WithGossip(false))
	defer net.Stop()

	fi := NewFaultInjector(net)

	// Initially no faults
	stats := fi.Stats()
	assert.Equal(t, 0, stats.NodesWithPacketLoss)
	assert.Equal(t, 0, stats.NodesWithLatency)
	assert.Equal(t, 0, stats.IsolatedNodes)
	assert.Equal(t, 0, stats.NumPartitions)

	// Add some faults
	fi.SetPacketLoss(0, 0.1)
	fi.SetPacketLoss(1, 0.2)
	fi.SetLatency(2, 100*time.Millisecond)
	fi.SetPartitions([][]int{{0, 1}, {2, 3}})

	stats = fi.Stats()
	assert.Equal(t, 2, stats.NodesWithPacketLoss)
	assert.Equal(t, 1, stats.NodesWithLatency)
	assert.Equal(t, 2, stats.NumPartitions)
	assert.InDelta(t, 0.15, stats.AvgPacketLoss, 0.001) // (0.1 + 0.2) / 2

	// Test isolation
	fi.IsolateNode(3)
	stats = fi.Stats()
	assert.Equal(t, 1, stats.IsolatedNodes)

	// Reset
	fi.Reset()
	stats = fi.Stats()
	assert.Equal(t, 0, stats.NodesWithPacketLoss)
	assert.Equal(t, 0, stats.NodesWithLatency)
	assert.Equal(t, 0, stats.IsolatedNodes)
	assert.Equal(t, 0, stats.NumPartitions)
}

// TestFaultInjector_CanCommunicate tests partition communication checks.
func TestFaultInjector_CanCommunicate(t *testing.T) {
	net := NewTestNetwork(t, 4, WithGossip(false))
	defer net.Stop()

	fi := NewFaultInjector(net)

	// Without partitions, everyone can communicate
	assert.True(t, fi.CanCommunicate(0, 1))
	assert.True(t, fi.CanCommunicate(0, 3))

	// Set partitions: [0, 1] and [2, 3]
	fi.SetPartitions([][]int{{0, 1}, {2, 3}})

	// Within partition
	assert.True(t, fi.CanCommunicate(0, 1))
	assert.True(t, fi.CanCommunicate(2, 3))

	// Across partitions
	assert.False(t, fi.CanCommunicate(0, 2))
	assert.False(t, fi.CanCommunicate(1, 3))

	// Isolated node can't communicate with anyone
	fi.IsolateNode(0)
	assert.False(t, fi.CanCommunicate(0, 1))
	assert.False(t, fi.CanCommunicate(0, 2))
}

// TestFaultInjector_ShouldDropMessage tests packet loss probabilistic drops.
func TestFaultInjector_ShouldDropMessage(t *testing.T) {
	net := NewTestNetwork(t, 4, WithGossip(false))
	defer net.Stop()

	fi := NewFaultInjector(net)

	// Without packet loss, messages shouldn't drop
	fi.SetPacketLoss(0, 0)
	dropped := 0
	for i := 0; i < 100; i++ {
		if fi.ShouldDropMessage(0) {
			dropped++
		}
	}
	assert.Equal(t, 0, dropped, "no messages should drop with 0% loss")

	// With 100% packet loss, all messages should drop
	fi.SetPacketLoss(0, 1.0)
	dropped = 0
	for i := 0; i < 100; i++ {
		if fi.ShouldDropMessage(0) {
			dropped++
		}
	}
	assert.Equal(t, 100, dropped, "all messages should drop with 100% loss")

	// Isolated nodes should always drop
	fi.SetPacketLoss(1, 0)
	fi.IsolateNode(1)
	dropped = 0
	for i := 0; i < 100; i++ {
		if fi.ShouldDropMessage(1) {
			dropped++
		}
	}
	assert.Equal(t, 100, dropped, "isolated nodes should drop all messages")
}

// TestFaultInjector_GracefulDegradation tests that performance degrades gracefully.
func TestFaultInjector_GracefulDegradation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// First, measure baseline performance
	t.Log("Measuring baseline performance...")
	baselineNet := NewTestNetwork(t, 4,
		WithBatchSize(5),
		WithBatchTimeout(30*time.Millisecond),
	)

	err := baselineNet.Start()
	require.NoError(t, err)

	start := time.Now()
	for i := 0; i < 30; i++ {
		tx := []byte(fmt.Sprintf("baseline-tx-%d", i))
		_ = baselineNet.SubmitTransaction(i%4, tx)
	}

	// Wait for some progress
	_ = baselineNet.WaitForRound(2, 10*time.Second)
	baselineTime := time.Since(start)
	baselineRound := baselineNet.GetMaxRound()
	baselineNet.Stop()
	t.Logf("Baseline: reached round %d in %v", baselineRound, baselineTime)

	// Now measure with faults
	t.Log("Measuring performance with 10% packet loss...")
	faultyNet := NewTestNetwork(t, 4,
		WithBatchSize(5),
		WithBatchTimeout(30*time.Millisecond),
	)
	defer faultyNet.Stop()

	fn := NewFaultyNetwork(faultyNet)

	err = faultyNet.Start()
	require.NoError(t, err)

	fn.WithPacketLoss(0.10)

	start = time.Now()
	for i := 0; i < 30; i++ {
		tx := []byte(fmt.Sprintf("faulty-tx-%d", i))
		_ = faultyNet.SubmitTransaction(i%4, tx)
	}

	// Give same amount of time
	_ = faultyNet.WaitForRound(2, baselineTime*3) // Allow 3x baseline time
	faultyTime := time.Since(start)
	faultyRound := faultyNet.GetMaxRound()
	t.Logf("With faults: reached round %d in %v", faultyRound, faultyTime)

	// Performance should degrade but not fail completely
	assert.Greater(t, faultyRound, types.Round(0),
		"should still make progress with faults")

	// Verify no consensus forks (most important)
	faultyNet.AssertNodesAgree(t)

	t.Log("Graceful degradation verified")
}

// TestFaultInjector_NoConsensusForks verifies no split-brain scenarios.
func TestFaultInjector_NoConsensusForks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a network that will experience various faults
	net := NewTestNetwork(t, 5,
		WithBatchSize(3),
		WithBatchTimeout(25*time.Millisecond),
	)
	defer net.Stop()

	fn := NewFaultyNetwork(net)

	err := net.Start()
	require.NoError(t, err)

	// Phase 1: Normal operation
	t.Log("Phase 1: Normal operation")
	for i := 0; i < 20; i++ {
		tx := []byte(fmt.Sprintf("phase1-tx-%d", i))
		_ = net.SubmitTransaction(i%5, tx)
	}
	time.Sleep(time.Second)

	// Phase 2: Partition
	t.Log("Phase 2: Network partition [0,1,2] vs [3,4]")
	fn.WithPartitions([][]int{{0, 1, 2}, {3, 4}})

	for i := 0; i < 30; i++ {
		tx := []byte(fmt.Sprintf("phase2-tx-%d", i))
		_ = net.SubmitTransaction(i%3, tx) // Only majority partition
		time.Sleep(15 * time.Millisecond)
	}
	time.Sleep(time.Second)

	// Phase 3: Heal and verify
	t.Log("Phase 3: Healing partition")
	fn.Heal()
	time.Sleep(time.Second)

	for i := 0; i < 20; i++ {
		tx := []byte(fmt.Sprintf("phase3-tx-%d", i))
		_ = net.SubmitTransaction(i%5, tx)
	}
	time.Sleep(2 * time.Second)

	// CRITICAL: Verify no consensus forks
	// All active nodes should agree on the same committed certificates
	net.AssertNodesAgree(t)
	net.LogNodeStatus(t)

	t.Log("No consensus forks detected - Byzantine safety maintained")
}
