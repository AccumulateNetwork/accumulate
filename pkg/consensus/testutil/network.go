// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package testutil provides testing utilities for the consensus package.
package testutil

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"sync"
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// TestNetwork holds multiple consensus nodes for testing.
type TestNetwork struct {
	t         *testing.T
	ctx       context.Context
	cancel    context.CancelFunc
	nodes     []*TestNode
	committee *types.Committee
	keys      []ed25519.PrivateKey
	partition string
}

// TestNode wraps a consensus Node with test-specific state.
type TestNode struct {
	Node     *consensus.Node
	Host     host.Host
	PubSub   *pubsub.PubSub
	KeyPair  ed25519.PrivateKey
	Index    int
	Commits  []*types.Certificate
	mu       sync.Mutex
	stopped  bool
}

// NetworkOption configures a TestNetwork.
type NetworkOption func(*networkConfig)

type networkConfig struct {
	stakes          []uint64
	numWorkers      int
	workerConfig    worker.Config
	partition       string
	useGossip       bool
	meshWaitTime    time.Duration
	batchSize       int
	batchTimeout    time.Duration
}

// WithStakes sets custom stake distribution for validators.
func WithStakes(stakes []uint64) NetworkOption {
	return func(c *networkConfig) {
		c.stakes = stakes
	}
}

// WithNumWorkers sets the number of workers per node.
func WithNumWorkers(n int) NetworkOption {
	return func(c *networkConfig) {
		c.numWorkers = n
	}
}

// WithPartition sets the partition name.
func WithPartition(partition string) NetworkOption {
	return func(c *networkConfig) {
		c.partition = partition
	}
}

// WithGossip enables or disables gossip networking.
func WithGossip(enabled bool) NetworkOption {
	return func(c *networkConfig) {
		c.useGossip = enabled
	}
}

// WithMeshWaitTime sets the time to wait for gossip mesh to form.
func WithMeshWaitTime(d time.Duration) NetworkOption {
	return func(c *networkConfig) {
		c.meshWaitTime = d
	}
}

// WithBatchSize sets the batch size for workers.
func WithBatchSize(size int) NetworkOption {
	return func(c *networkConfig) {
		c.batchSize = size
	}
}

// WithBatchTimeout sets the batch timeout for workers.
func WithBatchTimeout(d time.Duration) NetworkOption {
	return func(c *networkConfig) {
		c.batchTimeout = d
	}
}

// NewTestNetwork creates a new test network with the specified number of nodes.
func NewTestNetwork(t *testing.T, numNodes int, opts ...NetworkOption) *TestNetwork {
	t.Helper()

	cfg := networkConfig{
		numWorkers:   1,
		partition:    "test",
		useGossip:    true,
		meshWaitTime: 500 * time.Millisecond,
		batchSize:    10,
		batchTimeout: 50 * time.Millisecond,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	// Generate keys and create validators
	keys := make([]ed25519.PrivateKey, numNodes)
	validators := make([]types.ValidatorInfo, numNodes)

	for i := 0; i < numNodes; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys[i] = priv

		stake := uint64(100)
		if len(cfg.stakes) > i {
			stake = cfg.stakes[i]
		}

		validators[i] = types.ValidatorInfo{
			PublicKey: pub,
			Stake:     stake,
		}
	}

	committee := types.NewCommittee(validators, 1)

	ctx, cancel := context.WithCancel(context.Background())

	tn := &TestNetwork{
		t:         t,
		ctx:       ctx,
		cancel:    cancel,
		nodes:     make([]*TestNode, numNodes),
		committee: committee,
		keys:      keys,
		partition: cfg.partition,
	}

	// Create hosts and pubsub instances if gossip is enabled
	var hosts []host.Host
	var pubsubs []*pubsub.PubSub

	if cfg.useGossip {
		hosts = make([]host.Host, numNodes)
		pubsubs = make([]*pubsub.PubSub, numNodes)

		// Create hosts
		for i := 0; i < numNodes; i++ {
			netw := swarmt.GenSwarm(t)
			h := bhost.NewBlankHost(netw)
			hosts[i] = h
			t.Cleanup(func() { _ = h.Close() })
		}

		// Connect all hosts to each other
		for i := 0; i < numNodes; i++ {
			for j := i + 1; j < numNodes; j++ {
				pi := peer.AddrInfo{
					ID:    hosts[j].ID(),
					Addrs: hosts[j].Addrs(),
				}
				err := hosts[i].Connect(ctx, pi)
				require.NoError(t, err, "failed to connect hosts %d and %d", i, j)
			}
		}

		// Create pubsub instances
		for i := 0; i < numNodes; i++ {
			ps, err := pubsub.NewGossipSub(ctx, hosts[i])
			require.NoError(t, err)
			pubsubs[i] = ps
		}
	}

	// Create nodes
	for i := 0; i < numNodes; i++ {
		var h host.Host
		var ps *pubsub.PubSub

		if cfg.useGossip {
			h = hosts[i]
			ps = pubsubs[i]
		}

		nodeCfg := consensus.NodeConfig{
			Partition:  cfg.partition,
			KeyPair:    keys[i],
			NumWorkers: cfg.numWorkers,
			WorkerConfig: worker.Config{
				BatchSize:    cfg.batchSize,
				BatchTimeout: cfg.batchTimeout,
			},
		}

		node, err := consensus.NewNode(nodeCfg, committee, h, ps)
		require.NoError(t, err)

		tn.nodes[i] = &TestNode{
			Node:    node,
			Host:    h,
			PubSub:  ps,
			KeyPair: keys[i],
			Index:   i,
			Commits: make([]*types.Certificate, 0),
		}
	}

	return tn
}

