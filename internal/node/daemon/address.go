// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"strconv"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/p2p"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Advertize returns an address builder for the node's advertized address.
func (n *NodeInit) Advertize() AddressBuilder {
	return AddressBuilder{baseAddr: n.AdvertizeAddress, node: n}
}

// Listen returns an address builder for the node's listen address.
func (n *NodeInit) Listen() AddressBuilder {
	addr := n.ListenAddress
	if addr == "" {
		addr = n.AdvertizeAddress
	}
	return AddressBuilder{baseAddr: addr, node: n}
}

// Peer returns an address builder for the node's peer address.
func (n *NodeInit) Peer() AddressBuilder {
	addr := n.PeerAddress
	if addr == "" {
		addr = n.AdvertizeAddress
	}
	return AddressBuilder{baseAddr: addr, node: n}
}

// Peers returns an address slice builder for every node in the BVN other than
// the given one.
func (b *BvnInit) Peers(node *NodeInit) AddressSliceBuilder {
	var peers AddressSliceBuilder
	for _, n := range b.Nodes {
		if n != node {
			peers = append(peers, n.Peer())
		}
	}
	return peers
}

// Peers returns an address slice builder for every node in the network other
// than the given one.
func (n *NetworkInit) Peers(node *NodeInit) AddressSliceBuilder {
	var peers AddressSliceBuilder
	for _, b := range n.Bvns {
		peers = append(peers, b.Peers(node)...)
	}
	return peers
}

// AddressBuilder builds an address.
type AddressBuilder struct {
	node      *NodeInit
	baseAddr  string
	scheme    string
	partition protocol.PartitionType
	service   config.PortOffset
	withKey   bool
}

// Scheme sets the address scheme.
func (b AddressBuilder) Scheme(scheme string) AddressBuilder {
	b.scheme = scheme
	return b
}

// PartitionType sets the partition type.
func (b AddressBuilder) PartitionType(typ protocol.PartitionType) AddressBuilder {
	b.partition = typ
	return b
}

// Directory sets the partition type to [protocol.PartitionTypeDirectory].
func (b AddressBuilder) Directory() AddressBuilder {
	b.partition = protocol.PartitionTypeDirectory
	return b
}

// BlockValidator sets the partition type to
// [protocol.PartitionTypeBlockValidator].
func (b AddressBuilder) BlockValidator() AddressBuilder {
	b.partition = protocol.PartitionTypeBlockValidator
	return b
}

// BlockSummary sets the partition type to
// [protocol.PartitionTypeBlockSummary].
func (b AddressBuilder) BlockSummary() AddressBuilder {
	b.partition = protocol.PartitionTypeBlockSummary
	return b
}

// TendermintP2P sets the service to [config.PortOffsetTendermintP2P].
func (b AddressBuilder) TendermintP2P() AddressBuilder {
	b.service = config.PortOffsetTendermintP2P
	return b
}

// TendermintRPC sets the service to [config.PortOffsetTendermintRpc].
func (b AddressBuilder) TendermintRPC() AddressBuilder {
	b.service = config.PortOffsetTendermintRpc
	return b
}

// AccumulateP2P sets the service to [config.PortOffsetAccumulateP2P].
func (b AddressBuilder) AccumulateP2P() AddressBuilder {
	b.service = config.PortOffsetAccumulateP2P
	return b
}

// AccumulateAPI sets the service to [config.PortOffsetAccumulateApi].
func (b AddressBuilder) AccumulateAPI() AddressBuilder {
	b.service = config.PortOffsetAccumulateApi
	return b
}

// Prometheus sets the service to [config.PortOffsetPrometheus].
func (b AddressBuilder) Prometheus() AddressBuilder {
	b.service = config.PortOffsetPrometheus
	return b
}

// WithKey includes the node key in the address.
func (b AddressBuilder) WithKey() AddressBuilder {
	b.withKey = true
	return b
}

// String builds the address as a traditional URL address string.
func (b AddressBuilder) String() string {
	addr := b.baseAddr

	// Add the port
	if b.partition != 0 {
		port := b.node.BasePort + uint64(b.service)
		switch b.partition {
		case protocol.PartitionTypeDirectory:
			port += config.PortOffsetDirectory
		case protocol.PartitionTypeBlockValidator:
			port += config.PortOffsetBlockValidator
		case protocol.PartitionTypeBlockSummary:
			port += config.PortOffsetBlockSummary
		default:
			panic("invalid partition type")
		}
		addr += ":" + strconv.FormatUint(port, 10)
	}

	// Add the key
	if b.withKey {
		var sk []byte
		switch b.partition {
		case protocol.PartitionTypeDirectory:
			sk = b.node.DnNodeKey
		case protocol.PartitionTypeBlockValidator:
			sk = b.node.BvnNodeKey
		case protocol.PartitionTypeBlockSummary:
			sk = b.node.BsnNodeKey
		default:
			panic("invalid partition type")
		}
		nodeId := p2p.PubKeyToID(ed25519.PubKey(sk[32:]))
		addr = p2p.IDAddressString(nodeId, addr)
	}

	// Add the scheme
	if b.scheme != "" {
		addr = b.scheme + "://" + addr
	}

	return addr
}

