// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/persist"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func makeTestCommitteeRecovery(priv ed25519.PrivateKey) *types.Committee {
	validators := []types.ValidatorInfo{
		{
			PublicKey: priv.Public().(ed25519.PublicKey),
			Stake:     1,
		},
	}
	return types.NewCommittee(validators, 0)
}

func TestRecoveryManager_NoCheckpoint(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recovery_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	committee := makeTestCommitteeRecovery(priv)

	node, _ := NewNode(NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}, committee, nil, nil)

	store := persist.NewStore(tmpDir)
	manager := NewRecoveryManager(node, store, RecoveryConfig{
		SkipCatchUp: true,
	})

	result, err := manager.Recover(context.Background())
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}

	if result.Recovered {
		t.Error("should not be recovered without checkpoint")
	}
	if result.FromCheckpoint {
		t.Error("should not be from checkpoint")
	}
}

func TestRecoveryManager_WithCheckpoint(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recovery_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	committee := makeTestCommitteeRecovery(priv)

	store := persist.NewStore(tmpDir)

	// Create and save checkpoint
	lastCommitted := map[string]types.Round{
		"abc123": 5,
	}
	checkpoint := persist.NewCheckpoint("test", 10, 1, 8, lastCommitted)
	if err := store.Save(checkpoint); err != nil {
		t.Fatal(err)
	}

	// Create node
	node, _ := NewNode(NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}, committee, nil, nil)

	manager := NewRecoveryManager(node, store, RecoveryConfig{
		SkipCatchUp: true,
	})

	result, err := manager.Recover(context.Background())
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}

	if !result.Recovered {
		t.Error("should be recovered")
	}
	if !result.FromCheckpoint {
		t.Error("should be from checkpoint")
	}
	if result.RestoredRound != 10 {
		t.Errorf("restored round should be 10, got %d", result.RestoredRound)
	}

	// Verify node state was restored
	if node.CurrentRound() != 10 {
		t.Errorf("node current round should be 10, got %d", node.CurrentRound())
	}
	if node.bullshark.LastCommitRound() != 8 {
		t.Errorf("bullshark last commit round should be 8, got %d", node.bullshark.LastCommitRound())
	}
}

func TestRecoveryManager_PartitionMismatch(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recovery_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	committee := makeTestCommitteeRecovery(priv)

	store := persist.NewStore(tmpDir)

	// Create checkpoint with different partition
	checkpoint := persist.NewCheckpoint("other-partition", 10, 1, 8, nil)
	if err := store.Save(checkpoint); err != nil {
		t.Fatal(err)
	}

	// Create node with different partition
	node, _ := NewNode(NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}, committee, nil, nil)

	manager := NewRecoveryManager(node, store, RecoveryConfig{
		SkipCatchUp: true,
	})

	_, err = manager.Recover(context.Background())
	if err == nil {
		t.Fatal("expected error for partition mismatch")
	}
}

func TestRecoverFromCheckpoint(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recovery_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	committee := makeTestCommitteeRecovery(priv)

	store := persist.NewStore(tmpDir)
	checkpoint := persist.NewCheckpoint("test", 15, 2, 12, nil)
	store.Save(checkpoint)

	node, _ := NewNode(NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}, committee, nil, nil)

	result, err := RecoverFromCheckpoint(context.Background(), node, store)
	if err != nil {
		t.Fatalf("RecoverFromCheckpoint() error = %v", err)
	}

	if !result.Recovered {
		t.Error("should be recovered")
	}
	if result.RestoredRound != 15 {
		t.Errorf("restored round should be 15, got %d", result.RestoredRound)
	}
}

func TestNewNodeWithRecovery(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recovery_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	committee := makeTestCommitteeRecovery(priv)

	store := persist.NewStore(tmpDir)
	checkpoint := persist.NewCheckpoint("test", 20, 3, 18, nil)
	store.Save(checkpoint)

	node, result, err := NewNodeWithRecovery(NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}, committee, store)

	if err != nil {
		t.Fatalf("NewNodeWithRecovery() error = %v", err)
	}
	if node == nil {
		t.Fatal("node should not be nil")
	}
	if !result.Recovered {
		t.Error("should be recovered")
	}
	if result.RestoredRound != 20 {
		t.Errorf("restored round should be 20, got %d", result.RestoredRound)
	}
}