// Start starts all nodes in the network.
func (tn *TestNetwork) Start() error {
	// Insert genesis certificates
	for _, n := range tn.nodes {
		if err := n.Node.InsertGenesisForAll(tn.keys); err != nil {
			return fmt.Errorf("insert genesis for node %d: %w", n.Index, err)
		}
	}

	// Start all nodes
	for _, n := range tn.nodes {
		n := n // capture
		go func() {
			_ = n.Node.Start(tn.ctx)
		}()

		// Start commit collector
		go tn.collectCommits(n)
	}

	// Wait for nodes to start and gossip mesh to form
	// This is critical for proper timing - nodes need time to:
	// 1. Start their gossip layers
	// 2. Form the gossip mesh
	// 3. Be ready to receive messages
	if tn.nodes[0].Host != nil {
		// Give extra time for gossip mesh to stabilize
		time.Sleep(1 * time.Second)
	} else {
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// Stop stops all nodes and cleans up resources.
func (tn *TestNetwork) Stop() {
	tn.cancel()

	for _, n := range tn.nodes {
		n.mu.Lock()
		n.stopped = true
		n.mu.Unlock()
		n.Node.Stop()
	}
}

// SubmitTransaction submits a transaction to the specified node.
func (tn *TestNetwork) SubmitTransaction(nodeIdx int, tx []byte) error {
	if nodeIdx < 0 || nodeIdx >= len(tn.nodes) {
		return fmt.Errorf("invalid node index: %d", nodeIdx)
	}
	return tn.nodes[nodeIdx].Node.SubmitTransaction(tx)
}

// SubmitTransactions submits multiple transactions to nodes in round-robin fashion.
func (tn *TestNetwork) SubmitTransactions(count int, txPrefix string) error {
	for i := 0; i < count; i++ {
		tx := []byte(fmt.Sprintf("%s-%d", txPrefix, i))
		nodeIdx := i % len(tn.nodes)
		if err := tn.SubmitTransaction(nodeIdx, tx); err != nil {
			return fmt.Errorf("submit tx %d to node %d: %w", i, nodeIdx, err)
		}
	}
	return nil
}

// WaitForCommits waits until all nodes have at least minCommits committed certificates.
func (tn *TestNetwork) WaitForCommits(minCommits int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(tn.ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for commits: %w", ctx.Err())
		case <-ticker.C:
			allHaveCommits := true
			for _, n := range tn.nodes {
				n.mu.Lock()
				stopped := n.stopped
				count := len(n.Commits)
				n.mu.Unlock()

				if stopped {
					continue
				}

				if count < minCommits {
					allHaveCommits = false
					break
				}
			}
			if allHaveCommits {
				return nil
			}
		}
	}
}

// WaitForRound waits until at least one node reaches the specified round.
func (tn *TestNetwork) WaitForRound(round types.Round, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(tn.ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for round %d: %w", round, ctx.Err())
		case <-ticker.C:
			for _, n := range tn.nodes {
				if n.Node.CurrentRound() >= round {
					return nil
				}
			}
		}
	}
}

// WaitForAllNodesRound waits until all active nodes reach the specified round.
func (tn *TestNetwork) WaitForAllNodesRound(round types.Round, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(tn.ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for all nodes to reach round %d: %w", round, ctx.Err())
		case <-ticker.C:
			allAtRound := true
			for _, n := range tn.nodes {
				n.mu.Lock()
				stopped := n.stopped
				n.mu.Unlock()

				if stopped {
					continue
				}

				if n.Node.CurrentRound() < round {
					allAtRound = false
					break
				}
			}
			if allAtRound {
				return nil
			}
		}
	}
}

// GetNode returns the test node at the given index.
func (tn *TestNetwork) GetNode(idx int) *TestNode {
	if idx < 0 || idx >= len(tn.nodes) {
		return nil
	}
	return tn.nodes[idx]
}

// GetAllNodes returns all test nodes.
func (tn *TestNetwork) GetAllNodes() []*TestNode {
	return tn.nodes
}

// Committee returns the committee.
func (tn *TestNetwork) Committee() *types.Committee {
	return tn.committee
}

// Keys returns all private keys.
func (tn *TestNetwork) Keys() []ed25519.PrivateKey {
	return tn.keys
}

// Partition returns the partition name.
func (tn *TestNetwork) Partition() string {
	return tn.partition
}

// Context returns the network context.
func (tn *TestNetwork) Context() context.Context {
	return tn.ctx
}

// NumNodes returns the number of nodes.
func (tn *TestNetwork) NumNodes() int {
	return len(tn.nodes)
}

// StopNode stops a specific node (simulates failure).
func (tn *TestNetwork) StopNode(idx int) {
	if idx < 0 || idx >= len(tn.nodes) {
		return
	}

	n := tn.nodes[idx]
	n.mu.Lock()
	n.stopped = true
	n.mu.Unlock()
	n.Node.Stop()
}

// collectCommits reads committed certificates from a node.
func (tn *TestNetwork) collectCommits(n *TestNode) {
	for {
		select {
		case <-tn.ctx.Done():
			return
		case cert, ok := <-n.Node.Committed():
			if !ok {
				return
			}
			n.mu.Lock()
			n.Commits = append(n.Commits, cert)
			n.mu.Unlock()
		}
	}
}

// GetCommits returns a copy of the committed certificates for a node.
func (n *TestNode) GetCommits() []*types.Certificate {
	n.mu.Lock()
	defer n.mu.Unlock()

	result := make([]*types.Certificate, len(n.Commits))
	copy(result, n.Commits)
	return result
}

// CommitCount returns the number of committed certificates.
func (n *TestNode) CommitCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.Commits)
}

// IsStopped returns true if the node has been stopped.
func (n *TestNode) IsStopped() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.stopped
}
