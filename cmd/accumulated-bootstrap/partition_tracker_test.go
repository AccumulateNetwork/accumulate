// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	"github.com/cometbft/cometbft/crypto/tmhash"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTrackerPeer returns a libp2p peer ID and the CometBFT node ID
// expected for its key.
func newTrackerPeer(t *testing.T) (peer.ID, string) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	lpPub, err := crypto.UnmarshalEd25519PublicKey(pub)
	require.NoError(t, err)
	id, err := peer.IDFromPublicKey(lpPub)
	require.NoError(t, err)
	return id, hex.EncodeToString(tmhash.SumTruncated(pub))
}

// newBareTracker builds a tracker with no libp2p host. GetConsensusPeers
// and GetPeersByPartition only read the in-memory maps, so a host is not
// needed for these tests.
func newBareTracker() *PartitionTracker {
	return &PartitionTracker{
		peers:       make(map[peer.ID]*PeerPartitionInfo),
		byPartition: make(map[string]map[peer.ID]struct{}),
	}
}

func (pt *PartitionTracker) putPeer(id peer.ID, partition string, addrs ...string) {
	mas := make([]multiaddr.Multiaddr, 0, len(addrs))
	for _, a := range addrs {
		ma, err := multiaddr.NewMultiaddr(a)
		if err != nil {
			panic(err)
		}
		mas = append(mas, ma)
	}
	pt.peers[id] = &PeerPartitionInfo{
		PeerID:    id,
		Partition: partition,
		Addresses: mas,
		LastSeen:  time.Now(),
	}
	if pt.byPartition[partition] == nil {
		pt.byPartition[partition] = make(map[peer.ID]struct{})
	}
	pt.byPartition[partition][id] = struct{}{}
}

func TestGetConsensusPeers(t *testing.T) {
	pt := newBareTracker()

	id, wantID := newTrackerPeer(t)
	pt.putPeer(id, PartitionDN, "/ip4/203.0.113.7/tcp/16593")

	peers := pt.GetConsensusPeers(PartitionDN)
	require.Len(t, peers, 1)
	assert.Equal(t, wantID, string(peers[0].ID))
	assert.Equal(t, "203.0.113.7", peers[0].Host)
	assert.Equal(t, 16591, peers[0].Port)
	assert.Equal(t, wantID+"@203.0.113.7:16591", peers[0].DialString())
}

func TestGetConsensusPeers_SkipsLoopback(t *testing.T) {
	pt := newBareTracker()

	// A peer reachable only on loopback yields nothing dialable.
	loop, _ := newTrackerPeer(t)
	pt.putPeer(loop, PartitionDN, "/ip4/127.0.0.1/tcp/16593")

	// A peer with both a loopback and a routable address yields the
	// routable one.
	dual, wantID := newTrackerPeer(t)
	pt.putPeer(dual, PartitionDN, "/ip4/127.0.0.1/tcp/16593", "/ip4/198.51.100.4/tcp/16593")

	peers := pt.GetConsensusPeers(PartitionDN)
	require.Len(t, peers, 1)
	assert.Equal(t, wantID+"@198.51.100.4:16591", peers[0].DialString())
}

func TestGetConsensusPeers_UnknownPartition(t *testing.T) {
	pt := newBareTracker()
	assert.Empty(t, pt.GetConsensusPeers("nope"))
}
