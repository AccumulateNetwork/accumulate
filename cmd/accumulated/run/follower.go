// Copyright 2025 The Accumulate Authors
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (c *FollowerConfiguration) apply(inst *Instance, cfg *Config) error {
	// Set follower defaults
	setDefaultPtr(&c.StorageType, StorageTypeBadger)

	// Debug: log bootstrap peers
	inst.logger.Info("Follower configuration loaded",
		"dn_bootstrap_peers_count", len(c.DnBootstrapPeers),
		"bvn_bootstrap_peers_count", len(c.BvnBootstrapPeers))

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
	// CRITICAL: Use TransientPrivateKey for follower - it won't be in genesis
	// so CometBFT will assign voting_power=0 (follower mode)
	followerKey := &TransientPrivateKey{}

	switch c.Mode {
	case CoreValidatorModeDN, CoreValidatorModeDual:
		err := followerPartOpts{
			FollowerConfiguration: c,
			ValidatorKey:          followerKey,
			ID:                    protocol.Directory,
			Type:                  protocol.PartitionTypeDirectory,
			Genesis:               c.DnGenesis,
			BootstrapPeers:        c.DnBootstrapPeers,
			Dir:                   "dnn",
		}.apply(cfg)
		if err != nil {
			return err
		}
	}

	switch c.Mode {
	case CoreValidatorModeBVN, CoreValidatorModeDual:
		err := followerPartOpts{
			FollowerConfiguration: c,
			ValidatorKey:          followerKey,
			ID:                    c.BVN,
			Type:                  protocol.PartitionTypeBlockValidator,
			Genesis:               c.BvnGenesis,
			BootstrapPeers:        c.BvnBootstrapPeers,
			Dir:                   "bvnn",
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

type followerPartOpts struct {
	*FollowerConfiguration
	ValidatorKey     PrivateKey
	ID               string
	Type             protocol.PartitionType
	Genesis          string
	Dir              string
	BootstrapPeers   []multiaddr.Multiaddr
	MetricsNamespace string
}

func (p followerPartOpts) apply(cfg *Config) error {
	setDefaultPtr(&p.EnableSnapshots, false)

	var offset portOffset
	if p.Type == protocol.PartitionTypeDirectory {
		offset = portDir
	} else {
		offset = portBVN
	}

	// Consensus - with transient key for follower mode
	addService(cfg,
		&ConsensusService{
			NodeDir:          p.Dir,
			ValidatorKey:     p.ValidatorKey, // TransientPrivateKey - voting_power=0
			Genesis:          p.Genesis,
			Listen:           applyAddrTransforms(p.Listen, offset),
			BootstrapPeers:   p.BootstrapPeers,
			MetricsNamespace: p.MetricsNamespace,
			App: &CoreConsensusApp{
				EnableHealing:        p.EnableHealing,
				EnableDirectDispatch: p.EnableDirectDispatch,
				MaxEnvelopesPerBlock: p.MaxEnvelopesPerBlock,
				Partition: &protocol.PartitionInfo{
					ID:   p.ID,
					Type: p.Type,
				},
			},
		},
		func(c *ConsensusService) string { return c.App.partition().ID })

	// Storage
	if !haveService2(cfg, p.ID, func(s *StorageService) string { return s.Name }, nil) {
		storage, err := sStorage.New(*p.StorageType)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		storage.setPath(filepath.Join(p.Dir, "data", "accumulate.db"))
		cfg.Services = append(cfg.Services, &StorageService{Name: p.ID, Storage: storage})
	}

	// Snapshots; capture on every major block
	if *p.EnableSnapshots {
		addService(cfg,
			&SnapshotService{
				Partition: p.ID,
				Directory: filepath.Join(p.Dir, "snapshots"),
				Schedule:  network.MustParseCron("* * * * *")},
			func(s *SnapshotService) string { return s.Partition })
	}

	// Services
	addService(cfg, &Querier{Partition: p.ID}, func(s *Querier) string { return s.Partition })
	addService(cfg, &NetworkService{Partition: p.ID}, func(s *NetworkService) string { return s.Partition })
	addService(cfg, &MetricsService{Partition: p.ID}, func(s *MetricsService) string { return s.Partition })
	addService(cfg, &EventsService{Partition: p.ID}, func(s *EventsService) string { return s.Partition })

	return nil
}
