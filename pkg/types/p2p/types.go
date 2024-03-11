// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"encoding/json"
	"io"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// Multiaddr is a wrapper for the canonical multiaddr type.
type Multiaddr = multiaddr.Multiaddr

// CopyMultiaddr copies a multiaddr.
func CopyMultiaddr(v Multiaddr) Multiaddr {
	return v // No need to copy (immutable)
}

// EqualMultiaddr checks if two multiaddrs are equal.
func EqualMultiaddr(a, b Multiaddr) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(b)
}

// UnmarshalMultiaddr reads a multiaddr from the given bytes.
func UnmarshalMultiaddr(b []byte) (Multiaddr, error) {
	return multiaddr.NewMultiaddrBytes(b)
}

// UnmarshalMultiaddrFrom reads a multiaddr from the given reader.
func UnmarshalMultiaddrFrom(r io.Reader) (Multiaddr, error) {
	b, err := io.ReadAll(io.LimitReader(r, 1024))
	if err != nil {
		return nil, err
	}
	return multiaddr.NewMultiaddrBytes(b)
}

// UnmarshalMultiaddrJSON reads a multiaddr as a string from the given JSON
// bytes.
func UnmarshalMultiaddrJSON(b []byte) (Multiaddr, error) {
	var v string
	err := json.Unmarshal(b, &v)
	if err != nil {
		return nil, err
	}
	return multiaddr.NewMultiaddr(v)
}

type PeerID = peer.ID

func CopyPeerID(v PeerID) PeerID {
	return v // No need to copy (immutable)
}

func EqualPeerID(a, b PeerID) bool {
	return a == b
}

func UnmarshalPeerID(b []byte) (PeerID, error) {
	var v peer.ID
	return v, v.UnmarshalBinary(b)
}

func UnmarshalPeerIDFrom(r io.Reader) (PeerID, error) {
	var v peer.ID
	b, err := io.ReadAll(io.LimitReader(r, 1024))
	if err != nil {
		return "", err
	}
	return v, v.UnmarshalBinary(b)
}

func UnmarshalPeerIDJSON(b []byte) (PeerID, error) {
	var v peer.ID
	return v, v.UnmarshalJSON(b)
}
