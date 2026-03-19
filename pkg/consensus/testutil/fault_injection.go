// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testutil

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// FaultInjector provides network fault injection capabilities for testing.
// It can simulate packet loss, latency, partitions, and node isolation.
type FaultInjector struct {
	mu           sync.RWMutex
	network      *TestNetwork
	packetLoss   map[int]float64   // node index -> loss probability (0-1)
	latency      map[int]time.Duration // node index -> latency to add
	partitions   [][]int           // groups of node indices that can communicate
	isolated     map[int]bool      // node index -> true if isolated
	rng          *rand.Rand
	interceptors map[int]network.Notifiee // node index -> interceptor
}

// NewFaultInjector creates a new FaultInjector for the given test network.
func NewFaultInjector(tn *TestNetwork) *FaultInjector {
	return &FaultInjector{
		network:      tn,
		packetLoss:   make(map[int]float64),
		latency:      make(map[int]time.Duration),
		isolated:     make(map[int]bool),
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
		interceptors: make(map[int]network.Notifiee),
	}
}

// SetPacketLoss sets the packet loss probability for a node (0-1).
// A value of 0.1 means 10% of messages will be dropped.
func (fi *FaultInjector) SetPacketLoss(nodeIdx int, probability float64) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	if probability <= 0 {
		delete(fi.packetLoss, nodeIdx)
	} else {
		fi.packetLoss[nodeIdx] = probability
	}
}

// SetGlobalPacketLoss sets packet loss for all nodes.
func (fi *FaultInjector) SetGlobalPacketLoss(probability float64) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	for i := 0; i < fi.network.NumNodes(); i++ {
		if probability <= 0 {
			delete(fi.packetLoss, i)
		} else {
			fi.packetLoss[i] = probability
		}
	}
}

// SetLatency sets the latency to add for messages to/from a node.
func (fi *FaultInjector) SetLatency(nodeIdx int, latency time.Duration) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	if latency <= 0 {
		delete(fi.latency, nodeIdx)
	} else {
		fi.latency[nodeIdx] = latency
	}
}

// SetGlobalLatency sets latency for all nodes.
func (fi *FaultInjector) SetGlobalLatency(latency time.Duration) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	for i := 0; i < fi.network.NumNodes(); i++ {
		if latency <= 0 {
			delete(fi.latency, i)
		} else {
			fi.latency[i] = latency
		}
	}
}

// SetPartitions sets network partitions. Each slice represents a group of
// nodes that can communicate with each other but not with other groups.
// Pass nil to clear partitions.
func (fi *FaultInjector) SetPartitions(partitions [][]int) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	fi.partitions = partitions
}

// ClearPartitions removes all network partitions.
func (fi *FaultInjector) ClearPartitions() {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	fi.partitions = nil
}

// IsolateNode isolates a node from all network communication.
func (fi *FaultInjector) IsolateNode(nodeIdx int) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	fi.isolated[nodeIdx] = true
	fi.applyIsolation(nodeIdx)
}

// ReconnectNode reconnects a previously isolated node.
func (fi *FaultInjector) ReconnectNode(nodeIdx int) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	delete(fi.isolated, nodeIdx)
	fi.reconnect(nodeIdx)
}

// applyIsolation disconnects a node from all peers.
func (fi *FaultInjector) applyIsolation(nodeIdx int) {
	node := fi.network.GetNode(nodeIdx)
	if node == nil || node.Host == nil {
		return
	}

	// Disconnect from all peers
	for _, p := range node.Host.Network().Peers() {
		_ = node.Host.Network().ClosePeer(p)
	}
}

// reconnect re-establishes connections for a node.
func (fi *FaultInjector) reconnect(nodeIdx int) {
	node := fi.network.GetNode(nodeIdx)
	if node == nil || node.Host == nil {
		return
	}

	ctx := fi.network.Context()

	// Reconnect to all other non-isolated nodes
	for i, other := range fi.network.GetAllNodes() {
		if i == nodeIdx || other.Host == nil {
			continue
		}

		if fi.isolated[i] {
			continue
		}

		pi := peer.AddrInfo{
			ID:    other.Host.ID(),
			Addrs: other.Host.Addrs(),
		}
		_ = node.Host.Connect(ctx, pi)
	}
}