// Multiaddr builds the address as a multiaddr.
func (b AddressBuilder) Multiaddr() multiaddr.Multiaddr {
	addr := "/ip4/" + b.baseAddr

	// Add the port
	if b.scheme != "" {
		port := b.node.BasePort + uint64(b.service)
		switch b.partition {
		case protocol.PartitionTypeDirectory:
			port += config.PortOffsetDirectory
		case protocol.PartitionTypeBlockValidator:
			port += config.PortOffsetBlockValidator
		case protocol.PartitionTypeBlockSummary:
			port += config.PortOffsetBlockSummary
		default:
			panic("invalid partition type")
		}
		addr += "/" + b.scheme + "/" + strconv.FormatUint(port, 10)
		if b.scheme == "udp" {
			addr += "/quic"
		}
	}

	// Add the key
	if b.withKey {
		var sk []byte
		switch b.partition {
		case protocol.PartitionTypeDirectory:
			sk = b.node.DnNodeKey
		case protocol.PartitionTypeBlockValidator:
			sk = b.node.BvnNodeKey
		case protocol.PartitionTypeBlockSummary:
			sk = b.node.BsnNodeKey
		default:
			panic("invalid partition type")
		}
		key, err := crypto.UnmarshalEd25519PublicKey(sk[32:])
		if err != nil {
			panic(err)
		}
		id, err := peer.IDFromPublicKey(key)
		if err != nil {
			panic(err)
		}
		addr += "/p2p/" + id.String()
	}

	// Add the scheme
	ma, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		panic(err)
	}
	return ma
}

// AddressSliceBuilder builds a slice of addresses.
type AddressSliceBuilder []AddressBuilder

// Do returns the Cartesian product of the address list and the given
// operations. Given N addresses and M operations, Do returns N⨯M addresses.
func (b AddressSliceBuilder) Do(fn ...func(AddressBuilder) AddressBuilder) AddressSliceBuilder {
	c := make(AddressSliceBuilder, 0, len(b)*len(fn))
	for _, b := range b {
		for _, fn := range fn {
			c = append(c, fn(b))
		}
	}
	return c
}

// Scheme sets the address scheme.
func (b AddressSliceBuilder) Scheme(scheme string) AddressSliceBuilder {
	return b.Do(func(b AddressBuilder) AddressBuilder { return b.Scheme(scheme) })
}

// PartitionType sets the partition type.
func (b AddressSliceBuilder) PartitionType(typ protocol.PartitionType) AddressSliceBuilder {
	return b.Do(func(b AddressBuilder) AddressBuilder { return b.PartitionType(typ) })
}

// Directory sets the partition type to [protocol.PartitionTypeDirectory].
func (b AddressSliceBuilder) Directory() AddressSliceBuilder {
	return b.Do(AddressBuilder.Directory)
}

// [protocol.PartitionTypeBlockValidator].
func (b AddressSliceBuilder) BlockValidator() AddressSliceBuilder {
	return b.Do(AddressBuilder.BlockValidator)
}

// [protocol.PartitionTypeBlockSummary].
func (b AddressSliceBuilder) BlockSummary() AddressSliceBuilder {
	return b.Do(AddressBuilder.BlockSummary)
}

// TendermintP2P sets the service to [config.PortOffsetTendermintP2P].
func (b AddressSliceBuilder) TendermintP2P() AddressSliceBuilder {
	return b.Do(AddressBuilder.TendermintP2P)
}

// TendermintRPC sets the service to [config.PortOffsetTendermintRpc].
func (b AddressSliceBuilder) TendermintRPC() AddressSliceBuilder {
	return b.Do(AddressBuilder.TendermintRPC)
}

// AccumulateP2P sets the service to [config.PortOffsetAccumulateP2P].
func (b AddressSliceBuilder) AccumulateP2P() AddressSliceBuilder {
	return b.Do(AddressBuilder.AccumulateP2P)
}

// AccumulateAPI sets the service to [config.PortOffsetAccumulateApi].
func (b AddressSliceBuilder) AccumulateAPI() AddressSliceBuilder {
	return b.Do(AddressBuilder.AccumulateAPI)
}

// Prometheus sets the service to [config.PortOffsetPrometheus].
func (b AddressSliceBuilder) Prometheus() AddressSliceBuilder {
	return b.Do(AddressBuilder.Prometheus)
}

// WithKey includes the node key in the address.
func (b AddressSliceBuilder) WithKey() AddressSliceBuilder {
	return b.Do(AddressBuilder.WithKey)
}

// String builds the address as a traditional URL address string.
func (b AddressSliceBuilder) String() []string {
	c := make([]string, len(b))
	for i, b := range b {
		c[i] = b.String()
	}
	return c
}

// Multiaddr builds the address as a multiaddr.
func (b AddressSliceBuilder) Multiaddr() []multiaddr.Multiaddr {
	c := make([]multiaddr.Multiaddr, len(b))
	for i, b := range b {
		c[i] = b.Multiaddr()
	}
	return c
}
