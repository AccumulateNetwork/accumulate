// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package consensuspeer converts a peer's libp2p address into the
// CometBFT peer string (`NodeID@host:port`) used by consensus P2P.
//
// The conversion is purely derivational: a node's CometBFT node key is
// the same key as its libp2p host key (see convertNodeKey in
// cmd/accumulated/run/consensus.go), so the CometBFT NodeID is
// tmhash.SumTruncated(libp2p public key). Nothing about the consensus
// identity has to be asserted by the peer — it follows deterministically
// from the libp2p identity the discovery layer already observes.
//
// This is what lets the bootstrap server broker consensus reachability
// (#4043): for any libp2p peer it tracks, it can derive the peer's
// CometBFT `NodeID@host:port` and feed it into a node's
// Switch().DialPeersAsync, without a separate consensus-identity
// advertisement or trust authority. A forged address still cannot
// impersonate: CometBFT's secret-connection handshake rejects a remote
// whose key does not hash to the advertised NodeID.
package consensuspeer

import (
	"encoding/hex"
	"net"
	"strconv"

	"github.com/cometbft/cometbft/crypto/tmhash"
	tmp2p "github.com/cometbft/cometbft/p2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multihash"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// ProtocolID is the libp2p protocol a node serves to advertise its
// CometBFT consensus endpoints, one per partition it runs. The bootstrap
// server opens this stream on each peer it tracks and reads the
// Advertisement (#4043).
//
// Advertisement carries only reachability (partition → host:port). The
// CometBFT NodeID is NOT advertised — it is derived from the peer's
// libp2p key (NodeIDFromPeerID), which the libp2p stream already
// authenticates. This keeps identity un-forgeable while letting a
// multi-partition (dual) node disambiguate which port serves which
// partition — something pure address derivation cannot do.
const ProtocolID protocol.ID = "/acc/consensus-peer/1.0.0"

// Advertisement is the JSON a node serves over ProtocolID.
type Advertisement struct {
	Peers []Advertised `json:"peers"`
}

// Advertised is one partition's CometBFT P2P endpoint.
type Advertised struct {
	Partition string `json:"partition"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
}

// NodeIDFromPeerID derives the CometBFT node ID from a libp2p peer ID by
// extracting the embedded public key and hashing it the way CometBFT
// does. It errors if the peer ID does not embed an extractable key
// (e.g. a hashed, non-identity multihash for a non-ed25519 key).
func NodeIDFromPeerID(id peer.ID) (tmp2p.ID, error) {
	pub, err := id.ExtractPublicKey()
	if err != nil {
		return "", errors.BadRequest.WithFormat("extract public key from peer ID: %w", err)
	}
	raw, err := pub.Raw()
	if err != nil {
		return "", errors.BadRequest.WithFormat("unwrap public key: %w", err)
	}
	return tmp2p.ID(hex.EncodeToString(tmhash.SumTruncated(raw))), nil
}

// Libp2pToCometPortOffset is the difference between a node's libp2p P2P
// port and its CometBFT P2P port in the standard deployment layout:
// libp2p listens at the partition base +2 (e.g. 16593, 16693) and
// CometBFT at +0 (e.g. 16591, 16691). Adding this offset to a libp2p
// port yields the CometBFT port.
const Libp2pToCometPortOffset = -2

// Peer is a resolved CometBFT consensus peer: its node ID plus the
// host:port to dial.
type Peer struct {
	ID   tmp2p.ID
	Host string
	Port int
}

// DialString returns the `NodeID@host:port` form CometBFT expects in
// persistent_peers and Switch().DialPeersAsync.
func (p Peer) DialString() string {
	return tmp2p.IDAddressString(p.ID, net.JoinHostPort(p.Host, strconv.Itoa(p.Port)))
}

// FromLibp2pMultiaddr derives the CometBFT peer for a libp2p multiaddr
// that carries a /p2p/<id> component plus a host and a port. The node ID
// is computed from the embedded libp2p public key; the dial port is the
// multiaddr port shifted by Libp2pToCometPortOffset.
//
// It returns an error if the multiaddr is missing the peer ID, host, or
// port, or if the resulting CometBFT port is out of range.
func FromLibp2pMultiaddr(addr multiaddr.Multiaddr) (Peer, error) {
	var pub *multihash.DecodedMultihash
	var host, port string
	var err error
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_P2P:
			pub, err = multihash.Decode(c.RawValue())
		case multiaddr.P_IP4,
			multiaddr.P_IP6,
			multiaddr.P_DNS,
			multiaddr.P_DNS4,
			multiaddr.P_DNS6:
			host = c.Value()
		case multiaddr.P_TCP,
			multiaddr.P_UDP:
			port = c.Value()
		}
		if err != nil {
			return false
		}
		return pub == nil || host == "" || port == ""
	})
	if err != nil {
		return Peer{}, err
	}
	if pub == nil {
		return Peer{}, errors.BadRequest.With("missing peer ID")
	}
	if host == "" {
		return Peer{}, errors.BadRequest.With("missing host")
	}
	if port == "" {
		return Peer{}, errors.BadRequest.With("missing port")
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return Peer{}, errors.BadRequest.WithFormat("invalid port %q: %w", port, err)
	}
	cmtPort := portNum + Libp2pToCometPortOffset
	if cmtPort < 1 || cmtPort > 65535 {
		return Peer{}, errors.BadRequest.WithFormat("adjusted port %d out of valid range", cmtPort)
	}

	id, err := nodeIDFromPubKeyHash(pub)
	if err != nil {
		return Peer{}, err
	}
	return Peer{ID: id, Host: host, Port: cmtPort}, nil
}

// DialStringFromLibp2pMultiaddr is a convenience wrapper that returns the
// `NodeID@host:port` string directly.
func DialStringFromLibp2pMultiaddr(addr multiaddr.Multiaddr) (string, error) {
	p, err := FromLibp2pMultiaddr(addr)
	if err != nil {
		return "", err
	}
	return p.DialString(), nil
}

// nodeIDFromPubKeyHash computes the CometBFT node ID from the libp2p
// public key encoded in a /p2p/ multihash. CometBFT IDs are the
// truncated SHA-256 of the raw public key bytes.
func nodeIDFromPubKeyHash(pub *multihash.DecodedMultihash) (tmp2p.ID, error) {
	var hash []byte
	switch pub.Code {
	case multihash.IDENTITY:
		p, err := crypto.UnmarshalPublicKey(pub.Digest)
		if err != nil {
			return "", errors.BadRequest.WithFormat("decode public key: %w", err)
		}
		b, err := p.Raw()
		if err != nil {
			return "", errors.BadRequest.WithFormat("unwrap public key: %w", err)
		}
		hash = tmhash.SumTruncated(b)
	case multihash.SHA2_256:
		hash = pub.Digest[:tmhash.TruncatedSize]
	default:
		return "", errors.BadRequest.WithFormat("unsupported multihash type %v", pub.Name)
	}
	return tmp2p.ID(hex.EncodeToString(hash)), nil
}
