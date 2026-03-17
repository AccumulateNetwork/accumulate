// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testutil

import (
	"errors"
	"math/rand"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// MockP2PNetwork connects multiple MockHosts for testing.
// It enables simulation of network conditions like latency and packet loss.
type MockP2PNetwork struct {
	mu         sync.RWMutex
	hosts      map[peer.ID]*MockHost
	partitions map[peer.ID]int // Maps peer to partition group

	// Fault injection settings
	packetLoss float64       // Probability of dropping a message (0.0-1.0)
	latency    time.Duration // Artificial latency to add

	// Message tracking
	messageLog []NetworkMessage
}

// NetworkMessage records a message sent through the network.
type NetworkMessage struct {
	From      peer.ID
	To        peer.ID
	Protocol  protocol.ID
	Data      []byte
	Timestamp time.Time
	Dropped   bool
}

// NewMockP2PNetwork creates a new mock network.
func NewMockP2PNetwork() *MockP2PNetwork {
	return &MockP2PNetwork{
		hosts:      make(map[peer.ID]*MockHost),
		partitions: make(map[peer.ID]int),
	}
}

// AddHost adds a host to the network.
func (n *MockP2PNetwork) AddHost(h *MockHost) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.hosts[h.ID()] = h
	n.partitions[h.ID()] = 0 // Default partition
}

// RemoveHost removes a host from the network.
func (n *MockP2PNetwork) RemoveHost(id peer.ID) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.hosts, id)
	delete(n.partitions, id)
}

// GetHost returns the host with the given ID.
func (n *MockP2PNetwork) GetHost(id peer.ID) *MockHost {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.hosts[id]
}

// GetAllHosts returns all hosts in the network.
func (n *MockP2PNetwork) GetAllHosts() []*MockHost {
	n.mu.RLock()
	defer n.mu.RUnlock()
	hosts := make([]*MockHost, 0, len(n.hosts))
	for _, h := range n.hosts {
		hosts = append(hosts, h)
	}
	return hosts
}

// Connect creates a bidirectional connection between two hosts.
func (n *MockP2PNetwork) Connect(a, b peer.ID) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	hostA, okA := n.hosts[a]
	hostB, okB := n.hosts[b]

	if !okA {
		return errors.New("host A not found")
	}
	if !okB {
		return errors.New("host B not found")
	}

	// Create connections in both directions
	hostA.mu.Lock()
	hostA.connections[b] = &MockConn{
		localPeer:  a,
		remotePeer: b,
		streams:    make([]*MockStream, 0),
	}
	hostA.mu.Unlock()

	hostB.mu.Lock()
	hostB.connections[a] = &MockConn{
		localPeer:  b,
		remotePeer: a,
		streams:    make([]*MockStream, 0),
	}
	hostB.mu.Unlock()

	return nil
}

// Disconnect removes the connection between two hosts.
func (n *MockP2PNetwork) Disconnect(a, b peer.ID) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	hostA, okA := n.hosts[a]
	hostB, okB := n.hosts[b]

	if okA {
		hostA.mu.Lock()
		delete(hostA.connections, b)
		hostA.mu.Unlock()
	}

	if okB {
		hostB.mu.Lock()
		delete(hostB.connections, a)
		hostB.mu.Unlock()
	}

	return nil
}

// ConnectAll creates connections between all hosts.
func (n *MockP2PNetwork) ConnectAll() error {
	n.mu.RLock()
	peerIDs := make([]peer.ID, 0, len(n.hosts))
	for id := range n.hosts {
		peerIDs = append(peerIDs, id)
	}
	n.mu.RUnlock()

	for i := 0; i < len(peerIDs); i++ {
		for j := i + 1; j < len(peerIDs); j++ {
			if err := n.Connect(peerIDs[i], peerIDs[j]); err != nil {
				return err
			}
		}
	}
	return nil
}

// SetPacketLoss sets the packet loss rate (0.0-1.0).
func (n *MockP2PNetwork) SetPacketLoss(rate float64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.packetLoss = rate
}

