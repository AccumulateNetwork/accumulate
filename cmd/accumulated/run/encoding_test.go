// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"encoding/json"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

// TestPeerIDWidgetIsPeerIDsOwnEncoding pins the peer ID widget to peer.ID's
// own JSON (the base58 string), and to widget copies and comparisons that
// write through to the field, so a peer map entry reads back equal.
func TestPeerIDWidgetIsPeerIDsOwnEncoding(t *testing.T) {
	_, pub, err := crypto.GenerateEd25519Key(nil)
	require.NoError(t, err)
	id, err := peer.IDFromPublicKey(pub)
	require.NoError(t, err)

	want, err := json.Marshal(id)
	require.NoError(t, err)

	src := &HttpPeerMapEntry{ID: id}
	got, err := src.MarshalJSON()
	require.NoError(t, err)
	require.Contains(t, string(got), string(want))

	dst := new(HttpPeerMapEntry)
	require.NoError(t, dst.UnmarshalJSON(got))
	require.Equal(t, id, dst.ID)
	require.True(t, src.Equal(dst))

	cp := src.Copy()
	require.Equal(t, id, cp.ID)
	require.True(t, cp.Equal(src))

	require.False(t, new(HttpPeerMapEntry).Equal(src))
}

// TestMultiaddrWidgetRoundTrip pins the multiaddr widget's JSON to the
// multiaddr's own string form, and checks copy and equality semantics.
func TestMultiaddrWidgetRoundTrip(t *testing.T) {
	a := multiaddr.StringCast("/ip4/1.2.3.4/tcp/8080/http")
	src := &HttpListener{Listen: []multiaddr.Multiaddr{a}}

	b, err := src.MarshalJSON()
	require.NoError(t, err)
	require.Contains(t, string(b), `"/ip4/1.2.3.4/tcp/8080/http"`)

	dst := new(HttpListener)
	require.NoError(t, dst.UnmarshalJSON(b))
	require.Len(t, dst.Listen, 1)
	require.True(t, a.Equal(dst.Listen[0]))
	require.True(t, src.Equal(dst))

	cp := src.Copy()
	require.True(t, cp.Equal(src))

	other := &HttpListener{Listen: []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/1.2.3.4/tcp/9")}}
	require.False(t, other.Equal(src))
}
