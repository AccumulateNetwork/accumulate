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
		cfg.Apps = append(cfg.Apps, &HttpService{
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
				EnableHealing: *c.EnableHealing,
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

func listen(addr multiaddr.Multiaddr, defaultHost string, transform ...addrTransform) multiaddr.Multiaddr {
	if defaultHost != "" {
		addr = ensureHost(addr, defaultHost)
	}
	return applyAddrTransforms(addr, transform...)
}

func haveService[T any](cfg *Config, predicate func(T) bool, existing *T) bool {
	for _, s := range [][]Service{cfg.Apps, cfg.Services} {
		for _, s := range s {
			t, ok := s.(T)
			if ok && (predicate == nil || predicate(t)) {
				if existing != nil {
					*existing = t
				}
				return true
			}
		}
	}
	return false
}

func haveService2[T any](cfg *Config, wantID string, getID func(T) string, existing *T) bool {
	return haveService(cfg, func(s T) bool {
		return strings.EqualFold(wantID, getID(s))
	}, existing)
}

func addService[T Service](cfg *Config, s T, getID func(T) string) T {
	if !haveService2(cfg, getID(s), getID, &s) {
		cfg.Services = append(cfg.Services, s)
	}
	return s
}
