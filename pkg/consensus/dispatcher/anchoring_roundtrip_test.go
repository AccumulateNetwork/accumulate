// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dispatcher

import (
	"context"
	"fmt"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	bhost "github.com/libp2p/go-libp2p/p2p/host/blank"
	swarmt "github.com/libp2p/go-libp2p/p2p/net/swarm/testing"
	"github.com/stretchr/testify/require"
)

// anchorNode is one AnchorHandler on its own libp2p host.
type anchorNode struct {
	ps *pubsub.PubSub
	ah *AnchorHandler
}

// newAnchorNetwork builds one AnchorHandler per entry, each on its own host,
// all hosts connected. Entries may repeat a partition — that is the real
// topology: every node of a partition joins the same anchor topic.
func newAnchorNetwork(t *testing.T, ctx context.Context, partitions ...string) []*anchorNode {
	t.Helper()

	var nodes []*anchorNode
	for _, part := range partitions {
		h := bhost.NewBlankHost(swarmt.GenSwarm(t))
		t.Cleanup(func() { _ = h.Close() })

		ps, err := pubsub.NewGossipSub(ctx, h)
		require.NoError(t, err)

		ah, err := NewAnchorHandler(h, ps, part)
		require.NoError(t, err)
		nodes = append(nodes, &anchorNode{ps: ps, ah: ah})
	}

	for i, a := range nodes {
		ha := a.ah.host
		for _, b := range nodes[i+1:] {
			hb := b.ah.host
			ha.Peerstore().AddAddrs(hb.ID(), hb.Addrs(), time.Hour)
			require.NoError(t, ha.Connect(ctx, hb.Peerstore().PeerInfo(hb.ID())))
		}
	}

	for _, n := range nodes {
		require.NoError(t, n.ah.Start(ctx))
		t.Cleanup(func() { _ = n.ah.Close() })
	}
	return nodes
}

// TestAnchorHandler_RoundTripAndTopicIsolation: an anchor broadcast by one
// node of a partition reaches the other nodes of THAT partition with every
// field intact, the acknowledgment flows back, the pending-anchor lifecycle
// tracks it — and a handler on a different partition's topic hears nothing.
func TestAnchorHandler_RoundTripAndTopicIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	nodes := newAnchorNetwork(t, ctx, "directory", "directory", "bvn1")
	sender, receiver, outsider := nodes[0], nodes[1], nodes[2]

	blockHash := [32]byte{1, 2, 3}
	stateRoot := [32]byte{4, 5, 6}
	ts := time.Unix(0, 1_700_000_000_123_456_789)

	// Broadcast until the mesh delivers (anchors are re-broadcastable by
	// design — the pending map keys by block index, so a repeat overwrites
	// itself).
	var got *Anchor
	var sent uint64
	require.Eventually(t, func() bool {
		sent++
		require.NoError(t, sender.ah.BroadcastAnchor(ctx, &Anchor{
			SourcePartition: "directory",
			BlockIndex:      42,
			BlockHash:       blockHash,
			StateRoot:       stateRoot,
			Timestamp:       ts,
		}))
		select {
		case got = <-receiver.ah.SubscribeAnchors():
			return true
		case <-time.After(250 * time.Millisecond):
			return false
		}
	}, 30*time.Second, 10*time.Millisecond, "the anchor never reached its partition peers")

	// Every field survives the wire, including nanosecond timestamps.
	require.Equal(t, "directory", got.SourcePartition)
	require.Equal(t, uint64(42), got.BlockIndex)
	require.Equal(t, blockHash, got.BlockHash)
	require.Equal(t, stateRoot, got.StateRoot)
	require.True(t, ts.Equal(got.Timestamp), "timestamp must survive at nanosecond precision")

	// The sender tracks its pending anchor until cleared.
	pa, ok := sender.ah.GetPendingAnchor(42)
	require.True(t, ok)
	require.Equal(t, blockHash, pa.BlockHash)

	// The ack flows back on the same topic.
	var ack *AnchorAck
	require.Eventually(t, func() bool {
		require.NoError(t, receiver.ah.BroadcastAck(ctx, &AnchorAck{
			AnchorPartition: "directory",
			AckPartition:    "bvn2",
			BlockIndex:      42,
			BlockHash:       blockHash,
		}))
		select {
		case ack = <-sender.ah.SubscribeAcks():
			return true
		case <-time.After(250 * time.Millisecond):
			return false
		}
	}, 30*time.Second, 10*time.Millisecond, "the ack never reached the anchor's sender")
	require.Equal(t, "directory", ack.AnchorPartition)
	require.Equal(t, "bvn2", ack.AckPartition)
	require.Equal(t, uint64(42), ack.BlockIndex)
	require.Equal(t, blockHash, ack.BlockHash)

	// Pending lifecycle: record the ack, clear, gone.
	sender.ah.RecordAck(42, ack.AckPartition)
	sender.ah.ClearPendingAnchor(42)
	_, ok = sender.ah.GetPendingAnchor(42)
	require.False(t, ok, "a cleared anchor must not remain pending")

	// Topic isolation: the bvn1 handler shared the whole network and heard
	// none of it — anchors and acks stay on their partition's topic.
	select {
	case a := <-outsider.ah.SubscribeAnchors():
		t.Fatalf("anchor for partition %q crossed onto %q's topic", a.SourcePartition, "bvn1")
	case k := <-outsider.ah.SubscribeAcks():
		t.Fatalf("ack for partition %q crossed onto %q's topic", k.AnchorPartition, "bvn1")
	case <-time.After(500 * time.Millisecond):
	}
}

