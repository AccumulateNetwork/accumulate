// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"path/filepath"
	"strings"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/accumulate"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Configuration interface {
	Type() ConfigurationType
	CopyAsInterface() any

	apply(cfg *Config) error
}

func (c *CoreValidatorConfiguration) apply(cfg *Config) error {
	// Set core validator defaults
	setDefaultPtr(&c.EnableHealing, false)
	setDefaultPtr(&c.StorageType, StorageTypeBadger)

	// Validate
	if c.Listen == nil {
		return errors.BadRequest.With("must specify a listen address")
	}
	if !addrHasOneOf(c.Listen, "tcp", "udp") {
		return errors.BadRequest.With("listen address must specify a port")
	}

	// Set the network
	if cfg.Network != "" && cfg.Network != c.Network {
		return errors.Conflict.WithFormat("network ID mismatch")
	}
	cfg.Network = c.Network

	var dnnNodeKey PrivateKey = &CometNodeKeyFile{Path: filepath.Join("dnn", "config", "node_key.json")}

	// Set P2P defaults
	setDefaultVal(&cfg.P2P, new(P2P))
	setDefaultVal(&cfg.P2P.BootstrapPeers, accumulate.BootstrapServers) // Bootstrap servers
	setDefaultVal(&cfg.P2P.Key, dnnNodeKey)                             // Key
	setDefaultVal(&cfg.P2P.Listen, []multiaddr.Multiaddr{               // Listen addresses
		c.listen("/ip4/0.0.0.0", portDir+portAccP2P, useTCP{}),
		c.listen("/ip4/0.0.0.0", portDir+portAccP2P, useQUIC{}),
		c.listen("/ip4/0.0.0.0", portBVN+portAccP2P, useTCP{}),
		c.listen("/ip4/0.0.0.0", portBVN+portAccP2P, useQUIC{}),
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
	if !haveService[*HttpService](cfg, nil) {
		cfg.Apps = append(cfg.Apps, &HttpService{
			Listen: []multiaddr.Multiaddr{
				c.listen("", portDir+portAccAPI),
				c.listen("", portBVN+portAccAPI),
			},
			Router: ServiceReference[*RouterService]("Directory"),
		})
	}

	return nil
}

func (c *CoreValidatorConfiguration) applyPart(cfg *Config, partID string, partType protocol.PartitionType, dir string) error {
	// Consensus
	if !haveService[*ConsensusService](cfg, func(c *ConsensusService) bool { return strings.EqualFold(c.App.partition().ID, partID) }) {
		cfg.Apps = append(cfg.Apps, &ConsensusService{
			NodeDir: dir,
			App: &CoreConsensusApp{
				EnableHealing: *c.EnableHealing,
				Partition: &protocol.PartitionInfo{
					ID:   partID,
					Type: partType,
				},
			},
		})
	}

	// Storage
	if !haveService[*StorageService](cfg, func(c *StorageService) bool { return strings.EqualFold(c.Name, partID) }) {
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

	// Services
	cfg.Services = append(cfg.Services,
		&Querier{Partition: partID},
		&NetworkService{Partition: partID},
		&MetricsService{Partition: partID},
		&EventsService{Partition: partID},
	)

	return nil
}

func (c *CoreValidatorConfiguration) listen(defaultHost string, transform ...addrTransform) multiaddr.Multiaddr {
	addr := c.Listen
	if defaultHost != "" {
		addr = ensureHost(addr, defaultHost)
	}
	return applyAddrTransforms(addr, transform...)
}

func haveService[T any](cfg *Config, predicate func(T) bool) bool {
	for _, s := range cfg.Apps {
		t, ok := s.(T)
		if ok && (predicate == nil || predicate(t)) {
			return true
		}
	}
	for _, s := range cfg.Services {
		t, ok := s.(T)
		if ok && (predicate == nil || predicate(t)) {
			return true
		}
	}
	return false
}