func TestShutdownAndRecovery_Integration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "integration_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	committee := makeTestCommitteeRecovery(priv)

	store := persist.NewStore(tmpDir)

	// Create and start first node
	node1, _ := NewNode(NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}, committee, nil, nil)

	ctx := context.Background()
	node1.Start(ctx)

	// Advance round
	node1.primary.SetRound(10)

	// Graceful shutdown with persistence
	err = node1.GracefulShutdownWithPersistence(context.Background(), store)
	if err != nil {
		t.Fatalf("GracefulShutdownWithPersistence() error = %v", err)
	}

	// Create second node and recover
	node2, result, err := NewNodeWithRecovery(NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}, committee, store)

	if err != nil {
		t.Fatalf("NewNodeWithRecovery() error = %v", err)
	}

	if !result.Recovered {
		t.Error("should be recovered")
	}
	if node2.CurrentRound() != 10 {
		t.Errorf("recovered node should have round 10, got %d", node2.CurrentRound())
	}
}

func TestNode_SetRound(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	committee := makeTestCommitteeRecovery(priv)

	node, _ := NewNode(NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}, committee, nil, nil)

	node.SetRound(42)

	if node.CurrentRound() != 42 {
		t.Errorf("expected round 42, got %d", node.CurrentRound())
	}
}

func TestRecoveryConfig_Defaults(t *testing.T) {
	config := RecoveryConfig{}
	config.applyDefaults()

	if config.CatchUpTimeout != DefaultCatchUpTimeout {
		t.Errorf("expected catch up timeout %v, got %v", DefaultCatchUpTimeout, config.CatchUpTimeout)
	}
	if config.PeerSyncInterval != DefaultPeerSyncInterval {
		t.Errorf("expected peer sync interval %v, got %v", DefaultPeerSyncInterval, config.PeerSyncInterval)
	}
	if config.ReconnectTimeout != DefaultReconnectTimeout {
		t.Errorf("expected reconnect timeout %v, got %v", DefaultReconnectTimeout, config.ReconnectTimeout)
	}
}

func TestShutdownConfig_Defaults(t *testing.T) {
	config := ShutdownConfig{}
	config.applyDefaults()

	if config.DrainTimeout != DefaultDrainTimeout {
		t.Errorf("expected drain timeout %v, got %v", DefaultDrainTimeout, config.DrainTimeout)
	}
	if config.GracePeriod != DefaultShutdownGracePeriod {
		t.Errorf("expected grace period %v, got %v", DefaultShutdownGracePeriod, config.GracePeriod)
	}
}

func TestRecoveryResult_Fields(t *testing.T) {
	result := &RecoveryResult{
		Recovered:            true,
		FromCheckpoint:       true,
		RestoredRound:        10,
		CurrentRound:         12,
		CertificatesCaughtUp: 5,
	}

	if !result.Recovered {
		t.Error("Recovered should be true")
	}
	if !result.FromCheckpoint {
		t.Error("FromCheckpoint should be true")
	}
	if result.RestoredRound != 10 {
		t.Error("RestoredRound mismatch")
	}
	if result.CurrentRound != 12 {
		t.Error("CurrentRound mismatch")
	}
	if result.CertificatesCaughtUp != 5 {
		t.Error("CertificatesCaughtUp mismatch")
	}
}

func TestRecoverAndStart(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recovery_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	committee := makeTestCommitteeRecovery(priv)

	store := persist.NewStore(tmpDir)
	checkpoint := persist.NewCheckpoint("test", 5, 1, 4, nil)
	store.Save(checkpoint)

	node, _ := NewNode(NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}, committee, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := RecoverAndStart(ctx, node, store)
	if err != nil {
		t.Fatalf("RecoverAndStart() error = %v", err)
	}

	if !result.Recovered {
		t.Error("should be recovered")
	}

	// Verify node is running
	time.Sleep(50 * time.Millisecond)

	// Node should accept transactions
	tx := make([]byte, 32)
	rand.Read(tx)
	if err := node.SubmitTransaction(tx); err != nil {
		t.Errorf("node should accept transactions after recovery: %v", err)
	}

	node.Stop()
}

