// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/testutil"
)

func TestHostConfig_ApplyDefaults(t *testing.T) {
	config := HostConfig{}
	config.ApplyDefaults()

	assert.Equal(t, DefaultLowWatermark, config.LowWatermark)
	assert.Equal(t, DefaultHighWatermark, config.HighWatermark)
	assert.Equal(t, DefaultGracePeriod, config.GracePeriod)
}

func TestHostConfig_ApplyDefaults_CustomValues(t *testing.T) {
	config := HostConfig{
		LowWatermark:  50,
		HighWatermark: 200,
		GracePeriod:   2 * time.Minute,
	}
	config.ApplyDefaults()

	// Custom values should not be overwritten
	assert.Equal(t, 50, config.LowWatermark)
	assert.Equal(t, 200, config.HighWatermark)
	assert.Equal(t, 2*time.Minute, config.GracePeriod)
}

func TestDefaultValues(t *testing.T) {
	// Verify default values are reasonable
	assert.Greater(t, DefaultLowWatermark, 0)
	assert.Greater(t, DefaultHighWatermark, 0)
	assert.Greater(t, DefaultHighWatermark, DefaultLowWatermark)
	assert.Greater(t, DefaultGracePeriod, time.Duration(0))
}

func TestParseMultiaddr(t *testing.T) {
	// Valid multiaddr
	addr, err := ParseMultiaddr("/ip4/127.0.0.1/tcp/9000")
	require.NoError(t, err)
	assert.NotNil(t, addr)

	// Invalid multiaddr
	_, err = ParseMultiaddr("invalid")
	assert.Error(t, err)
}

func TestParseMultiaddrs(t *testing.T) {
	// Valid multiaddrs
	addrs, err := ParseMultiaddrs([]string{
		"/ip4/127.0.0.1/tcp/9000",
		"/ip4/127.0.0.1/tcp/9001",
	})
	require.NoError(t, err)
	assert.Len(t, addrs, 2)

	// Invalid in list
	_, err = ParseMultiaddrs([]string{
		"/ip4/127.0.0.1/tcp/9000",
		"invalid",
	})
	assert.Error(t, err)

	// Empty list
	addrs, err = ParseMultiaddrs([]string{})
	require.NoError(t, err)
	assert.Len(t, addrs, 0)
}

func TestAddrInfoFromString(t *testing.T) {
	// Valid multiaddr with p2p component
	validAddr := "/ip4/127.0.0.1/tcp/9000/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN"
	info, err := AddrInfoFromString(validAddr)
	require.NoError(t, err)
	assert.NotEmpty(t, info.ID)
	assert.Len(t, info.Addrs, 1)

	// Invalid multiaddr
	_, err = AddrInfoFromString("invalid")
	assert.Error(t, err)

	// Valid multiaddr without p2p component
	_, err = AddrInfoFromString("/ip4/127.0.0.1/tcp/9000")
	assert.Error(t, err)
}

func TestAddrInfosFromStrings(t *testing.T) {
	// Valid multiaddrs with p2p components
	validAddrs := []string{
		"/ip4/127.0.0.1/tcp/9000/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN",
	}
	infos, err := AddrInfosFromStrings(validAddrs)
	require.NoError(t, err)
	assert.Len(t, infos, 1)

	// Invalid in list
	invalidAddrs := []string{
		"/ip4/127.0.0.1/tcp/9000/p2p/12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN",
		"invalid",
	}
	_, err = AddrInfosFromStrings(invalidAddrs)
	assert.Error(t, err)

	// Empty list
	infos, err = AddrInfosFromStrings([]string{})
	require.NoError(t, err)
	assert.Len(t, infos, 0)
}

func TestMockHost_BasicOperations(t *testing.T) {
	mockHost := testutil.NewMockHost(peer.ID("test-host-1"))

	// ID
	assert.Equal(t, peer.ID("test-host-1"), mockHost.ID())

	// Initially no addresses
	addrs := mockHost.Addrs()
	assert.Nil(t, addrs)

	// Initially no connections
	conns := mockHost.GetConnections()
	assert.Len(t, conns, 0)
}

