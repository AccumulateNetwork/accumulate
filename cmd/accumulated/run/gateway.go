// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"os/user"
	"path/filepath"
	"strings"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func (g *GatewayConfiguration) apply(cfg *Config) error {
	// Validate
	if g.Listen != nil && !addrHasOneOf(g.Listen, "tcp", "udp") {
		return errors.BadRequest.With("listen address must specify a port")
	}

	// Set the P2P section
	setDefaultVal(&cfg.P2P, new(P2P))
	setDefaultVal(&cfg.P2P.BootstrapPeers, accumulate.BootstrapServers)

	if g.Listen != nil {
		setDefaultVal(&cfg.P2P.Listen, []multiaddr.Multiaddr{
			listen(g.Listen, "/ip4/0.0.0.0", portAccP2P, useTCP{}),
			listen(g.Listen, "/ip4/0.0.0.0", portAccP2P, useQUIC{}),
		})
	}

	if cu, err := user.Current(); err == nil {
		setDefaultVal(&cfg.P2P.PeerDB, filepath.Join(cu.HomeDir, ".accumulate", "cache", "peerdb.json"))
	}

	// Set the HTTP section
	http := addService(cfg, &HttpService{}, func(*HttpService) string { return "" })
	setDefaultVal(&http.Router, ServiceValue(&RouterService{}))

	if g.Listen != nil {
		setDefaultVal(&http.Listen, []multiaddr.Multiaddr{
			listen(g.Listen, "/ip4/0.0.0.0", portAccAPI, useHTTP{}),
		})
	}

	if strings.EqualFold(cfg.Network, "MainNet") {
		setDefaultVal(&http.PeerMap, []*HttpPeerMapEntry{
			{
				ID:         mustParsePeer("12D3KooWAgrBYpWEXRViTnToNmpCoC3dvHdmR6m1FmyKjDn1NYpj"),
				Addresses:  []multiaddr.Multiaddr{mustParseMulti("/dns/apollo-mainnet.accumulate.defidevs.io")},
				Partitions: []string{"Apollo", "Directory"},
			},
			{
				ID:         mustParsePeer("12D3KooWDqFDwjHEog1bNbxai2dKSaR1aFvq2LAZ2jivSohgoSc7"),
				Addresses:  []multiaddr.Multiaddr{mustParseMulti("/dns/yutu-mainnet.accumulate.defidevs.io")},
				Partitions: []string{"Yutu", "Directory"},
			},
			{
				ID:         mustParsePeer("12D3KooWHzjkoeAqe7L55tAaepCbMbhvNu9v52ayZNVQobdEE1RL"),
				Addresses:  []multiaddr.Multiaddr{mustParseMulti("/dns/chandrayaan-mainnet.accumulate.defidevs.io")},
				Partitions: []string{"Chandrayaan", "Directory"},
			},
		})
	}

	return nil
}
