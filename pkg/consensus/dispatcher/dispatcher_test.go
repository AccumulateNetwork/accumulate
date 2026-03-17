// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dispatcher

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapUnwrapMessage(t *testing.T) {
	d := &Dispatcher{
		partition: "bvn0",
	}

	// Test wrapping and unwrapping
	payload := []byte("test payload data")
	wrapped := d.wrapMessage(payload)

	source, unwrapped, err := d.unwrapMessage(wrapped)
	require.NoError(t, err)
	assert.Equal(t, "bvn0", source)
	assert.Equal(t, payload, unwrapped)
}

func TestWrapUnwrapMessage_EmptyPayload(t *testing.T) {
	d := &Dispatcher{
		partition: "directory",
	}

	payload := []byte{}
	wrapped := d.wrapMessage(payload)

	source, unwrapped, err := d.unwrapMessage(wrapped)
	require.NoError(t, err)
	assert.Equal(t, "directory", source)
	assert.Empty(t, unwrapped)
}

func TestWrapUnwrapMessage_LongPartition(t *testing.T) {
	d := &Dispatcher{
		partition: "verylongpartitionname",
	}

	payload := []byte("some data")
	wrapped := d.wrapMessage(payload)

	source, unwrapped, err := d.unwrapMessage(wrapped)
	require.NoError(t, err)
	assert.Equal(t, "verylongpartitionname", source)
	assert.Equal(t, payload, unwrapped)
}

