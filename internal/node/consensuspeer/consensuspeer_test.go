// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensuspeer

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/cometbft/cometbft/crypto/tmhash"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestPeer generates an ed25519 libp2p identity and returns its
// libp2p peer.ID plus the CometBFT node ID expected for that key
// (truncated SHA-256 of the raw public key).
func newTestPeer(t *testing.T) (peer.ID, string) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// Wrap the ed25519 public key as a libp2p key and derive its peer ID.
	lpPub, err := crypto.UnmarshalEd25519PublicKey(pub)
	require.NoError(t, err)
	id, err := peer.IDFromPublicKey(lpPub)
	require.NoError(t, err)

	wantID := hex.EncodeToString(tmhash.SumTruncated(pub))
	return id, wantID
}

func TestFromLibp2pMultiaddr(t *testing.T) {
	id, wantID := newTestPeer(t)

	addr, err := multiaddr.NewMultiaddr("/ip4/203.0.113.7/tcp/16593/p2p/" + id.String())
	require.NoError(t, err)

	p, err := FromLibp2pMultiaddr(addr)
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.7", p.Host)
	assert.Equal(t, 16591, p.Port, "libp2p port should shift by the comet offset")
	assert.Equal(t, wantID, string(p.ID))
	assert.Equal(t, wantID+"@203.0.113.7:16591", p.DialString())
}

func TestFromLibp2pMultiaddr_DNSAndUDP(t *testing.T) {
	id, wantID := newTestPeer(t)

	// DNS host + UDP/QUIC port should resolve the same way.
	addr, err := multiaddr.NewMultiaddr("/dns4/node.example.com/udp/16693/quic-v1/p2p/" + id.String())
	require.NoError(t, err)

	p, err := FromLibp2pMultiaddr(addr)
	require.NoError(t, err)
	assert.Equal(t, "node.example.com", p.Host)
	assert.Equal(t, 16691, p.Port)
	assert.Equal(t, wantID+"@node.example.com:16691", p.DialString())
}

func TestFromLibp2pMultiaddr_Errors(t *testing.T) {
	id, _ := newTestPeer(t)

	cases := map[string]string{
		"missing peer ID": "/ip4/203.0.113.7/tcp/16593",
		"missing host":    "/p2p/" + id.String(), // no host/port
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			addr, err := multiaddr.NewMultiaddr(s)
			require.NoError(t, err)
			_, err = FromLibp2pMultiaddr(addr)
			require.Error(t, err)
		})
	}
}

func TestFromLibp2pMultiaddr_PortOutOfRange(t *testing.T) {
	id, _ := newTestPeer(t)

	// Port 1 shifts to -1, which is out of range.
	addr, err := multiaddr.NewMultiaddr("/ip4/203.0.113.7/tcp/1/p2p/" + id.String())
	require.NoError(t, err)
	_, err = FromLibp2pMultiaddr(addr)
	require.Error(t, err)
}
