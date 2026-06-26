// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package peerregistry

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/cometbft/cometbft/crypto/tmhash"
	tmp2p "github.com/cometbft/cometbft/p2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/consensuspeer"
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
// only reads consensusByPartition, so a host is not needed here.
func newBareTracker() *PartitionTracker {
	return &PartitionTracker{
		peers:                make(map[peer.ID]*PeerPartitionInfo),
		consensusByPartition: make(map[string]map[peer.ID]consensuspeer.Peer),
	}
}

func TestGetConsensusPeers(t *testing.T) {
	pt := newBareTracker()

	id, nodeID := newTrackerPeer(t)
	pt.recordAdvertised(id, nodeID, "directory", "198.51.100.4", 26656)

	peers := pt.GetConsensusPeers("Directory") // query is case-insensitive
	require.Len(t, peers, 1)
	assert.Equal(t, nodeID, string(peers[0].ID))
	assert.Equal(t, "198.51.100.4", peers[0].Host)
	assert.Equal(t, 26656, peers[0].Port)
	assert.Equal(t, nodeID+"@198.51.100.4:26656", peers[0].DialString())
}

func TestGetConsensusPeers_DualNodeDistinguishesPartitions(t *testing.T) {
	pt := newBareTracker()

	// One dual node advertises distinct DN and BVN endpoints on the same
	// node ID — the exact case derivation could not disambiguate.
	id, nodeID := newTrackerPeer(t)
	pt.recordAdvertised(id, nodeID, "directory", "198.51.100.4", 26656)
	pt.recordAdvertised(id, nodeID, "bvn1", "198.51.100.4", 26756)

	dn := pt.GetConsensusPeers("directory")
	require.Len(t, dn, 1)
	assert.Equal(t, 26656, dn[0].Port)

	bvn := pt.GetConsensusPeers("bvn1")
	require.Len(t, bvn, 1)
	assert.Equal(t, 26756, bvn[0].Port)
	assert.Equal(t, dn[0].ID, bvn[0].ID, "same node ID, different port per partition")
}

func TestGetConsensusPeers_UnknownPartition(t *testing.T) {
	pt := newBareTracker()
	assert.Empty(t, pt.GetConsensusPeers("nope"))
}

func TestGetConsensusPeers_ExcludesPrivate(t *testing.T) {
	pt := newBareTracker()

	pubID, pubNode := newTrackerPeer(t)
	privID, privNode := newTrackerPeer(t)
	pt.recordAdvertised(pubID, pubNode, "bvn1", "198.51.100.4", 26656)
	// A guarded validator advertised itself private to its guard: the registry
	// knows it but must never serve it (#4047, privacy-by-absence).
	pt.consensusByPartition["bvn1"][privID] = consensuspeer.Peer{
		ID: tmp2p.ID(privNode), Host: "203.0.113.9", Port: 26656, Private: true,
	}

	peers := pt.GetConsensusPeers("bvn1")
	require.Len(t, peers, 1, "the private peer must not be served")
	assert.Equal(t, pubNode, string(peers[0].ID))
}

func TestForgetConsensusPeer(t *testing.T) {
	pt := newBareTracker()
	id, nodeID := newTrackerPeer(t)
	pt.recordAdvertised(id, nodeID, "directory", "198.51.100.4", 26656)
	pt.recordAdvertised(id, nodeID, "bvn1", "198.51.100.4", 26756)

	pt.forgetConsensusPeer(id)
	assert.Empty(t, pt.GetConsensusPeers("directory"))
	assert.Empty(t, pt.GetConsensusPeers("bvn1"))
}

// recordAdvertised stores a peer's advertised endpoint for a partition,
// keyed exactly as probeConsensusAdvertise does (lowercased partition).
func (pt *PartitionTracker) recordAdvertised(id peer.ID, nodeID, partition, host string, port int) {
	if pt.consensusByPartition[partition] == nil {
		pt.consensusByPartition[partition] = make(map[peer.ID]consensuspeer.Peer)
	}
	pt.consensusByPartition[partition][id] = consensuspeer.Peer{
		ID:   tmp2p.ID(nodeID),
		Host: host,
		Port: port,
	}
}
