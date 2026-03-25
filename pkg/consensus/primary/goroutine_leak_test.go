// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"crypto/ed25519"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestNoGoroutineLeaks verifies that the Primary doesn't leak goroutines.
func TestNoGoroutineLeaks(t *testing.T) {
	// Force GC to clean up any existing goroutines
	runtime.GC()
	time.Sleep(50 * time.Millisecond)

	// Record initial goroutine count
	before := runtime.NumGoroutine()
	t.Logf("Goroutines before: %d", before)

	// Create test primary
	pub, priv, _ := ed25519.GenerateKey(nil)
	committee := createTestCommittee(t, 4)

	config := Config{
		Partition:            "test",
		KeyPair:              priv,
		RoundAdvanceInterval: 50 * time.Millisecond,
		NewCertsChannelSize:  100,
		MinRoundInterval:     10 * time.Millisecond,
	}

	d := dag.NewDAG(10)
	p := New(config, committee, nil, d, nil)

	// Start primary
	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1)
	go func() {
		errChan <- p.Start(ctx)
	}()

	// Give it time to start background goroutines
	time.Sleep(100 * time.Millisecond)

	// Process some headers and votes to trigger goroutines
	for i := 0; i < 10; i++ {
		header := createTestHeader(t, pub, priv, i, committee.Epoch)
		p.OnHeaderReceived(header)

		vote := createTestVote(t, pub, priv, header)
		p.OnVoteReceived(vote)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Stop the primary
	cancel()
	p.Stop()

	// Wait for goroutines to finish
	select {
	case <-errChan:
		// Expected - primary stopped
	case <-time.After(5 * time.Second):
		t.Fatal("Primary didn't stop within timeout")
	}

	// Force GC again
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	// Check goroutine count
	after := runtime.NumGoroutine()
	t.Logf("Goroutines after: %d", after)

	// Allow small delta for background goroutines (test framework, GC, etc.)
	delta := after - before
	require.LessOrEqual(t, delta, 5,
		"Goroutine leak detected: before=%d after=%d delta=%d", before, after, delta)
}

// TestGracefulShutdown verifies that Primary stops cleanly even under load.
func TestGracefulShutdown(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	committee := createTestCommittee(t, 4)

	config := Config{
		Partition:            "test",
		KeyPair:              priv,
		RoundAdvanceInterval: 50 * time.Millisecond,
		NewCertsChannelSize:  100,
		MinRoundInterval:     10 * time.Millisecond,
	}

	d := dag.NewDAG(10)
	p := New(config, committee, nil, d, nil)

	// Start primary
	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1)
	go func() {
		errChan <- p.Start(ctx)
	}()

	// Start processing load
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			header := createTestHeader(t, pub, priv, i, committee.Epoch)
			p.OnHeaderReceived(header)

			vote := createTestVote(t, pub, priv, header)
			p.OnVoteReceived(vote)

			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Stop immediately while processing
	time.Sleep(10 * time.Millisecond)
	cancel()
	p.Stop()

	// Verify shutdown completed within timeout
	select {
	case <-errChan:
		// Expected - primary stopped
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown timeout - goroutines may be stuck")
	}

	// Processing goroutine may or may not complete - that's ok
	select {
	case <-done:
		t.Log("Processing completed")
	default:
		t.Log("Processing interrupted (expected)")
	}
}

// TestContextCancellation verifies goroutines respect context cancellation.
func TestContextCancellation(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	committee := createTestCommittee(t, 4)

	config := Config{
		Partition:            "test",
		KeyPair:              priv,
		RoundAdvanceInterval: 50 * time.Millisecond,
		NewCertsChannelSize:  100,
		MinRoundInterval:     10 * time.Millisecond,
	}

	d := dag.NewDAG(10)
	p := New(config, committee, nil, d, nil)

	// Start with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1)
	go func() {
		errChan <- p.Start(ctx)
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context first
	cancel()

	// Try to trigger goroutines after cancellation
	for i := 0; i < 10; i++ {
		header := createTestHeader(t, pub, priv, i, committee.Epoch)
		p.OnHeaderReceived(header)
	}

	// Stop should complete quickly since context is already cancelled
	stopStart := time.Now()
	p.Stop()
	stopDuration := time.Since(stopStart)

	require.Less(t, stopDuration, time.Second,
		"Stop took too long (%v) - goroutines may not respect context", stopDuration)

	// Verify primary stopped
	select {
	case <-errChan:
		// Expected
	case <-time.After(time.Second):
		t.Fatal("Primary didn't stop after context cancellation")
	}
}

// Helper functions

func createTestHeader(t *testing.T, pub ed25519.PublicKey, priv ed25519.PrivateKey, round int, epoch uint64) *types.Header {
	header := types.NewHeader(pub, types.Round(round), epoch, nil, []types.CertificateDigest{})
	err := header.Sign(priv)
	require.NoError(t, err)
	return header
}

func createTestVote(t *testing.T, pub ed25519.PublicKey, priv ed25519.PrivateKey, header *types.Header) *types.Vote {
	vote := types.NewVote(header.Digest(), header.Round, header.Epoch, pub)
	err := vote.Sign(priv)
	require.NoError(t, err)
	return vote
}

func createTestCommittee(t *testing.T, n int) *types.Committee {
	validators := make([]types.ValidatorInfo, n)
	for i := 0; i < n; i++ {
		pub, _, _ := ed25519.GenerateKey(nil)
		validators[i] = types.ValidatorInfo{
			PublicKey: pub,
			Stake:     100,
		}
	}
	committee := types.NewCommittee(validators, 1)
	return committee
}
