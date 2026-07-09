// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testutil

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/connmgr"
	ic "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
)

// Compile-time interface checks
var _ host.Host = (*MockHost)(nil)
var _ network.Stream = (*MockStream)(nil)
var _ network.Conn = (*MockConn)(nil)

// MockHost implements host.Host for testing without real networking.
type MockHost struct {
	mu          sync.RWMutex
	id          peer.ID
	addrs       []multiaddr.Multiaddr
	handlers    map[protocol.ID]network.StreamHandler
	connections map[peer.ID]*MockConn
	peerstore   peerstore.Peerstore
	network     *MockNetworkAdapter
	closed      bool
}

// NewMockHost creates a new mock host with the given peer ID.
func NewMockHost(id peer.ID) *MockHost {
	h := &MockHost{
		id:          id,
		handlers:    make(map[protocol.ID]network.StreamHandler),
		connections: make(map[peer.ID]*MockConn),
	}
	h.network = &MockNetworkAdapter{host: h}
	return h
}

// ID returns the host's peer ID.
func (h *MockHost) ID() peer.ID {
	return h.id
}

// Peerstore returns the host's peerstore.
func (h *MockHost) Peerstore() peerstore.Peerstore {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.peerstore
}

// Addrs returns the host's addresses.
func (h *MockHost) Addrs() []multiaddr.Multiaddr {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.addrs
}

// Network returns the host's network interface.
func (h *MockHost) Network() network.Network {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.network
}

// Mux returns nil (not implemented for mock).
func (h *MockHost) Mux() protocol.Switch {
	return nil
}

// Connect connects to a peer.
func (h *MockHost) Connect(ctx context.Context, pi peer.AddrInfo) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return errors.New("host closed")
	}

	// Create mock connection
	conn := &MockConn{
		localPeer:  h.id,
		remotePeer: pi.ID,
		streams:    make([]*MockStream, 0),
	}
	h.connections[pi.ID] = conn
	return nil
}

// SetStreamHandler sets the handler for a protocol.
func (h *MockHost) SetStreamHandler(pid protocol.ID, handler network.StreamHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[pid] = handler
}

// SetStreamHandlerMatch sets a handler with a match function.
func (h *MockHost) SetStreamHandlerMatch(pid protocol.ID, match func(protocol.ID) bool, handler network.StreamHandler) {
	h.SetStreamHandler(pid, handler)
}

// RemoveStreamHandler removes the handler for a protocol.
func (h *MockHost) RemoveStreamHandler(pid protocol.ID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.handlers, pid)
}

// NewStream creates a new stream to a peer.
func (h *MockHost) NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return nil, errors.New("host closed")
	}

	conn, exists := h.connections[p]
	if !exists {
		return nil, errors.New("not connected to peer")
	}

	if len(pids) == 0 {
		return nil, errors.New("no protocols specified")
	}

	stream := &MockStream{
		readBuf:    bytes.NewBuffer(nil),
		writeBuf:   bytes.NewBuffer(nil),
		protocolID: pids[0],
		conn:       conn,
	}
	conn.streams = append(conn.streams, stream)

	return stream, nil
}

// Close closes the host.
func (h *MockHost) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	return nil
}

// ConnManager returns nil (not implemented for mock).
func (h *MockHost) ConnManager() connmgr.ConnManager {
	return nil
}

// EventBus returns nil (not implemented for mock).
func (h *MockHost) EventBus() event.Bus {
	return nil
}

// GetHandler returns the handler for a protocol.
func (h *MockHost) GetHandler(pid protocol.ID) (network.StreamHandler, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	handler, ok := h.handlers[pid]
	return handler, ok
}

// GetConnections returns all connections.
func (h *MockHost) GetConnections() map[peer.ID]*MockConn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make(map[peer.ID]*MockConn, len(h.connections))
	for k, v := range h.connections {
		result[k] = v
	}
	return result
}

// MockStream implements network.Stream for testing.
type MockStream struct {
	mu         sync.Mutex
	readBuf    *bytes.Buffer
	writeBuf   *bytes.Buffer
	protocolID protocol.ID
	conn       *MockConn
	closed     bool
	deadline   time.Time
}

// Read reads from the stream.
func (s *MockStream) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, io.EOF
	}
	return s.readBuf.Read(p)
}

