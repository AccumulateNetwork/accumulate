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
