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
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	bhost "github.com/libp2p/go-libp2p/p2p/host/blank"
	swarmt "github.com/libp2p/go-libp2p/p2p/net/swarm/testing"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestExtended_ThirtyMinuteIntegration runs a 30-minute integration test with 7 nodes.
// This test is designed to verify long-term stability and sustained throughput.
//
// Run with: go test -run TestExtended_ThirtyMinuteIntegration -v -timeout 35m
//
// To run for a shorter time during development, set EXTENDED_TEST_DURATION=5m
func TestExtended_ThirtyMinuteIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping extended test in short mode")
	}

	const numNodes = 7
	const targetTPS = 1000 // Target 1000 TPS
	const minAcceptableTPS = 500
	const blockInterval = 200 * time.Millisecond

	// Allow override via environment variable for development
	testDuration := 30 * time.Minute
	if dur := os.Getenv("EXTENDED_TEST_DURATION"); dur != "" {
		if d, err := time.ParseDuration(dur); err == nil {
			testDuration = d
		}
	}

	t.Logf("Starting 7-node integration test for %v at %d TPS target", testDuration, targetTPS)

	ctx, cancel := context.WithTimeout(context.Background(), testDuration+5*time.Minute)
	defer cancel()

	// Track memory
	var memStats runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memStats)
	initialMemory := memStats.Alloc
	t.Logf("Initial memory: %d MB", initialMemory/1024/1024)

	// Generate validator keys
	keys := make([]ed25519.PrivateKey, numNodes)
	validators := make([]types.ValidatorInfo, numNodes)
	pubKeys := make([]ed25519.PublicKey, numNodes)

	for i := 0; i < numNodes; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys[i] = priv
		pubKeys[i] = pub
		validators[i] = types.ValidatorInfo{
			PublicKey: pub,
			Stake:     100,
		}
	}

	committee := types.NewCommittee(validators, 1)

	// Create libp2p hosts
	hosts := make([]host.Host, numNodes)
	pubsubs := make([]*pubsub.PubSub, numNodes)

	for i := 0; i < numNodes; i++ {
		netw := swarmt.GenSwarm(t)
		h := bhost.NewBlankHost(netw)
		hosts[i] = h
		t.Cleanup(func() { _ = h.Close() })
	}

	// Connect all hosts in mesh
	for i := 0; i < numNodes; i++ {
		for j := i + 1; j < numNodes; j++ {
			pi := peer.AddrInfo{
				ID:    hosts[j].ID(),
				Addrs: hosts[j].Addrs(),
			}
			err := hosts[i].Connect(ctx, pi)
			require.NoError(t, err)
		}
	}

	// Create pubsub instances
	for i := 0; i < numNodes; i++ {
		ps, err := pubsub.NewGossipSub(ctx, hosts[i])
		require.NoError(t, err)
		pubsubs[i] = ps
	}

	// Create executors and nodes
	executors := make([]*Executor, numNodes)
	nodes := make([]*consensus.Node, numNodes)

	for i := 0; i < numNodes; i++ {
		exec, err := NewExecutor(ExecutorConfig{
			Validators:    pubKeys,
			BlockInterval: blockInterval,
			TxRate:        uint32(targetTPS / numNodes),
			DataDir:       fmt.Sprintf("/tmp/extended-test-node-%d", i),
		})
		require.NoError(t, err)
		executors[i] = exec
		t.Cleanup(func() { _ = exec.Cleanup() })

		nodeCfg := consensus.NodeConfig{
			Partition:        "extended-test",
			KeyPair:          keys[i],
			NumWorkers:       4,
			CommitBufferSize: 100000,
		}
		node, err := consensus.NewNode(nodeCfg, committee, hosts[i], pubsubs[i])
		require.NoError(t, err)
		nodes[i] = node
	}

	// Initialize genesis
	for i := 0; i < numNodes; i++ {
		err := nodes[i].InsertGenesisForAll(keys)
		require.NoError(t, err)
	}

	// Wait for mesh
	time.Sleep(3 * time.Second)

	// Start all nodes
	for i := 0; i < numNodes; i++ {
		err := nodes[i].Start(ctx)
		require.NoError(t, err)
	}

	// Start executors and commit processors
	var wg sync.WaitGroup
	for i := 0; i < numNodes; i++ {
		i := i
		executors[i].Start(ctx)

		wg.Add(1)
		go func() {
			defer wg.Done()
			committed := nodes[i].Committed()
			workers := nodes[i].Workers()
			for {
				select {
				case <-ctx.Done():
					return
				case cert, ok := <-committed:
					if !ok {
						return
					}
					if cert != nil {
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
						executors[i].ProcessCertificate(cert, batches)
						for _, w := range workers {
							w.PruneBatches(digests)
						}
					}
				}
			}
		}()
	}

	// Transaction submission - high throughput
	var submitted atomic.Uint64
	numGenerators := 20
	ratePerGenerator := targetTPS / numNodes / numGenerators

	for i := 0; i < numNodes; i++ {
		for g := 0; g < numGenerators; g++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(time.Second / time.Duration(ratePerGenerator))
				defer ticker.Stop()

				payload := make([]byte, 256)
				_, _ = rand.Read(payload)

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						nonce := submitted.Add(1)
						tx := NewDataTx(pubKeys[i], payload, nonce)
						tx.Sign(keys[i])
						_ = nodes[i].SubmitTransaction(tx.Marshal())
					}
				}
			}()
		}
	}

	// Progress reporter - every minute
	reportDone := make(chan struct{})
	go func() {
		defer close(reportDone)
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		startTime := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)

				blocks := make([]uint64, numNodes)
				for i := 0; i < numNodes; i++ {
					blocks[i] = executors[i].GetBlockCount()
				}

				elapsed := time.Since(startTime)
				remaining := testDuration - elapsed

				t.Logf("[%v elapsed, %v remaining] Memory: %dMB, Blocks: %v, Submitted: %d",
					elapsed.Round(time.Second),
					remaining.Round(time.Second),
					m.Alloc/1024/1024,
					blocks,
					submitted.Load())
			}
		}
	}()

	// Wait for test duration
	select {
	case <-ctx.Done():
	case <-time.After(testDuration):
	}

	// Stop and collect results
	cancel()
	<-reportDone

	// Final statistics
	runtime.GC()
	runtime.ReadMemStats(&memStats)
	finalMemory := memStats.Alloc
	memoryGrowth := int64(finalMemory) - int64(initialMemory)

	t.Log("=== EXTENDED TEST RESULTS ===")
	t.Logf("Test duration: %v", testDuration)
	t.Logf("Transactions submitted: %d", submitted.Load())
	t.Logf("Submission TPS: %.2f", float64(submitted.Load())/testDuration.Seconds())
	t.Logf("Initial memory: %d MB", initialMemory/1024/1024)
	t.Logf("Final memory: %d MB", finalMemory/1024/1024)
	t.Logf("Memory growth: %d MB", memoryGrowth/1024/1024)

	// Check block heights
	maxBlocks := uint64(0)
	minBlocks := uint64(^uint64(0))
	for i := 0; i < numNodes; i++ {
		blocks := executors[i].GetBlockCount()
		t.Logf("Node %d blocks: %d", i, blocks)
		if blocks > maxBlocks {
			maxBlocks = blocks
		}
		if blocks < minBlocks {
			minBlocks = blocks
		}
	}

	// Check state hashes
	stateHashes := make(map[string]int)
	for i := 0; i < numNodes; i++ {
		h := executors[i].GetStateHash()
		hash := fmt.Sprintf("%x", h[:8])
		stateHashes[hash]++
	}

	maxMatch := 0
	for _, count := range stateHashes {
		if count > maxMatch {
			maxMatch = count
		}
	}
	t.Logf("State hash agreement: %d/%d nodes", maxMatch, numNodes)

	// Stop nodes
	for _, node := range nodes {
		node.Stop()
	}

	// Verify success criteria
	actualTPS := float64(submitted.Load()) / testDuration.Seconds()
	t.Logf("Actual submission TPS: %.2f (target: %d, minimum: %d)", actualTPS, targetTPS, minAcceptableTPS)

	// Memory should not grow excessively (less than 1GB)
	if memoryGrowth > 1024*1024*1024 {
		t.Errorf("Memory growth exceeded 1GB: %d MB", memoryGrowth/1024/1024)
	}

	// Block heights should be within range (all nodes progressing)
	if maxBlocks-minBlocks > 100 {
		t.Errorf("Block height divergence too large: max=%d, min=%d", maxBlocks, minBlocks)
	}

	// State hash consensus (at least 5/7 should agree)
	if maxMatch < 5 {
		t.Errorf("Insufficient state hash consensus: only %d/7 nodes agree", maxMatch)
	}

	t.Log("=== EXTENDED TEST COMPLETE ===")
}