// Write writes to the stream.
func (s *MockStream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, errors.New("stream closed")
	}
	return s.writeBuf.Write(p)
}

// Close closes the stream.
func (s *MockStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// CloseWrite closes the write side of the stream.
func (s *MockStream) CloseWrite() error {
	return nil
}

// CloseRead closes the read side of the stream.
func (s *MockStream) CloseRead() error {
	return nil
}

// Reset resets the stream.
func (s *MockStream) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.readBuf.Reset()
	s.writeBuf.Reset()
	return nil
}

// SetDeadline sets the deadline for the stream.
func (s *MockStream) SetDeadline(t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deadline = t
	return nil
}

// SetReadDeadline sets the read deadline.
func (s *MockStream) SetReadDeadline(t time.Time) error {
	return s.SetDeadline(t)
}

// SetWriteDeadline sets the write deadline.
func (s *MockStream) SetWriteDeadline(t time.Time) error {
	return s.SetDeadline(t)
}

// Protocol returns the stream's protocol ID.
func (s *MockStream) Protocol() protocol.ID {
	return s.protocolID
}

// SetProtocol sets the protocol ID.
func (s *MockStream) SetProtocol(pid protocol.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.protocolID = pid
	return nil
}

// Stat returns stream statistics.
func (s *MockStream) Stat() network.Stats {
	return network.Stats{}
}

// Conn returns the connection the stream belongs to.
func (s *MockStream) Conn() network.Conn {
	return s.conn
}

// ID returns the stream ID.
func (s *MockStream) ID() string {
	return "mock-stream"
}

// Scope returns the resource scope.
func (s *MockStream) Scope() network.StreamScope {
	return nil
}

// WriteData writes data to the stream's buffer (for testing).
func (s *MockStream) WriteData(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readBuf.Write(data)
}

// ReadWrittenData reads data that was written to the stream (for testing).
func (s *MockStream) ReadWrittenData() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeBuf.Bytes()
}

// IsClosed returns whether the stream is closed.
func (s *MockStream) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// MockConn implements network.Conn for testing.
type MockConn struct {
	mu         sync.Mutex
	localPeer  peer.ID
	remotePeer peer.ID
	streams    []*MockStream
	closed     bool
	localAddr  multiaddr.Multiaddr
	remoteAddr multiaddr.Multiaddr
}

// Close closes the connection.
func (c *MockConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for _, s := range c.streams {
		_ = s.Close()
	}
	return nil
}

// LocalPeer returns the local peer ID.
func (c *MockConn) LocalPeer() peer.ID {
	return c.localPeer
}

// RemotePeer returns the remote peer ID.
func (c *MockConn) RemotePeer() peer.ID {
	return c.remotePeer
}

// LocalMultiaddr returns the local multiaddr.
func (c *MockConn) LocalMultiaddr() multiaddr.Multiaddr {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.localAddr
}

// RemoteMultiaddr returns the remote multiaddr.
func (c *MockConn) RemoteMultiaddr() multiaddr.Multiaddr {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.remoteAddr
}

// RemotePublicKey returns nil (not implemented for mock).
func (c *MockConn) RemotePublicKey() ic.PubKey {
	return nil
}

// NewStream creates a new stream on the connection.
func (c *MockConn) NewStream(ctx context.Context) (network.Stream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("connection closed")
	}
	stream := &MockStream{
		readBuf:  bytes.NewBuffer(nil),
		writeBuf: bytes.NewBuffer(nil),
		conn:     c,
	}
	c.streams = append(c.streams, stream)
	return stream, nil
}

// GetStreams returns all streams on the connection.
func (c *MockConn) GetStreams() []network.Stream {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]network.Stream, len(c.streams))
	for i, s := range c.streams {
		result[i] = s
	}
	return result
}

// Stat returns connection statistics.
func (c *MockConn) Stat() network.ConnStats {
	return network.ConnStats{}
}

// Scope returns the resource scope.
func (c *MockConn) Scope() network.ConnScope {
	return nil
}

// ID returns the connection ID.
func (c *MockConn) ID() string {
	return "mock-conn"
}

// IsClosed returns whether the connection is closed.
func (c *MockConn) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// ConnState returns the connection state.
func (c *MockConn) ConnState() network.ConnectionState {
	return network.ConnectionState{}
}

// MockNetworkAdapter implements network.Network for MockHost.
type MockNetworkAdapter struct {
	host *MockHost
}

