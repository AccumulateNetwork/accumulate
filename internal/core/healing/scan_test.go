// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package healing

import (
	"encoding/json"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// This package had no tests at all — anchors, scan, sequenced and synthetic,
// 1300 lines of the subsystem responsible for recovering messages the network
// lost, entirely unexercised. These cover the parts that can be tested without
// a network: peer bookkeeping and the JSON round-trip that persists a scan.

func mustPeer(t *testing.T, s string) peer.ID {
	t.Helper()
	id, err := peer.Decode(s)
	require.NoError(t, err)
	return id
}

// Three real peer IDs, so the tests exercise actual multihash encoding rather
// than strings that happen to look like IDs.
const (
	peerA = "12D3KooWGZbLR4CFXo6cKcVCQwGvJDsGvBLpMKvFhFbUFTGuY3Ub"
	peerB = "12D3KooWRR9EnBnGgWMkGDrCJTdWHmGVGqSjPKEEXNRXJqZKPAxY"
	peerC = "12D3KooWBhKkTMgTKVGvYNKRWEBRRLbhJZQxdY5F3AHrZ1eNfHwU"
)

func TestPeerByID_FindsAcrossPartitions(t *testing.T) {
	a, b := mustPeer(t, peerA), mustPeer(t, peerB)
	ni := &NetworkInfo{Peers: map[string]PeerList{
		"Directory": {a: &PeerInfo{ID: a}},
		"BVN1":      {b: &PeerInfo{ID: b}},
	}}

	require.NotNil(t, ni.PeerByID(a), "a Directory peer must be found")
	assert.Equal(t, a, ni.PeerByID(a).ID)
	require.NotNil(t, ni.PeerByID(b), "a BVN peer must be found")
	assert.Equal(t, b, ni.PeerByID(b).ID)
}

func TestPeerByID_UnknownPeerReturnsNil(t *testing.T) {
	a := mustPeer(t, peerA)
	ni := &NetworkInfo{Peers: map[string]PeerList{"BVN1": {a: &PeerInfo{ID: a}}}}
	assert.Nil(t, ni.PeerByID(mustPeer(t, peerC)))
}

// A nil NetworkInfo must not panic. Healing runs against a scan that may have
// failed, and a nil dereference in the recovery path takes the node down at
// precisely the moment it is trying to recover.
func TestPeerByID_NilNetworkInfoIsSafe(t *testing.T) {
	var ni *NetworkInfo
	assert.NotPanics(t, func() { assert.Nil(t, ni.PeerByID(mustPeer(t, peerA))) })

	empty := &NetworkInfo{}
	assert.Nil(t, empty.PeerByID(mustPeer(t, peerA)), "no peers means no match")
}

// PeerList is keyed by peer.ID, which is not a string, so it needs custom JSON
// on both sides. A scan is persisted and reloaded; if the round trip drops or
// mangles peers, healing silently loses the peers it would have asked.
func TestPeerList_JSONRoundTrip(t *testing.T) {
	a, b := mustPeer(t, peerA), mustPeer(t, peerB)
	op := url.MustParse("acc://operator.acme")

	orig := PeerList{
		a: &PeerInfo{ID: a, Operator: op},
		b: &PeerInfo{ID: b},
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	var back PeerList
	require.NoError(t, json.Unmarshal(data, &back))

	require.Len(t, back, 2, "both peers must survive the round trip")
	require.Contains(t, back, a)
	require.Contains(t, back, b)
	assert.Equal(t, a, back[a].ID, "the ID must be restored, not just the map key")
	require.NotNil(t, back[a].Operator)
	assert.Equal(t, op.String(), back[a].Operator.String())
	assert.Nil(t, back[b].Operator, "a peer with no operator stays that way")
}

func TestPeerList_EmptyRoundTrip(t *testing.T) {
	data, err := json.Marshal(PeerList{})
	require.NoError(t, err)

	var back PeerList
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Empty(t, back)
}

// Malformed input must be an error, not a panic or a silently empty list: a
// corrupt cache file should be reported, not treated as "no peers", which
// would disable healing without saying so.
func TestPeerList_MalformedJSONIsAnError(t *testing.T) {
	var back PeerList
	assert.Error(t, json.Unmarshal([]byte(`{"not-a-peer-id":{}}`), &back),
		"an unparseable peer ID must be reported")

	var back2 PeerList
	assert.Error(t, json.Unmarshal([]byte(`["not","an","object"]`), &back2))
}

// String() is what appears in every healing log line. An operator-less peer
// must still render usefully rather than as an empty string.
func TestPeerInfo_String(t *testing.T) {
	a := mustPeer(t, peerA)

	bare := (&PeerInfo{ID: a}).String()
	assert.Contains(t, bare, a.String(), "the ID must always appear")

	named := (&PeerInfo{ID: a, Operator: url.MustParse("acc://operator.acme")}).String()
	assert.Contains(t, named, "operator.acme", "the operator should be named when known")
	assert.Contains(t, named, a.String(), "and the ID kept, so it can still be dialled")
}
