// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package peerregistry

import (
	"testing"

	tmp2p "github.com/cometbft/cometbft/p2p"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/consensuspeer"
)

func TestEmbeddedGetServicePeers(t *testing.T) {
	pt := newBareTracker()
	pubID, pubNode := newTrackerPeer(t)
	privID, privNode := newTrackerPeer(t)
	pt.recordAdvertised(pubID, pubNode, "bvn1", "1.2.3.4", 26656)
	pt.consensusByPartition["bvn1"][privID] = consensuspeer.Peer{
		ID: tmp2p.ID(privNode), Host: "5.6.7.8", Port: 26656, Private: true,
	}
	pt.privatePeers[privID] = true
	pt.peers[pubID] = &PeerPartitionInfo{PeerID: pubID,
		Addresses: []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/1.2.3.4/tcp/16658")}}
	pt.peers[privID] = &PeerPartitionInfo{PeerID: privID,
		Addresses: []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/5.6.7.8/tcp/16658")}}

	peers := (&Embedded{tracker: pt}).GetServicePeers("bvn1")
	require.Len(t, peers, 1, "the private peer must be excluded from service peers")
	require.Equal(t, pubID, peers[0].ID)
	require.NotEmpty(t, peers[0].Addrs)
}