// Connectedness returns the connectedness to a peer.
func (n *MockNetworkAdapter) Connectedness(p peer.ID) network.Connectedness {
	if n.host == nil {
		return network.NotConnected
	}
	n.host.mu.RLock()
	defer n.host.mu.RUnlock()
	if _, ok := n.host.connections[p]; ok {
		return network.Connected
	}
	return network.NotConnected
}

// Peers returns connected peers (stub implementation).
func (n *MockNetworkAdapter) Peers() []peer.ID {
	if n.host == nil {
		return nil
	}
	n.host.mu.RLock()
	defer n.host.mu.RUnlock()
	peers := make([]peer.ID, 0, len(n.host.connections))
	for p := range n.host.connections {
		peers = append(peers, p)
	}
	return peers
}

// Conns returns connections (stub implementation).
func (n *MockNetworkAdapter) Conns() []network.Conn {
	if n.host == nil {
		return nil
	}
	n.host.mu.RLock()
	defer n.host.mu.RUnlock()
	conns := make([]network.Conn, 0, len(n.host.connections))
	for _, c := range n.host.connections {
		conns = append(conns, c)
	}
	return conns
}

// ConnsToPeer returns connections to a peer.
func (n *MockNetworkAdapter) ConnsToPeer(p peer.ID) []network.Conn {
	if n.host == nil {
		return nil
	}
	n.host.mu.RLock()
	defer n.host.mu.RUnlock()
	if conn, ok := n.host.connections[p]; ok {
		return []network.Conn{conn}
	}
	return nil
}

// Notify registers a notifiee (stub implementation).
func (n *MockNetworkAdapter) Notify(notifiee network.Notifiee) {}

// StopNotify stops notifying a notifiee (stub implementation).
func (n *MockNetworkAdapter) StopNotify(notifiee network.Notifiee) {}

// Close closes the network (stub implementation).
func (n *MockNetworkAdapter) Close() error { return nil }

// SetStreamHandler is a stub.
func (n *MockNetworkAdapter) SetStreamHandler(network.StreamHandler) {}

// NewStream creates a new stream.
func (n *MockNetworkAdapter) NewStream(ctx context.Context, p peer.ID) (network.Stream, error) {
	if n.host == nil {
		return nil, errors.New("no host")
	}
	return n.host.NewStream(ctx, p, "")
}

// Listen is a stub.
func (n *MockNetworkAdapter) Listen(...multiaddr.Multiaddr) error { return nil }

// ListenAddresses returns listen addresses (stub).
func (n *MockNetworkAdapter) ListenAddresses() []multiaddr.Multiaddr { return nil }

// InterfaceListenAddresses returns interface listen addresses (stub).
func (n *MockNetworkAdapter) InterfaceListenAddresses() ([]multiaddr.Multiaddr, error) {
	return nil, nil
}

// Process is required by the interface.
func (n *MockNetworkAdapter) Process() interface{} { return nil }

// Peerstore returns the peerstore (stub).
func (n *MockNetworkAdapter) Peerstore() peerstore.Peerstore { return nil }

// LocalPeer returns the local peer ID.
func (n *MockNetworkAdapter) LocalPeer() peer.ID {
	if n.host == nil {
		return ""
	}
	return n.host.ID()
}

// DialPeer dials a peer.
func (n *MockNetworkAdapter) DialPeer(ctx context.Context, p peer.ID) (network.Conn, error) {
	if n.host == nil {
		return nil, errors.New("no host")
	}
	n.host.mu.Lock()
	defer n.host.mu.Unlock()
	conn := &MockConn{
		localPeer:  n.host.id,
		remotePeer: p,
		streams:    make([]*MockStream, 0),
	}
	n.host.connections[p] = conn
	return conn, nil
}

// ClosePeer closes the connection to a peer.
func (n *MockNetworkAdapter) ClosePeer(p peer.ID) error {
	if n.host == nil {
		return nil
	}
	n.host.mu.Lock()
	defer n.host.mu.Unlock()
	if conn, ok := n.host.connections[p]; ok {
		_ = conn.Close()
		delete(n.host.connections, p)
	}
	return nil
}

// ResourceManager returns nil (not implemented for mock).
func (n *MockNetworkAdapter) ResourceManager() network.ResourceManager { return nil }