func TestMockHost_Connect(t *testing.T) {
	host1 := testutil.NewMockHost(peer.ID("host-1"))
	host2 := testutil.NewMockHost(peer.ID("host-2"))

	// Connect host1 to host2
	err := host1.Connect(nil, peer.AddrInfo{ID: host2.ID()})
	require.NoError(t, err)

	// Should have connection
	conns := host1.GetConnections()
	assert.Len(t, conns, 1)
	assert.Contains(t, conns, host2.ID())
}

func TestMockHost_SetStreamHandler(t *testing.T) {
	host := testutil.NewMockHost(peer.ID("test-host"))

	// Set a stream handler using the network.StreamHandler type
	host.SetStreamHandler("test-protocol", func(s network.Stream) {
		// Handler body
	})

	// Handler should be retrievable
	_, ok := host.GetHandler("test-protocol")
	assert.True(t, ok)

	// Non-existent handler
	_, ok = host.GetHandler("other-protocol")
	assert.False(t, ok)

	// Remove handler
	host.RemoveStreamHandler("test-protocol")
	_, ok = host.GetHandler("test-protocol")
	assert.False(t, ok)
}

func TestMockHost_Close(t *testing.T) {
	host := testutil.NewMockHost(peer.ID("test-host"))

	// Connect to another peer first
	err := host.Connect(nil, peer.AddrInfo{ID: peer.ID("other")})
	require.NoError(t, err)

	// Close the host
	err = host.Close()
	require.NoError(t, err)

	// Should not be able to connect after close
	err = host.Connect(nil, peer.AddrInfo{ID: peer.ID("another")})
	assert.Error(t, err)
}

func TestMockHost_NewStream(t *testing.T) {
	host := testutil.NewMockHost(peer.ID("test-host"))

	// Connect to a peer
	peer2 := peer.ID("peer-2")
	err := host.Connect(nil, peer.AddrInfo{ID: peer2})
	require.NoError(t, err)

	// Create a stream
	stream, err := host.NewStream(nil, peer2, "test-protocol")
	require.NoError(t, err)
	assert.NotNil(t, stream)

	// Try to create stream to unconnected peer
	_, err = host.NewStream(nil, peer.ID("unconnected"), "test-protocol")
	assert.Error(t, err)

	// No protocol specified
	_, err = host.NewStream(nil, peer2)
	assert.Error(t, err)
}

func TestMockStream_ReadWrite(t *testing.T) {
	host := testutil.NewMockHost(peer.ID("test-host"))
	peer2 := peer.ID("peer-2")

	// Connect
	err := host.Connect(nil, peer.AddrInfo{ID: peer2})
	require.NoError(t, err)

	// Create stream
	stream, err := host.NewStream(nil, peer2, "test-protocol")
	require.NoError(t, err)

	mockStream := stream.(*testutil.MockStream)

	// Write data
	n, err := mockStream.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	// Read written data using test helper
	written := mockStream.ReadWrittenData()
	assert.Equal(t, []byte("hello"), written)

	// Write data to read buffer using test helper
	mockStream.WriteData([]byte("world"))

	// Read it back
	buf := make([]byte, 10)
	n, err = mockStream.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, []byte("world"), buf[:n])
}

func TestMockStream_Close(t *testing.T) {
	host := testutil.NewMockHost(peer.ID("test-host"))
	peer2 := peer.ID("peer-2")

	err := host.Connect(nil, peer.AddrInfo{ID: peer2})
	require.NoError(t, err)

	stream, err := host.NewStream(nil, peer2, "test-protocol")
	require.NoError(t, err)

	mockStream := stream.(*testutil.MockStream)

	// Close the stream
	err = mockStream.Close()
	require.NoError(t, err)

	// Check closed state
	assert.True(t, mockStream.IsClosed())

	// Writing to closed stream should fail
	_, err = mockStream.Write([]byte("test"))
	assert.Error(t, err)
}