// ShouldDropMessage returns true if a message to/from the given node should be dropped.
func (fi *FaultInjector) ShouldDropMessage(nodeIdx int) bool {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	if fi.isolated[nodeIdx] {
		return true
	}

	loss, ok := fi.packetLoss[nodeIdx]
	if !ok || loss <= 0 {
		return false
	}

	return fi.rng.Float64() < loss
}

// GetLatency returns the latency configured for a node.
func (fi *FaultInjector) GetLatency(nodeIdx int) time.Duration {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	return fi.latency[nodeIdx]
}

// CanCommunicate returns true if two nodes can communicate given current partitions.
func (fi *FaultInjector) CanCommunicate(nodeA, nodeB int) bool {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	// Check isolation
	if fi.isolated[nodeA] || fi.isolated[nodeB] {
		return false
	}

	// No partitions means everyone can communicate
	if len(fi.partitions) == 0 {
		return true
	}

	// Find which partition each node is in
	partitionA := -1
	partitionB := -1

	for i, partition := range fi.partitions {
		for _, n := range partition {
			if n == nodeA {
				partitionA = i
			}
			if n == nodeB {
				partitionB = i
			}
		}
	}

	// Nodes in the same partition can communicate
	return partitionA == partitionB && partitionA >= 0
}

// ApplyPartitions applies the current partition configuration by disconnecting
// nodes that should not communicate.
func (fi *FaultInjector) ApplyPartitions() {
	fi.mu.RLock()
	partitions := fi.partitions
	fi.mu.RUnlock()

	if len(partitions) == 0 {
		return
	}

	nodes := fi.network.GetAllNodes()

	// For each pair of nodes, disconnect if they're in different partitions
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			if !fi.CanCommunicate(i, j) {
				fi.disconnectPeers(i, j)
			}
		}
	}
}

// HealPartitions reconnects all partitioned nodes.
func (fi *FaultInjector) HealPartitions() {
	fi.mu.Lock()
	fi.partitions = nil
	fi.mu.Unlock()

	nodes := fi.network.GetAllNodes()
	ctx := fi.network.Context()

	// Reconnect all non-isolated nodes
	for i := 0; i < len(nodes); i++ {
		if nodes[i].Host == nil {
			continue
		}

		fi.mu.RLock()
		isolated := fi.isolated[i]
		fi.mu.RUnlock()

		if isolated {
			continue
		}

		for j := i + 1; j < len(nodes); j++ {
			if nodes[j].Host == nil {
				continue
			}

			fi.mu.RLock()
			jIsolated := fi.isolated[j]
			fi.mu.RUnlock()

			if jIsolated {
				continue
			}

			// Reconnect
			pi := peer.AddrInfo{
				ID:    nodes[j].Host.ID(),
				Addrs: nodes[j].Host.Addrs(),
			}
			_ = nodes[i].Host.Connect(ctx, pi)
		}
	}
}

// disconnectPeers disconnects two peers from each other.
func (fi *FaultInjector) disconnectPeers(nodeA, nodeB int) {
	nodes := fi.network.GetAllNodes()

	if nodeA >= len(nodes) || nodeB >= len(nodes) {
		return
	}

	hostA := nodes[nodeA].Host
	hostB := nodes[nodeB].Host

	if hostA == nil || hostB == nil {
		return
	}

	// Close connections in both directions
	_ = hostA.Network().ClosePeer(hostB.ID())
	_ = hostB.Network().ClosePeer(hostA.ID())
}

// Reset clears all fault injection settings.
func (fi *FaultInjector) Reset() {
	fi.mu.Lock()
	fi.packetLoss = make(map[int]float64)
	fi.latency = make(map[int]time.Duration)
	fi.partitions = nil
	fi.isolated = make(map[int]bool)
	fi.mu.Unlock()

	// Reconnect all nodes
	fi.HealPartitions()
}

// Stats returns current fault injection statistics.
func (fi *FaultInjector) Stats() FaultStats {
	fi.mu.RLock()
	defer fi.mu.RUnlock()

	stats := FaultStats{
		NodesWithPacketLoss: len(fi.packetLoss),
		NodesWithLatency:    len(fi.latency),
		IsolatedNodes:       len(fi.isolated),
		NumPartitions:       len(fi.partitions),
	}

	for _, loss := range fi.packetLoss {
		stats.AvgPacketLoss += loss
	}
	if len(fi.packetLoss) > 0 {
		stats.AvgPacketLoss /= float64(len(fi.packetLoss))
	}

	for _, lat := range fi.latency {
		stats.AvgLatency += lat
	}
	if len(fi.latency) > 0 {
		stats.AvgLatency /= time.Duration(len(fi.latency))
	}

	return stats
}

