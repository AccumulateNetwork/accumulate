// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build dagbft

package run

import (
	"path/filepath"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (c *CoreValidatorConfiguration) apply(_ *Instance, cfg *Config) error {
	// Set core validator defaults
	setDefaultPtr(&c.StorageType, StorageTypeLevelDB)

	// Validate
	if c.Listen == nil {
		return errors.BadRequest.With("must specify a listen address")
	}
	if cfg.Network == "" {
		return errors.BadRequest.With("must specify the network")
	}
	if !addrHasOneOf(c.Listen, "tcp", "udp") {
		return errors.BadRequest.With("listen address must specify a port")
	}

	var nodeKey PrivateKey
	if c.Mode == CoreValidatorModeBVN {
		nodeKey = &CometNodeKeyFile{Path: filepath.Join("bvnn", "config", "node_key.json")}
	} else {
		nodeKey = &CometNodeKeyFile{Path: filepath.Join("dnn", "config", "node_key.json")}
	}

	// Set P2P defaults
	setDefaultVal(&cfg.P2P, new(P2P))
	setDefaultSlice(&cfg.P2P.BootstrapPeers, accumulate.BootstrapServers...) // Bootstrap servers
	setDefaultVal(&cfg.P2P.Key, nodeKey)                                     // Key
	setDefaultSlice(&cfg.P2P.Listen,                                         // Listen addresses
		listen(c.Listen, "/ip4/0.0.0.0", portDir+portAccP2P, useTCP{}),
		listen(c.Listen, "/ip4/0.0.0.0", portDir+portAccP2P, useQUIC{}),
		listen(c.Listen, "/ip4/0.0.0.0", portBVN+portAccP2P, useTCP{}),
		listen(c.Listen, "/ip4/0.0.0.0", portBVN+portAccP2P, useQUIC{}),
	)

	// Create partition services
	switch c.Mode {
	case CoreValidatorModeDN, CoreValidatorModeDual:
		err := partOpts{
			CoreValidatorConfiguration: c,

			ID:             protocol.Directory,
			Type:           protocol.PartitionTypeDirectory,
			Genesis:        c.DnGenesis,
			BootstrapPeers: c.DnBootstrapPeers,
			Dir:            "dnn",
			NumWorkers:     c.NumWorkers,
		}.apply(cfg)
		if err != nil {
			return err
		}
	}

	switch c.Mode {
	case CoreValidatorModeBVN, CoreValidatorModeDual:
		err := partOpts{
			CoreValidatorConfiguration: c,

			ID:             c.BVN,
			Type:           protocol.PartitionTypeBlockValidator,
			Genesis:        c.BvnGenesis,
			BootstrapPeers: c.BvnBootstrapPeers,
			Dir:            "bvnn",
			NumWorkers:     c.NumWorkers,
		}.apply(cfg)
		if err != nil {
			return err
		}
	}

	// Create HTTP configuration
	if !haveService[*HttpService](cfg, nil, nil) {
		var routerPart string
		switch c.Mode {
		case CoreValidatorModeDual,
			CoreValidatorModeDN:
			routerPart = "Directory"
		default:
			routerPart = c.BVN
		}

		cfg.Services = append(cfg.Services, &HttpService{
			HttpListener: HttpListener{Listen: []multiaddr.Multiaddr{
				listen(c.Listen, "", portDir+portAccAPI),
				listen(c.Listen, "", portBVN+portAccAPI),
			}},
			Router: ServiceReference[*RouterService](routerPart),
		})
	}

	return nil
}