func TestMockStream_Reset(t *testing.T) {
	host := testutil.NewMockHost(peer.ID("test-host"))
	peer2 := peer.ID("peer-2")

	err := host.Connect(nil, peer.AddrInfo{ID: peer2})
	require.NoError(t, err)

	stream, err := host.NewStream(nil, peer2, "test-protocol")
	require.NoError(t, err)

	mockStream := stream.(*testutil.MockStream)

	// Write some data
	mockStream.Write([]byte("test"))

	// Reset the stream
	err = mockStream.Reset()
	require.NoError(t, err)

	// Should be closed
	assert.True(t, mockStream.IsClosed())
}

func TestMockStream_Protocol(t *testing.T) {
	host := testutil.NewMockHost(peer.ID("test-host"))
	peer2 := peer.ID("peer-2")

	err := host.Connect(nil, peer.AddrInfo{ID: peer2})
	require.NoError(t, err)

	stream, err := host.NewStream(nil, peer2, "test-protocol")
	require.NoError(t, err)

	mockStream := stream.(*testutil.MockStream)

	// Check protocol
	assert.Equal(t, "test-protocol", string(mockStream.Protocol()))

	// Set different protocol
	err = mockStream.SetProtocol("other-protocol")
	require.NoError(t, err)
	assert.Equal(t, "other-protocol", string(mockStream.Protocol()))
}

func TestMockStream_Deadlines(t *testing.T) {
	host := testutil.NewMockHost(peer.ID("test-host"))
	peer2 := peer.ID("peer-2")

	err := host.Connect(nil, peer.AddrInfo{ID: peer2})
	require.NoError(t, err)

	stream, err := host.NewStream(nil, peer2, "test-protocol")
	require.NoError(t, err)

	mockStream := stream.(*testutil.MockStream)

	// Set deadlines (should not error)
	deadline := time.Now().Add(time.Minute)
	err = mockStream.SetDeadline(deadline)
	require.NoError(t, err)

	err = mockStream.SetReadDeadline(deadline)
	require.NoError(t, err)

	err = mockStream.SetWriteDeadline(deadline)
	require.NoError(t, err)
}

func TestMockConn_BasicOperations(t *testing.T) {
	host := testutil.NewMockHost(peer.ID("test-host"))
	peer2 := peer.ID("peer-2")

	err := host.Connect(nil, peer.AddrInfo{ID: peer2})
	require.NoError(t, err)

	conns := host.GetConnections()
	conn := conns[peer2]

	// LocalPeer and RemotePeer
	assert.Equal(t, peer.ID("test-host"), conn.LocalPeer())
	assert.Equal(t, peer2, conn.RemotePeer())

	// Not closed initially
	assert.False(t, conn.IsClosed())

	// Close
	err = conn.Close()
	require.NoError(t, err)
	assert.True(t, conn.IsClosed())
}

func TestMockConn_NewStream(t *testing.T) {
	host := testutil.NewMockHost(peer.ID("test-host"))
	peer2 := peer.ID("peer-2")

	err := host.Connect(nil, peer.AddrInfo{ID: peer2})
	require.NoError(t, err)

	conns := host.GetConnections()
	conn := conns[peer2]

	// Create stream on connection
	stream, err := conn.NewStream(nil)
	require.NoError(t, err)
	assert.NotNil(t, stream)

	// Get streams
	streams := conn.GetStreams()
	assert.Len(t, streams, 1)
}

func TestMockConn_NewStream_Closed(t *testing.T) {
	host := testutil.NewMockHost(peer.ID("test-host"))
	peer2 := peer.ID("peer-2")

	err := host.Connect(nil, peer.AddrInfo{ID: peer2})
	require.NoError(t, err)

	conns := host.GetConnections()
	conn := conns[peer2]

	// Close connection
	err = conn.Close()
	require.NoError(t, err)

	// Creating stream on closed connection should fail
	_, err = conn.NewStream(nil)
	assert.Error(t, err)
}
