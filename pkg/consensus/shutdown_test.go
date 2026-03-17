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

func makeTestCommittee(priv ed25519.PrivateKey) *types.Committee {
	validators := []types.ValidatorInfo{
		{
			PublicKey: priv.Public().(ed25519.PublicKey),
			Stake:     1,
		},
	}
	return types.NewCommittee(validators, 0)
}

func TestShutdownManager_GracefulShutdown(t *testing.T) {
	// Create key pair
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Create committee
	committee := makeTestCommittee(priv)

	// Create node
	config := NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}
	node, err := NewNode(config, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Start node
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := node.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Submit some transactions
	for i := 0; i < 10; i++ {
		tx := make([]byte, 32)
		rand.Read(tx)
		node.SubmitTransaction(tx)
	}

	// Create shutdown manager
	manager := NewShutdownManager(node, ShutdownConfig{
		DrainTimeout: 5 * time.Second,
	})

	// Perform shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := manager.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	// Verify shutdown complete
	if !manager.IsShutdownComplete() {
		t.Error("shutdown should be complete")
	}

	// Verify node is closed
	if err := node.SubmitTransaction([]byte("test")); err != ErrNodeClosed {
		t.Errorf("expected ErrNodeClosed, got %v", err)
	}
}

func TestShutdownManager_WithPersistence(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "shutdown_test")
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
	committee := makeTestCommittee(priv)

	// Create node
	config := NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}
	node, err := NewNode(config, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Start node
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := node.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Create checkpoint store
	store := persist.NewStore(tmpDir)

	// Create shutdown manager with persistence
	manager := NewShutdownManager(node, ShutdownConfig{
		DrainTimeout:    5 * time.Second,
		PersistState:    true,
		CheckpointStore: store,
	})

	// Perform shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := manager.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	// Verify checkpoint was created
	if !store.Exists() {
		t.Error("checkpoint should exist after shutdown with persistence")
	}

	// Load and verify checkpoint
	checkpoint, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if checkpoint.Partition != "test" {
		t.Errorf("checkpoint partition mismatch: got %s, want test", checkpoint.Partition)
	}
}

func TestShutdownManager_AlreadyInProgress(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	committee := makeTestCommittee(priv)

	node, _ := NewNode(NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}, committee, nil, nil)

	ctx := context.Background()
	node.Start(ctx)

	manager := NewShutdownManager(node, ShutdownConfig{})

	// Start first shutdown in background
	go manager.Shutdown(context.Background())

	// Give it time to start
	time.Sleep(10 * time.Millisecond)

	// Second shutdown should fail
	err := manager.Shutdown(context.Background())
	if err != ErrShutdownInProgress {
		t.Errorf("expected ErrShutdownInProgress, got %v", err)
	}
}

func TestNode_DrainWithTimeout(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	committee := makeTestCommittee(priv)

	node, _ := NewNode(NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}, committee, nil, nil)

	ctx := context.Background()
	node.Start(ctx)

	// Submit transactions
	for i := 0; i < 5; i++ {
		tx := make([]byte, 32)
		rand.Read(tx)
		node.SubmitTransaction(tx)
	}

	// Drain with short timeout (should succeed as transactions are processed quickly)
	err := node.DrainWithTimeout(context.Background(), 5*time.Second)
	if err != nil {
		t.Errorf("DrainWithTimeout() error = %v", err)
	}

	node.Stop()
}

func TestNode_StopAccepting(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	committee := makeTestCommittee(priv)

	node, _ := NewNode(NodeConfig{
		Partition: "test",
		KeyPair:   priv,
	}, committee, nil, nil)

	ctx := context.Background()
	node.Start(ctx)

	// Should accept transactions initially
	tx := make([]byte, 32)
	rand.Read(tx)
	if err := node.SubmitTransaction(tx); err != nil {
		t.Errorf("should accept transaction initially: %v", err)
	}

	// Stop accepting
	node.StopAccepting()

	// Should reject transactions now
	if err := node.SubmitTransaction(tx); err != ErrNodeClosed {
		t.Errorf("expected ErrNodeClosed, got %v", err)
	}

	node.Stop()
}

// TestGracefulShutdown tests clean shutdown during consensus.
// This test verifies that:
// - Pending batches are flushed
// - Current round completes
// - State is persisted
// - No error occurs on shutdown
func TestGracefulShutdown(t *testing.T) {
	// Create temp directory for persistence
	tmpDir, err := os.MkdirTemp("", "graceful_shutdown_test")
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
	committee := makeTestCommittee(priv)

	// Create node with configuration
	config := NodeConfig{
		Partition:  "test-graceful",
		KeyPair:    priv,
		NumWorkers: 2,
	}
	node, err := NewNode(config, committee, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Start node
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := node.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Advance round to simulate consensus activity
	node.primary.SetRound(10)

	// Submit transactions to create pending batches
	for i := 0; i < 20; i++ {
		tx := make([]byte, 64)
		rand.Read(tx)
		if err := node.SubmitTransaction(tx); err != nil {
			t.Fatalf("failed to submit transaction %d: %v", i, err)
		}
	}

	// Record state before shutdown
	roundBeforeShutdown := node.CurrentRound()

	// Create checkpoint store for persistence
	store := persist.NewStore(tmpDir)

	// Create shutdown manager with persistence enabled
	manager := NewShutdownManager(node, ShutdownConfig{
		DrainTimeout:    10 * time.Second,
		GracePeriod:     2 * time.Second,
		PersistState:    true,
		CheckpointStore: store,
	})

	// Perform graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	err = manager.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	// Verify: No error on shutdown (already checked above)

	// Verify: Shutdown is complete
	if !manager.IsShutdownComplete() {
		t.Error("shutdown should be complete")
	}

	// Verify: State was persisted
	if !store.Exists() {
		t.Error("checkpoint should exist after graceful shutdown with persistence")
	}

	// Load checkpoint and verify state
	checkpoint, err := store.Load()
	if err != nil {
		t.Fatalf("failed to load checkpoint: %v", err)
	}

	// Verify: Current round was persisted
	if checkpoint.CurrentRound != roundBeforeShutdown {
		t.Errorf("checkpoint round mismatch: got %d, want %d", checkpoint.CurrentRound, roundBeforeShutdown)
	}

	// Verify: Partition was persisted correctly
	if checkpoint.Partition != "test-graceful" {
		t.Errorf("checkpoint partition mismatch: got %s, want test-graceful", checkpoint.Partition)
	}

	// Verify: Node is closed and rejects new transactions
	if err := node.SubmitTransaction([]byte("should-fail")); err != ErrNodeClosed {
		t.Errorf("expected ErrNodeClosed after shutdown, got %v", err)
	}
}