// FaultStats contains statistics about current fault injection.
type FaultStats struct {
	NodesWithPacketLoss int
	NodesWithLatency    int
	IsolatedNodes       int
	NumPartitions       int
	AvgPacketLoss       float64
	AvgLatency          time.Duration
}

// FaultyNetwork wraps a TestNetwork with fault injection capabilities.
type FaultyNetwork struct {
	*TestNetwork
	Injector *FaultInjector
}

// NewFaultyNetwork creates a test network with fault injection capabilities.
func NewFaultyNetwork(tn *TestNetwork) *FaultyNetwork {
	return &FaultyNetwork{
		TestNetwork: tn,
		Injector:    NewFaultInjector(tn),
	}
}

// WithPacketLoss returns a FaultyNetwork configured with global packet loss.
func (fn *FaultyNetwork) WithPacketLoss(probability float64) *FaultyNetwork {
	fn.Injector.SetGlobalPacketLoss(probability)
	return fn
}

// WithLatency returns a FaultyNetwork configured with global latency.
func (fn *FaultyNetwork) WithLatency(latency time.Duration) *FaultyNetwork {
	fn.Injector.SetGlobalLatency(latency)
	return fn
}

// WithPartitions returns a FaultyNetwork configured with network partitions.
func (fn *FaultyNetwork) WithPartitions(partitions [][]int) *FaultyNetwork {
	fn.Injector.SetPartitions(partitions)
	fn.Injector.ApplyPartitions()
	return fn
}

// IsolateNode isolates a specific node.
func (fn *FaultyNetwork) IsolateNode(nodeIdx int) *FaultyNetwork {
	fn.Injector.IsolateNode(nodeIdx)
	return fn
}

// ReconnectNode reconnects a previously isolated node.
func (fn *FaultyNetwork) ReconnectNode(nodeIdx int) *FaultyNetwork {
	fn.Injector.ReconnectNode(nodeIdx)
	return fn
}

// Heal removes all fault injections and reconnects nodes.
func (fn *FaultyNetwork) Heal() *FaultyNetwork {
	fn.Injector.Reset()
	return fn
}

// WaitForRecovery waits for the network to recover after healing.
func (fn *FaultyNetwork) WaitForRecovery(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Wait for gossip mesh to reform
	time.Sleep(500 * time.Millisecond)

	// Wait for nodes to sync
	return fn.WaitForAllNodesInSync(ctx, timeout)
}

// WaitForAllNodesInSync waits until all active nodes are at similar rounds.
func (fn *FaultyNetwork) WaitForAllNodesInSync(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			minRound := fn.GetMinRound()
			maxRound := fn.GetMaxRound()

			// Allow some variance (up to 3 rounds difference)
			if maxRound-minRound <= 3 {
				return nil
			}
		}
	}
}

// ConnectionInterceptor intercepts network connections for fault injection.
type ConnectionInterceptor struct {
	host      host.Host
	injector  *FaultInjector
	nodeIdx   int
	origConns map[peer.ID]network.Conn
}

// NewConnectionInterceptor creates a new connection interceptor.
func NewConnectionInterceptor(h host.Host, fi *FaultInjector, nodeIdx int) *ConnectionInterceptor {
	return &ConnectionInterceptor{
		host:      h,
		injector:  fi,
		nodeIdx:   nodeIdx,
		origConns: make(map[peer.ID]network.Conn),
	}
}

// Listen is called when the network starts listening on an address.
func (ci *ConnectionInterceptor) Listen(network.Network, multiaddr.Multiaddr) {}

// ListenClose is called when the network stops listening on an address.
func (ci *ConnectionInterceptor) ListenClose(network.Network, multiaddr.Multiaddr) {}

// Connected is called when a connection is established.
func (ci *ConnectionInterceptor) Connected(n network.Network, conn network.Conn) {
	// Check if we should block this connection based on partitions
	// This would require mapping peer IDs to node indices
}

// Disconnected is called when a connection is closed.
func (ci *ConnectionInterceptor) Disconnected(network.Network, network.Conn) {}
