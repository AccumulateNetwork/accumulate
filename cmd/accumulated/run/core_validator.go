// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"path/filepath"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (c *CoreValidatorConfiguration) apply(cfg *Config) error {
	// Set core validator defaults
	setDefaultPtr(&c.StorageType, StorageTypeBadger)

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

	var dnnNodeKey PrivateKey = &CometNodeKeyFile{Path: filepath.Join("dnn", "config", "node_key.json")}

	// Set P2P defaults
	setDefaultVal(&cfg.P2P, new(P2P))
	setDefaultVal(&cfg.P2P.BootstrapPeers, accumulate.BootstrapServers) // Bootstrap servers
	setDefaultVal(&cfg.P2P.Key, dnnNodeKey)                             // Key
	setDefaultVal(&cfg.P2P.Listen, []multiaddr.Multiaddr{               // Listen addresses
		listen(c.Listen, "/ip4/0.0.0.0", portDir+portAccP2P, useTCP{}),
		listen(c.Listen, "/ip4/0.0.0.0", portDir+portAccP2P, useQUIC{}),
		listen(c.Listen, "/ip4/0.0.0.0", portBVN+portAccP2P, useTCP{}),
		listen(c.Listen, "/ip4/0.0.0.0", portBVN+portAccP2P, useQUIC{}),
	})

	// Create partition services
	err := partOpts{
		CoreValidatorConfiguration: c,

		ID:             protocol.Directory,
		Type:           protocol.PartitionTypeDirectory,
		Genesis:        c.DnGenesis,
		BootstrapPeers: c.DnBootstrapPeers,
		Dir:            "dnn",
	}.apply(cfg)
	if err != nil {
		return err
	}

	err = partOpts{
		CoreValidatorConfiguration: c,

		ID:             c.BVN,
		Type:           protocol.PartitionTypeBlockValidator,
		Genesis:        c.BvnGenesis,
		BootstrapPeers: c.BvnBootstrapPeers,
		Dir:            "bvnn",
	}.apply(cfg)
	if err != nil {
		return err
	}

	// Create HTTP configuration
	if !haveService[*HttpService](cfg, nil, nil) {
		cfg.Services = append(cfg.Services, &HttpService{
			Listen: []multiaddr.Multiaddr{
				listen(c.Listen, "", portDir+portAccAPI),
				listen(c.Listen, "", portBVN+portAccAPI),
			},
			Router: ServiceReference[*RouterService]("Directory"),
		})
	}

	return nil
}

type partOpts struct {
	*CoreValidatorConfiguration
	ID             string
	Type           protocol.PartitionType
	Genesis        string
	Dir            string
	BootstrapPeers []multiaddr.Multiaddr
}

func (p partOpts) apply(cfg *Config) error {
	partID, partType, dir := p.ID, p.Type, p.Dir

	// Consensus
	addService(cfg,
		&ConsensusService{
			NodeDir:        dir,
			ValidatorKey:   p.ValidatorKey,
			Genesis:        p.Genesis,
			Listen:         p.Listen,
			BootstrapPeers: p.BootstrapPeers,
			App: &CoreConsensusApp{
				EnableHealing:        p.EnableHealing,
				EnableDirectDispatch: p.EnableDirectDispatch,
				Partition: &protocol.PartitionInfo{
					ID:   partID,
					Type: partType,
				},
			},
		},
		func(c *ConsensusService) string { return c.App.partition().ID })

	// Storage
	if !haveService2[*StorageService](cfg, partID, func(s *StorageService) string { return s.Name }, nil) {
		switch *p.StorageType {
		case StorageTypeMemory:
			cfg.Services = append(cfg.Services, &StorageService{
				Name:    partID,
				Storage: &MemoryStorage{},
			})

		case StorageTypeBadger:
			cfg.Services = append(cfg.Services, &StorageService{
				Name: partID,
				Storage: &BadgerStorage{
					Path: filepath.Join(dir, "data", "accumulate.db"),
				},
			})

		default:
			return errors.BadRequest.WithFormat("unsupported storage type %v", p.StorageType)
		}
	}

	// Snapshots
	addService(cfg,
		&SnapshotService{
			Partition: partID,
			Directory: filepath.Join(dir, "snapshots"),
		},
		func(s *SnapshotService) string { return s.Partition })

	// Services
	addService(cfg, &Querier{Partition: partID}, func(s *Querier) string { return s.Partition })
	addService(cfg, &NetworkService{Partition: partID}, func(s *NetworkService) string { return s.Partition })
	addService(cfg, &MetricsService{Partition: partID}, func(s *MetricsService) string { return s.Partition })
	addService(cfg, &EventsService{Partition: partID}, func(s *EventsService) string { return s.Partition })

	return nil
}