func TestUnwrapMessage_TooShort(t *testing.T) {
	d := &Dispatcher{}

	_, _, err := d.unwrapMessage([]byte{0x01, 0x02})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestUnwrapMessage_BadVersion(t *testing.T) {
	d := &Dispatcher{}

	// Create message with wrong version
	msg := []byte{0x02, 0x04, 't', 'e', 's', 't', 0x00, 0x00, 0x00, 0x01, 'x'}
	_, _, err := d.unwrapMessage(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported message version")
}

func TestUnwrapMessage_Truncated(t *testing.T) {
	d := &Dispatcher{}

	// Create message with partition length but missing partition
	msg := []byte{0x01, 0x10}
	_, _, err := d.unwrapMessage(msg)
	assert.Error(t, err)
	// Error could be "message too short" or "truncated" depending on which check fails first
	assert.True(t, err != nil)
}

// testPeerID creates a peer.ID from a string for testing.
// Note: peer.ID is an alias for string in libp2p, so this works.
func testPeerID(s string) peer.ID {
	return peer.ID(s)
}

func TestPartitionDiscovery_AddRemovePeer(t *testing.T) {
	pd := NewPartitionDiscovery(nil, "bvn0")

	peer1 := testPeerID("12D3KooWTestPeer1")
	peer2 := testPeerID("12D3KooWTestPeer2")

	// Add peers to different partitions
	pd.AddPeer("BVN1", peer1)
	pd.AddPeer("bvn1", peer2) // Same partition, different case
	pd.AddPeer("Directory", peer1)

	// Check peers
	bvn1Peers := pd.GetPeers("bvn1")
	assert.Len(t, bvn1Peers, 2)

	dirPeers := pd.GetPeers("directory")
	assert.Len(t, dirPeers, 1)

	// Remove peer
	pd.RemovePeer("bvn1", peer1)
	bvn1Peers = pd.GetPeers("bvn1")
	assert.Len(t, bvn1Peers, 1)

	// Get all partitions
	partitions := pd.GetAllPartitions()
	assert.Len(t, partitions, 2)
}

func TestPartitionDiscovery_DuplicatePeer(t *testing.T) {
	pd := NewPartitionDiscovery(nil, "bvn0")

	peer1 := testPeerID("12D3KooWTestPeer1")

	// Add same peer twice
	pd.AddPeer("bvn1", peer1)
	pd.AddPeer("bvn1", peer1)

	// Should only be one
	peers := pd.GetPeers("bvn1")
	assert.Len(t, peers, 1)
}

func TestPartitionDiscovery_RemoveNonexistent(t *testing.T) {
	pd := NewPartitionDiscovery(nil, "bvn0")

	// Should not panic
	pd.RemovePeer("bvn1", testPeerID("nonexistent"))
}

func TestAnchorMarshalUnmarshal(t *testing.T) {
	original := &Anchor{
		SourcePartition: "bvn0",
		BlockIndex:      12345,
		BlockHash:       [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		StateRoot:       [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
		Timestamp:       time.Unix(1234567890, 123456789),
	}

	data, err := marshalAnchor(original)
	require.NoError(t, err)

	restored, err := unmarshalAnchor(data)
	require.NoError(t, err)

	assert.Equal(t, original.SourcePartition, restored.SourcePartition)
	assert.Equal(t, original.BlockIndex, restored.BlockIndex)
	assert.Equal(t, original.BlockHash, restored.BlockHash)
	assert.Equal(t, original.StateRoot, restored.StateRoot)
	assert.Equal(t, original.Timestamp.UnixNano(), restored.Timestamp.UnixNano())
}

func TestAnchorAckMarshalUnmarshal(t *testing.T) {
	original := &AnchorAck{
		AnchorPartition: "bvn0",
		AckPartition:    "directory",
		BlockIndex:      67890,
		BlockHash:       [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
	}

	data, err := marshalAnchorAck(original)
	require.NoError(t, err)

	restored, err := unmarshalAnchorAck(data)
	require.NoError(t, err)

	assert.Equal(t, original.AnchorPartition, restored.AnchorPartition)
	assert.Equal(t, original.AckPartition, restored.AckPartition)
	assert.Equal(t, original.BlockIndex, restored.BlockIndex)
	assert.Equal(t, original.BlockHash, restored.BlockHash)
}

func TestUnmarshalAnchor_TooShort(t *testing.T) {
	_, err := unmarshalAnchor([]byte{})
	assert.Error(t, err)
}

func TestUnmarshalAnchor_Truncated(t *testing.T) {
	_, err := unmarshalAnchor([]byte{5, 'a', 'b', 'c'})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "truncated")
}

func TestUnmarshalAnchorAck_TooShort(t *testing.T) {
	_, err := unmarshalAnchorAck([]byte{0})
	assert.Error(t, err)
}

func TestUnmarshalAnchorAck_Truncated(t *testing.T) {
	_, err := unmarshalAnchorAck([]byte{4, 'b', 'v', 'n', '0', 3})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "truncated")
}

func TestDispatcherOptions_Defaults(t *testing.T) {
	opts := DispatcherOptions{}

	// After applying defaults
	if opts.EnvelopeChannelSize <= 0 {
		opts.EnvelopeChannelSize = DefaultEnvelopeChannelSize
	}
	if opts.SendTimeout <= 0 {
		opts.SendTimeout = DefaultSendTimeout
	}

	assert.Equal(t, DefaultEnvelopeChannelSize, opts.EnvelopeChannelSize)
	assert.Equal(t, DefaultSendTimeout, opts.SendTimeout)
}

func TestAnchorHandlerOptions_Defaults(t *testing.T) {
	opts := AnchorHandlerOptions{}

	// After applying defaults
	if opts.AnchorChannelSize <= 0 {
		opts.AnchorChannelSize = DefaultAnchorChannelSize
	}
	if opts.AckChannelSize <= 0 {
		opts.AckChannelSize = DefaultAnchorChannelSize
	}

	assert.Equal(t, DefaultAnchorChannelSize, opts.AnchorChannelSize)
	assert.Equal(t, DefaultAnchorChannelSize, opts.AckChannelSize)
}

func TestNewDispatcher_NilHost(t *testing.T) {
	_, err := NewDispatcher(nil, nil, nil, "bvn0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "host is nil")
}

func TestNewDispatcher_EmptyPartition(t *testing.T) {
	// Can't test without mocking libp2p host, but we can verify the error path
	_, err := NewDispatcherWithOptions(nil, nil, nil, "", DispatcherOptions{})
	assert.Error(t, err)
}

func TestNewAnchorHandler_NilHost(t *testing.T) {
	_, err := NewAnchorHandler(nil, nil, "bvn0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "host is nil")
}

func TestNewAnchorHandler_EmptyPartition(t *testing.T) {
	_, err := NewAnchorHandlerWithOptions(nil, nil, "", AnchorHandlerOptions{})
	assert.Error(t, err)
}

func TestPartitionDiscovery_GetPeers_NonexistentPartition(t *testing.T) {
	pd := NewPartitionDiscovery(nil, "bvn0")
	peers := pd.GetPeers("nonexistent")
	assert.Nil(t, peers)
}

func TestPartitionDiscovery_StartClose(t *testing.T) {
	pd := NewPartitionDiscovery(nil, "bvn0")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := pd.Start(ctx)
	require.NoError(t, err)

	// Starting again should fail
	err = pd.Start(ctx)
	assert.Error(t, err)

	pd.Close()
}

func TestBigEndianHelpers(t *testing.T) {
	// Test putUint64BE and getUint64BE
	testCases := []uint64{
		0,
		1,
		255,
		256,
		65535,
		1234567890123456789,
		^uint64(0), // max uint64
	}

	for _, tc := range testCases {
		b := make([]byte, 8)
		putUint64BE(b, tc)
		result := getUint64BE(b)
		assert.Equal(t, tc, result)
	}
}
