// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testutil

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// ClusterNode wraps a consensus node with test utilities for cluster testing.
// Unlike TestNode which uses real networking, ClusterNode uses mocks.
type ClusterNode struct {
	mu        sync.Mutex
	Index     int
	Host      *MockHost
	PubSub    *MockPubSub
	Network   *MockP2PNetwork
	Committee *types.Committee
	PrivKey   ed25519.PrivateKey
	PubKey    ed25519.PublicKey
	PeerID    peer.ID
	stopped   bool
	commits   []*types.Certificate
}

// NewClusterNode creates a consensus node with mock networking.
func NewClusterNode(t *testing.T, index int, committee *types.Committee, privKey ed25519.PrivateKey) *ClusterNode {
	t.Helper()

	pubKey := privKey.Public().(ed25519.PublicKey)
	peerID := peerIDFromKey(pubKey)
	host := NewMockHost(peerID)

	return &ClusterNode{
		Index:     index,
		Host:      host,
		Committee: committee,
		PrivKey:   privKey,
		PubKey:    pubKey,
		PeerID:    peerID,
		commits:   make([]*types.Certificate, 0),
	}
}

// peerIDFromKey generates a deterministic peer ID from a public key.
func peerIDFromKey(pubKey ed25519.PublicKey) peer.ID {
	// Create a deterministic peer ID from the public key
	// This is a simplified version for testing
	return peer.ID(pubKey[:])
}

// AddCommit adds a committed certificate.
func (n *ClusterNode) AddCommit(cert *types.Certificate) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.commits = append(n.commits, cert)
}

// GetCommits returns a copy of committed certificates.
func (n *ClusterNode) GetCommits() []*types.Certificate {
	n.mu.Lock()
	defer n.mu.Unlock()
	result := make([]*types.Certificate, len(n.commits))
	copy(result, n.commits)
	return result
}

// CommitCount returns the number of committed certificates.
func (n *ClusterNode) CommitCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.commits)
}

// Stop marks the node as stopped.
func (n *ClusterNode) Stop() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.stopped = true
	_ = n.Host.Close()
}

// IsStopped returns whether the node is stopped.
func (n *ClusterNode) IsStopped() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.stopped
}

// TestCluster manages multiple ClusterNodes for testing consensus scenarios.
type TestCluster struct {
	t         *testing.T
	ctx       context.Context
	cancel    context.CancelFunc
	Nodes     []*ClusterNode
	Network   *MockP2PNetwork
	Committee *types.Committee
	Keys      []ed25519.PrivateKey
	Registry  *MockPubSubRegistry
	mu        sync.Mutex
}

// ClusterConfig holds configuration for a test cluster.
type ClusterConfig struct {
	NumNodes     int
	Stakes       []uint64
	Partition    string
	UseRegistry  bool // Whether to use a shared pubsub registry
}

// DefaultClusterConfig returns default cluster configuration.
func DefaultClusterConfig(n int) ClusterConfig {
	return ClusterConfig{
		NumNodes:    n,
		Partition:   "test",
		UseRegistry: true,
	}
}

// NewTestCluster creates a new test cluster with n nodes using mock networking.
func NewTestCluster(t *testing.T, n int) *TestCluster {
	return NewTestClusterWithConfig(t, DefaultClusterConfig(n))
}

// NewTestClusterWithConfig creates a new test cluster with the specified configuration.
func NewTestClusterWithConfig(t *testing.T, cfg ClusterConfig) *TestCluster {
	t.Helper()

	// Generate keys
	keys := make([]ed25519.PrivateKey, cfg.NumNodes)
	validators := make([]types.ValidatorInfo, cfg.NumNodes)

	for i := 0; i < cfg.NumNodes; i++ {
		keys[i] = generateTestKey(i)
		pubKey := keys[i].Public().(ed25519.PublicKey)

		stake := uint64(100)
		if len(cfg.Stakes) > i {
			stake = cfg.Stakes[i]
		}

		validators[i] = types.ValidatorInfo{
			PublicKey: pubKey,
			Stake:     stake,
		}
	}

	committee := types.NewCommittee(validators, 1)
	require.NoError(t, committee.Validate())

	network := NewMockP2PNetwork()
	ctx, cancel := context.WithCancel(context.Background())

	var registry *MockPubSubRegistry
	if cfg.UseRegistry {
		registry = NewMockPubSubRegistry()
	}

	cluster := &TestCluster{
		t:         t,
		ctx:       ctx,
		cancel:    cancel,
		Nodes:     make([]*ClusterNode, cfg.NumNodes),
		Network:   network,
		Committee: committee,
		Keys:      keys,
		Registry:  registry,
	}

	// Create nodes
	for i := 0; i < cfg.NumNodes; i++ {
		node := NewClusterNode(t, i, committee, keys[i])
		node.Network = network
		network.AddHost(node.Host)

		// Setup pubsub
		if registry != nil {
			node.PubSub = NewMockPubSubWithRegistry(node.PeerID, registry)
		} else {
			node.PubSub = NewMockPubSub(node.PeerID)
		}

		cluster.Nodes[i] = node
	}

	// Connect all nodes
	require.NoError(t, network.ConnectAll())

	return cluster
}

// generateTestKey generates a deterministic test key based on the index.
func generateTestKey(index int) ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	binary.BigEndian.PutUint64(seed, uint64(index))
	return ed25519.NewKeyFromSeed(seed)
}

// GenerateRandomKey generates a random ed25519 key pair.
func GenerateRandomKey() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	return priv, pub, err
}