// TestAnchorHandler_MalformedPayloadsDoNotKill: garbage on the anchor topic —
// empty frames, unknown message types, truncated anchors and acks, length
// prefixes that overrun the payload — must be dropped without killing
// handleMessages; a legitimate anchor still arrives afterward.
func TestAnchorHandler_MalformedPayloadsDoNotKill(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	nodes := newAnchorNetwork(t, ctx, "directory", "directory")
	sender, receiver := nodes[0], nodes[1]

	// A raw publisher on the same topic, from a third host.
	rawHost := bhost.NewBlankHost(swarmt.GenSwarm(t))
	t.Cleanup(func() { _ = rawHost.Close() })
	rawPS, err := pubsub.NewGossipSub(ctx, rawHost)
	require.NoError(t, err)
	for _, n := range nodes {
		h := n.ah.host
		rawHost.Peerstore().AddAddrs(h.ID(), h.Addrs(), time.Hour)
		require.NoError(t, rawHost.Connect(ctx, rawHost.Peerstore().PeerInfo(h.ID())))
	}
	topic, err := rawPS.Join(fmt.Sprintf(TopicAnchor, "directory"))
	require.NoError(t, err)

	garbage := [][]byte{
		{},                // empty frame
		{0x7F},            // unknown message type
		{0x01},            // anchor with no payload
		{0x01, 0xFF, 'x'}, // anchor whose partition length overruns
		{0x02},            // ack with no payload
		{0x02, 0x05, 'a'}, // ack truncated inside the first partition
		{0x01, 0x00},      // anchor truncated after an empty partition
	}

	var got *Anchor
	var sent uint64
	require.Eventually(t, func() bool {
		for _, g := range garbage {
			require.NoError(t, topic.Publish(ctx, g))
		}
		sent++
		require.NoError(t, sender.ah.BroadcastAnchor(ctx, &Anchor{
			SourcePartition: "directory",
			BlockIndex:      sent,
			BlockHash:       [32]byte{9},
			Timestamp:       time.Unix(0, 1),
		}))
		select {
		case got = <-receiver.ah.SubscribeAnchors():
			return true
		case <-time.After(250 * time.Millisecond):
			return false
		}
	}, 30*time.Second, 10*time.Millisecond, "handleMessages did not survive malformed traffic")
	require.Equal(t, "directory", got.SourcePartition)
}