// SetLatency sets the artificial latency.
func (n *MockP2PNetwork) SetLatency(d time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.latency = d
}

// SendMessage simulates sending a message with optional fault injection.
func (n *MockP2PNetwork) SendMessage(from, to peer.ID, proto protocol.ID, data []byte) error {
	n.mu.RLock()
	packetLoss := n.packetLoss
	latency := n.latency
	fromPartition := n.partitions[from]
	toPartition := n.partitions[to]
	targetHost := n.hosts[to]
	n.mu.RUnlock()

	msg := NetworkMessage{
		From:      from,
		To:        to,
		Protocol:  proto,
		Data:      data,
		Timestamp: time.Now(),
	}

	// Check partition
	if fromPartition != toPartition {
		msg.Dropped = true
		n.recordMessage(msg)
		return errors.New("peers are in different partitions")
	}

	// Apply packet loss
	if packetLoss > 0 && rand.Float64() < packetLoss {
		msg.Dropped = true
		n.recordMessage(msg)
		return nil // Silently dropped
	}

	// Apply latency
	if latency > 0 {
		time.Sleep(latency)
	}

	n.recordMessage(msg)

	// Deliver to target host's handler
	if targetHost == nil {
		return errors.New("target host not found")
	}

	handler, ok := targetHost.GetHandler(proto)
	if !ok {
		return errors.New("no handler for protocol")
	}

	// Create a stream for the handler
	stream := &MockStream{
		protocolID: proto,
		conn: &MockConn{
			localPeer:  to,
			remotePeer: from,
		},
	}
	stream.WriteData(data)

	// Call handler in a goroutine
	go handler(stream)

	return nil
}

// recordMessage records a message in the log.
func (n *MockP2PNetwork) recordMessage(msg NetworkMessage) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.messageLog = append(n.messageLog, msg)
}

// Partition isolates a set of nodes from the rest.
// Nodes in the isolated set can communicate with each other but not with others.
func (n *MockP2PNetwork) Partition(isolated []peer.ID) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Find next partition number
	maxPartition := 0
	for _, p := range n.partitions {
		if p > maxPartition {
			maxPartition = p
		}
	}

	// Move isolated nodes to new partition
	newPartition := maxPartition + 1
	for _, id := range isolated {
		n.partitions[id] = newPartition
	}
}

// Heal removes all partitions, allowing all nodes to communicate.
func (n *MockP2PNetwork) Heal() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for id := range n.partitions {
		n.partitions[id] = 0
	}
}

// GetPartition returns the partition number for a peer.
func (n *MockP2PNetwork) GetPartition(id peer.ID) int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.partitions[id]
}

// IsConnected returns true if two peers can communicate (same partition and connected).
func (n *MockP2PNetwork) IsConnected(a, b peer.ID) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Check partitions
	if n.partitions[a] != n.partitions[b] {
		return false
	}

	// Check actual connection
	hostA, ok := n.hosts[a]
	if !ok {
		return false
	}

	hostA.mu.RLock()
	_, connected := hostA.connections[b]
	hostA.mu.RUnlock()

	return connected
}

// GetMessageLog returns a copy of the message log.
func (n *MockP2PNetwork) GetMessageLog() []NetworkMessage {
	n.mu.RLock()
	defer n.mu.RUnlock()
	result := make([]NetworkMessage, len(n.messageLog))
	copy(result, n.messageLog)
	return result
}

// ClearMessageLog clears the message log.
func (n *MockP2PNetwork) ClearMessageLog() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.messageLog = nil
}

// MessageCount returns the number of messages sent.
func (n *MockP2PNetwork) MessageCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.messageLog)
}

// DroppedMessageCount returns the number of dropped messages.
func (n *MockP2PNetwork) DroppedMessageCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	count := 0
	for _, msg := range n.messageLog {
		if msg.Dropped {
			count++
		}
	}
	return count
}

// HostCount returns the number of hosts in the network.
func (n *MockP2PNetwork) HostCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.hosts)
}

// Close closes all hosts in the network.
func (n *MockP2PNetwork) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, h := range n.hosts {
		_ = h.Close()
	}
	return nil
}