// Start starts all nodes in the cluster.
func (c *TestCluster) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// This is a placeholder - actual node starting depends on the consensus implementation
	return nil
}

// Stop stops all nodes and cleans up resources.
func (c *TestCluster) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cancel()

	for _, node := range c.Nodes {
		node.Stop()
	}

	_ = c.Network.Close()
}

// GetNode returns the node at the given index.
func (c *TestCluster) GetNode(idx int) *ClusterNode {
	if idx < 0 || idx >= len(c.Nodes) {
		return nil
	}
	return c.Nodes[idx]
}

// StopNode stops a specific node (simulates failure).
func (c *TestCluster) StopNode(idx int) {
	if idx < 0 || idx >= len(c.Nodes) {
		return
	}
	c.Nodes[idx].Stop()
}

// PartitionNode isolates a node from the rest of the network.
func (c *TestCluster) PartitionNode(idx int) {
	if idx < 0 || idx >= len(c.Nodes) {
		return
	}
	c.Network.Partition([]peer.ID{c.Nodes[idx].PeerID})
}

// PartitionNodes isolates multiple nodes from the rest.
func (c *TestCluster) PartitionNodes(indices []int) {
	peerIDs := make([]peer.ID, 0, len(indices))
	for _, idx := range indices {
		if idx >= 0 && idx < len(c.Nodes) {
			peerIDs = append(peerIDs, c.Nodes[idx].PeerID)
		}
	}
	c.Network.Partition(peerIDs)
}

// HealPartition removes all network partitions.
func (c *TestCluster) HealPartition() {
	c.Network.Heal()
}

// SetPacketLoss sets the packet loss rate for the network.
func (c *TestCluster) SetPacketLoss(rate float64) {
	c.Network.SetPacketLoss(rate)
}

// SetLatency sets the network latency.
func (c *TestCluster) SetLatency(d time.Duration) {
	c.Network.SetLatency(d)
}

// WaitForRound waits until all nodes reach the specified round.
func (c *TestCluster) WaitForRound(round types.Round, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(c.ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for round %d: %w", round, ctx.Err())
		case <-ticker.C:
			// This is a placeholder - actual round checking depends on consensus implementation
			return nil
		}
	}
}

// WaitForCommits waits until all active nodes have at least minCommits.
func (c *TestCluster) WaitForCommits(minCommits int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(c.ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for commits: %w", ctx.Err())
		case <-ticker.C:
			allHaveCommits := true
			for _, node := range c.Nodes {
				if node.IsStopped() {
					continue
				}
				if node.CommitCount() < minCommits {
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

// VerifyConsistency checks all nodes have the same committed state.
func (c *TestCluster) VerifyConsistency() error {
	var firstCommits []*types.Certificate
	for _, node := range c.Nodes {
		if node.IsStopped() {
			continue
		}

		commits := node.GetCommits()
		if firstCommits == nil {
			firstCommits = commits
			continue
		}

		// Compare lengths
		minLen := len(firstCommits)
		if len(commits) < minLen {
			minLen = len(commits)
		}

		// Compare digests
		for i := 0; i < minLen; i++ {
			if firstCommits[i].Digest() != commits[i].Digest() {
				return fmt.Errorf("certificate mismatch at index %d", i)
			}
		}
	}

	return nil
}

// ActiveNodeCount returns the number of active (non-stopped) nodes.
func (c *TestCluster) ActiveNodeCount() int {
	count := 0
	for _, node := range c.Nodes {
		if !node.IsStopped() {
			count++
		}
	}
	return count
}

// GetMinCommits returns the minimum number of commits across all active nodes.
func (c *TestCluster) GetMinCommits() int {
	minCommits := -1
	for _, node := range c.Nodes {
		if node.IsStopped() {
			continue
		}
		count := node.CommitCount()
		if minCommits < 0 || count < minCommits {
			minCommits = count
		}
	}
	if minCommits < 0 {
		return 0
	}
	return minCommits
}

// Context returns the cluster context.
func (c *TestCluster) Context() context.Context {
	return c.ctx
}

// NumNodes returns the number of nodes in the cluster.
func (c *TestCluster) NumNodes() int {
	return len(c.Nodes)
}

// GenerateTestCommittee generates a committee with n validators.
func GenerateTestCommittee(n int) (*types.Committee, []ed25519.PrivateKey) {
	keys := make([]ed25519.PrivateKey, n)
	validators := make([]types.ValidatorInfo, n)

	for i := 0; i < n; i++ {
		keys[i] = generateTestKey(i)
		pubKey := keys[i].Public().(ed25519.PublicKey)

		validators[i] = types.ValidatorInfo{
			PublicKey: pubKey,
			Stake:     100,
		}
	}

	return types.NewCommittee(validators, 1), keys
}

// GenerateTestCommitteeWithStakes generates a committee with custom stakes.
func GenerateTestCommitteeWithStakes(stakes []uint64) (*types.Committee, []ed25519.PrivateKey) {
	n := len(stakes)
	keys := make([]ed25519.PrivateKey, n)
	validators := make([]types.ValidatorInfo, n)

	for i := 0; i < n; i++ {
		keys[i] = generateTestKey(i)
		pubKey := keys[i].Public().(ed25519.PublicKey)

		validators[i] = types.ValidatorInfo{
			PublicKey: pubKey,
			Stake:     stakes[i],
		}
	}

	return types.NewCommittee(validators, 1), keys
}
