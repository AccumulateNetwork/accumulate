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
	err := c.applyPart(cfg, protocol.Directory, protocol.PartitionTypeDirectory, "dnn")
	if err != nil {
		return err
	}

	err = c.applyPart(cfg, c.BVN, protocol.PartitionTypeBlockValidator, "bvnn")
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

func (c *CoreValidatorConfiguration) applyPart(cfg *Config, partID string, partType protocol.PartitionType, dir string) error {
	// Consensus
	addService(cfg,
		&ConsensusService{
			NodeDir: dir,
			App: &CoreConsensusApp{
				EnableHealing:        c.EnableHealing,
				EnableDirectDispatch: c.EnableDirectDispatch,
				Partition: &protocol.PartitionInfo{
					ID:   partID,
					Type: partType,
				},
			},
		},
		func(c *ConsensusService) string { return c.App.partition().ID })

	// Storage
	if !haveService2[*StorageService](cfg, partID, func(s *StorageService) string { return s.Name }, nil) {
		switch *c.StorageType {
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
			return errors.BadRequest.WithFormat("unsupported storage type %v", c.StorageType)
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
