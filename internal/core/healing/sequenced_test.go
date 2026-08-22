// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package healing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Anchors and synthetics live in different accounts. Querying the wrong one
// finds nothing, and healing then reports the message as unresolvable when it
// is simply being looked for in the wrong place.
func TestSequencedAccount_AnchorsAndSyntheticsDiffer(t *testing.T) {
	assert.Equal(t, protocol.AnchorPool, sequencedAccount(true))
	assert.Equal(t, protocol.Synthetic, sequencedAccount(false))
	assert.NotEqual(t, sequencedAccount(true), sequencedAccount(false),
		"the two must not resolve to the same account")
}

// The peer map is keyed lower-case; callers pass partition IDs from a scan, a
// config, or an operator's command line, in whatever case those use.
//
// A mismatch returns no peers and healing does nothing — silently, because an
// empty peer list looks exactly like "asked everyone, nobody had it". This is
// the difference between healing that is broken and healing that is finished.
func TestPeersForSource_PartitionIDIsCaseInsensitive(t *testing.T) {
	a := mustPeer(t, peerA)
	net := &NetworkInfo{Peers: map[string]PeerList{
		"bvn1": {a: &PeerInfo{ID: a}},
	}}

	for _, id := range []string{"bvn1", "BVN1", "Bvn1", "bVn1"} {
		peers := peersForSource(net, id)
		require.Len(t, peers, 1, "source %q should resolve to the bvn1 peers", id)
		assert.Contains(t, peers, a)
	}
}

func TestPeersForSource_UnknownPartitionIsEmptyNotAnError(t *testing.T) {
	a := mustPeer(t, peerA)
	net := &NetworkInfo{Peers: map[string]PeerList{"bvn1": {a: &PeerInfo{ID: a}}}}
	assert.Empty(t, peersForSource(net, "bvn9"),
		"an unknown partition has no peers")
}

// Nil-safe: healing runs against a scan that may have failed, and a nil
// dereference in the recovery path takes the node down at precisely the moment
// it is trying to recover.
func TestPeersForSource_NilNetworkIsSafe(t *testing.T) {
	assert.NotPanics(t, func() {
		assert.Empty(t, peersForSource(nil, "bvn1"))
	})
	assert.NotPanics(t, func() {
		assert.Empty(t, peersForSource(&NetworkInfo{}, "bvn1"),
			"a scan with no peers must not panic")
	})
}

// The Directory is a partition like any other here and must resolve.
func TestPeersForSource_DirectoryResolves(t *testing.T) {
	a := mustPeer(t, peerA)
	net := &NetworkInfo{Peers: map[string]PeerList{
		"directory": {a: &PeerInfo{ID: a}},
	}}
	require.Len(t, peersForSource(net, protocol.Directory), 1,
		"protocol.Directory must find the directory peers")
	require.Len(t, peersForSource(net, "DIRECTORY"), 1)
}

// An empty source ID must not accidentally match a partition.
func TestPeersForSource_EmptyIDMatchesNothing(t *testing.T) {
	a := mustPeer(t, peerA)
	net := &NetworkInfo{Peers: map[string]PeerList{"bvn1": {a: &PeerInfo{ID: a}}}}
	assert.Empty(t, peersForSource(net, ""))
}

// Several partitions coexist without bleeding into each other: healing a BVN1
// message must not ask BVN2's peers.
func TestPeersForSource_PartitionsAreIsolated(t *testing.T) {
	a, b := mustPeer(t, peerA), mustPeer(t, peerB)
	net := &NetworkInfo{Peers: map[string]PeerList{
		"bvn1": {a: &PeerInfo{ID: a}},
		"bvn2": {b: &PeerInfo{ID: b}},
	}}

	p1 := peersForSource(net, "BVN1")
	require.Len(t, p1, 1)
	assert.Contains(t, p1, a)
	assert.NotContains(t, p1, b, "BVN1 must not be given BVN2's peers")

	p2 := peersForSource(net, "BVN2")
	require.Len(t, p2, 1)
	assert.Contains(t, p2, b)
}
