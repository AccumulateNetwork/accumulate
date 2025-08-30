// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// GetMainnetPeers returns hardcoded mainnet peers with correct public addresses
// This is a workaround for mainnet nodes advertising private IPs
func GetMainnetPeers() []peer.AddrInfo {
	configs := []struct {
		id    string
		addrs []string
	}{
		{
			// Apollo node with correct public address
			id: "12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn",
			addrs: []string{
				"/ip4/23.22.212.106/tcp/16593",
				"/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593",
			},
		},
		// Add more nodes as needed
	}
	
	var peers []peer.AddrInfo
	for _, cfg := range configs {
		id, err := peer.Decode(cfg.id)
		if err != nil {
			continue
		}
		
		var addrs []multiaddr.Multiaddr
		for _, addr := range cfg.addrs {
			ma, err := multiaddr.NewMultiaddr(addr)
			if err != nil {
				continue
			}
			addrs = append(addrs, ma)
		}
		
		if len(addrs) > 0 {
			peers = append(peers, peer.AddrInfo{
				ID:    id,
				Addrs: addrs,
			})
		}
	}
	
	return peers
}

// FixMainnetBootstrap updates Options to use correct mainnet addresses
func FixMainnetBootstrap(opts *Options) {
	if opts.Network != "MainNet" {
		return
	}
	
	// Override with correct addresses
	opts.BootstrapPeers = []multiaddr.Multiaddr{}
	for _, p := range GetMainnetPeers() {
		for _, addr := range p.Addrs {
			// Build full multiaddr with peer ID
			fullAddr, err := multiaddr.NewMultiaddr("/p2p/" + p.ID.String())
			if err != nil {
				continue
			}
			combined := addr.Encapsulate(fullAddr)
			opts.BootstrapPeers = append(opts.BootstrapPeers, combined)
		}
	}
}

// ConnectToMainnetPeers manually connects to mainnet peers with correct addresses
func (n *Node) ConnectToMainnetPeers(ctx context.Context) error {
	for _, peerInfo := range GetMainnetPeers() {
		if err := n.host.Connect(ctx, peerInfo); err != nil {
			// Log but don't fail
			continue
		}
	}
	return nil
}