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

func TestGetPeersByPartition_SynthesizesAddressFromConsensus(t *testing.T) {
	// A peer advertised a consensus endpoint but the registry has no libp2p
	// connection to it (no pt.peers entry, nil host). The directory must still
	// hand out a dialable libp2p address derived from the advertisement, or the
	// delivery backstop gets an addressless peer and every dial fails (#4047).
	pt := newBareTracker()
	id, node := newTrackerPeer(t)
	pt.recordAdvertised(id, node, "bvn2", "10.0.0.5", 26656)

	peers := pt.GetPeersByPartition("bvn2")
	require.Len(t, peers, 1)
	require.Equal(t, id, peers[0].PeerID)
	require.Len(t, peers[0].Addresses, 1, "address synthesized from the consensus advertisement")
	require.Equal(t, "/ip4/10.0.0.5/tcp/26658", peers[0].Addresses[0].String(),
		"CometBFT P2P port 26656 maps to libp2p 26658")
}

func TestLibp2pAddrFromConsensus(t *testing.T) {
	got := func(host string, port int) string {
		a := libp2pAddrFromConsensus(consensuspeer.Peer{Host: host, Port: port})
		if a == nil {
			return ""
		}
		return a.String()
	}
	require.Equal(t, "/ip4/172.20.0.14/tcp/26758", got("172.20.0.14", 26756))
	require.Equal(t, "/dns4/node.example.com/tcp/16595", got("node.example.com", 16593))
	require.Equal(t, "", got("", 26656), "no host -> nil")
	require.Equal(t, "", got("1.2.3.4", 0), "no port -> nil")
}

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