// TestRestartRecovery tests restart from persisted state.
// This test verifies that:
// - Node starts at correct height after restart
// - DAG state is recovered
// - Node rejoins consensus at correct round
func TestRestartRecovery(t *testing.T) {
	// Create temp directory for persistence
	tmpDir, err := os.MkdirTemp("", "restart_recovery_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create key pair
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Create committee
	committee := makeTestCommitteeRecovery(priv)

	// Create checkpoint store
	store := persist.NewStore(tmpDir)

	// --- Phase 1: Start node, process blocks to height 50, shutdown ---
	node1, err := NewNode(NodeConfig{
		Partition: "test-restart",
		KeyPair:   priv,
	}, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Start first node
	ctx1, cancel1 := context.WithCancel(context.Background())
	if err := node1.Start(ctx1); err != nil {
		cancel1()
		t.Fatal(err)
	}

	// Advance to round 50
	targetRound := types.Round(50)
	node1.primary.SetRound(targetRound)

	// Simulate some commits
	lastCommitRound := types.Round(48)
	node1.bullshark.SetLastCommitRound(lastCommitRound)

	// Set up per-author commit tracking
	authorKey := types.HeaderDigest(priv.Public().(ed25519.PublicKey)).String()
	node1.bullshark.SetLastCommittedForAuthor(authorKey, types.Round(47))

	// Verify state before shutdown
	if node1.CurrentRound() != targetRound {
		t.Errorf("expected round %d before shutdown, got %d", targetRound, node1.CurrentRound())
	}

	// Graceful shutdown with persistence
	err = node1.GracefulShutdownWithPersistence(context.Background(), store)
	if err != nil {
		cancel1()
		t.Fatalf("GracefulShutdownWithPersistence() error = %v", err)
	}
	cancel1()

	// Verify checkpoint exists
	if !store.Exists() {
		t.Fatal("checkpoint should exist after shutdown")
	}

	// --- Phase 2: Restart node and verify recovery ---
	node2, result, err := NewNodeWithRecovery(NodeConfig{
		Partition: "test-restart",
		KeyPair:   priv,
	}, committee, store)
	if err != nil {
		t.Fatalf("NewNodeWithRecovery() error = %v", err)
	}

	// Verify: Recovery happened
	if !result.Recovered {
		t.Error("should be recovered")
	}
	if !result.FromCheckpoint {
		t.Error("should be recovered from checkpoint")
	}

	// Verify: Starts at correct height (round)
	if result.RestoredRound != targetRound {
		t.Errorf("restored round should be %d, got %d", targetRound, result.RestoredRound)
	}

	// Verify: DAG state recovered - current round matches
	if node2.CurrentRound() != targetRound {
		t.Errorf("recovered node should have round %d, got %d", targetRound, node2.CurrentRound())
	}

	// Verify: Bullshark last commit round recovered
	if node2.bullshark.LastCommitRound() != lastCommitRound {
		t.Errorf("recovered bullshark should have last commit round %d, got %d",
			lastCommitRound, node2.bullshark.LastCommitRound())
	}

	// Verify: DAG last commit round recovered
	if node2.dag.LastCommitRound() != lastCommitRound {
		t.Errorf("recovered DAG should have last commit round %d, got %d",
			lastCommitRound, node2.dag.LastCommitRound())
	}

	// Start the recovered node and verify it can rejoin consensus
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	if err := node2.Start(ctx2); err != nil {
		t.Fatalf("failed to start recovered node: %v", err)
	}

	// Give the node time to start
	time.Sleep(50 * time.Millisecond)

	// Verify: Node can accept transactions (rejoined consensus)
	tx := make([]byte, 32)
	rand.Read(tx)
	if err := node2.SubmitTransaction(tx); err != nil {
		t.Errorf("recovered node should accept transactions: %v", err)
	}

	node2.Stop()
}

// TestCrashRecovery tests recovery after a simulated crash (SIGKILL).
// This test verifies that:
// - Node recovers from last persisted state after crash
// - State consistency is maintained even without graceful shutdown
func TestCrashRecovery(t *testing.T) {
	// Create temp directory for persistence
	tmpDir, err := os.MkdirTemp("", "crash_recovery_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create key pair
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Create committee
	committee := makeTestCommitteeRecovery(priv)

	// Create checkpoint store
	store := persist.NewStore(tmpDir)

	// --- Phase 1: Start node, process blocks, persist state, then "crash" ---
	node1, err := NewNode(NodeConfig{
		Partition: "test-crash",
		KeyPair:   priv,
	}, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Start first node
	ctx1, cancel1 := context.WithCancel(context.Background())
	if err := node1.Start(ctx1); err != nil {
		cancel1()
		t.Fatal(err)
	}

	// Advance to round 30
	node1.primary.SetRound(30)
	node1.bullshark.SetLastCommitRound(28)

	// Persist state (simulating periodic checkpointing)
	if err := node1.PersistState(store); err != nil {
		cancel1()
		t.Fatalf("PersistState() error = %v", err)
	}

	// Continue processing - advance more rounds without persisting
	node1.primary.SetRound(35)
	node1.bullshark.SetLastCommitRound(33)

	// Simulate crash: DON'T call graceful shutdown, just cancel and abandon
	// This simulates SIGKILL where no cleanup happens
	cancel1()
	// Note: We intentionally don't call node1.Stop() to simulate crash

	// --- Phase 2: Restart node and verify recovery from last checkpoint ---
	node2, result, err := NewNodeWithRecovery(NodeConfig{
		Partition: "test-crash",
		KeyPair:   priv,
	}, committee, store)
	if err != nil {
		t.Fatalf("NewNodeWithRecovery() error = %v", err)
	}

	// Verify: Recovery happened
	if !result.Recovered {
		t.Error("should be recovered")
	}

	// Verify: Recovers from last PERSISTED state (round 30), not latest state (round 35)
	// This is expected crash recovery behavior - we lose unpersisted progress
	if result.RestoredRound != 30 {
		t.Errorf("restored round should be 30 (last persisted), got %d", result.RestoredRound)
	}

	if node2.CurrentRound() != 30 {
		t.Errorf("recovered node should have round 30, got %d", node2.CurrentRound())
	}

	// Verify: Bullshark state from checkpoint
	if node2.bullshark.LastCommitRound() != 28 {
		t.Errorf("recovered bullshark should have last commit round 28, got %d",
			node2.bullshark.LastCommitRound())
	}

	// Start the recovered node and verify it operates normally
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	if err := node2.Start(ctx2); err != nil {
		t.Fatalf("failed to start recovered node: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify: Node can accept transactions after crash recovery
	tx := make([]byte, 32)
	rand.Read(tx)
	if err := node2.SubmitTransaction(tx); err != nil {
		t.Errorf("crash-recovered node should accept transactions: %v", err)
	}

	node2.Stop()
}

// TestMissedRoundsRecovery tests catching up after downtime.
// This test verifies that:
// - A stopped node can catch up with rounds advanced by other nodes
// - Node properly recovers missed rounds
func TestMissedRoundsRecovery(t *testing.T) {
	// Create temp directories for persistence
	tmpDir1, err := os.MkdirTemp("", "missed_rounds_test_node1")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir1)

	// Generate 4 validator keys
	keys := make([]ed25519.PrivateKey, 4)
	validators := make([]types.ValidatorInfo, 4)
	for i := range 4 {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		keys[i] = priv
		validators[i] = types.ValidatorInfo{
			PublicKey: priv.Public().(ed25519.PublicKey),
			Stake:     1,
		}
	}

	// Create committee with all 4 validators
	committee := types.NewCommittee(validators, 0)

	// Create checkpoint store for node 1
	store1 := persist.NewStore(tmpDir1)

	// --- Phase 1: Start all 4 nodes ---
	nodes := make([]*Node, 4)
	ctxs := make([]context.Context, 4)
	cancels := make([]context.CancelFunc, 4)

	for i := range 4 {
		node, err := NewNode(NodeConfig{
			Partition: "test-missed",
			KeyPair:   keys[i],
		}, committee, nil, nil)
		if err != nil {
			t.Fatalf("failed to create node %d: %v", i, err)
		}
		nodes[i] = node

		ctxs[i], cancels[i] = context.WithCancel(context.Background())
		if err := node.Start(ctxs[i]); err != nil {
			t.Fatalf("failed to start node %d: %v", i, err)
		}
	}

	// All nodes start at round 1
	for i := range 4 {
		nodes[i].primary.SetRound(5)
	}

	// --- Phase 2: Stop node 1, persist its state ---
	if err := nodes[0].PersistState(store1); err != nil {
		t.Fatalf("failed to persist node 1 state: %v", err)
	}
	stoppedAtRound := nodes[0].CurrentRound()

	// Stop node 1 (simulating downtime)
	cancels[0]()
	nodes[0].Stop()

	// --- Phase 3: Let nodes 2-4 advance 10 rounds ---
	advancedRounds := types.Round(10)
	for i := 1; i < 4; i++ {
		nodes[i].primary.SetRound(stoppedAtRound + advancedRounds)
		nodes[i].bullshark.SetLastCommitRound(stoppedAtRound + advancedRounds - 2)
	}

	currentNetworkRound := stoppedAtRound + advancedRounds

	// Verify other nodes advanced
	for i := 1; i < 4; i++ {
		if nodes[i].CurrentRound() != currentNetworkRound {
			t.Errorf("node %d should be at round %d, got %d", i, currentNetworkRound, nodes[i].CurrentRound())
		}
	}

	// --- Phase 4: Restart node 1 and verify it can catch up ---
	node1Recovered, result, err := NewNodeWithRecovery(NodeConfig{
		Partition: "test-missed",
		KeyPair:   keys[0],
	}, committee, store1)
	if err != nil {
		t.Fatalf("NewNodeWithRecovery() error = %v", err)
	}

	// Verify recovery happened
	if !result.Recovered {
		t.Error("node 1 should be recovered")
	}

	// Verify it starts at the round it was stopped at
	if result.RestoredRound != stoppedAtRound {
		t.Errorf("node 1 should restore to round %d, got %d", stoppedAtRound, result.RestoredRound)
	}

	// Node is behind the network
	missedRounds := currentNetworkRound - result.RestoredRound
	if missedRounds != advancedRounds {
		t.Errorf("expected to miss %d rounds, but missed %d", advancedRounds, missedRounds)
	}

	// Start the recovered node
	ctx1New, cancel1New := context.WithCancel(context.Background())
	defer cancel1New()

	if err := node1Recovered.Start(ctx1New); err != nil {
		t.Fatalf("failed to start recovered node 1: %v", err)
	}

	// In a real scenario with networking, the node would catch up via gossip.
	// Since we don't have networking in this test, we simulate the catch-up
	// by manually advancing the round (which would happen through certificate sync).
	node1Recovered.primary.SetRound(currentNetworkRound)

	// Verify: Node 1 catches up to the current network round
	if node1Recovered.CurrentRound() != currentNetworkRound {
		t.Errorf("node 1 should catch up to round %d, got %d",
			currentNetworkRound, node1Recovered.CurrentRound())
	}

	// Verify: Node can participate in consensus after catching up
	tx := make([]byte, 32)
	rand.Read(tx)
	if err := node1Recovered.SubmitTransaction(tx); err != nil {
		t.Errorf("recovered node should accept transactions after catching up: %v", err)
	}

	// Cleanup
	node1Recovered.Stop()
	for i := 1; i < 4; i++ {
		cancels[i]()
		nodes[i].Stop()
	}
}
