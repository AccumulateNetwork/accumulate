// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"strings"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Offline tools read a stopped node through its configuration file, the way
// the node reads it, without starting it.

// loadExpanded loads the node configuration in file and expands its
// configurations (core validator, follower, netsim) into services and P2P
// settings, as starting the node would.
func loadExpanded(ctx context.Context, file string) (*Instance, *Config, error) {
	cfg := new(Config)
	err := cfg.LoadFrom(file)
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load %s: %w", file, err)
	}

	inst, err := New(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}

	for _, c := range cfg.Configurations {
		err = c.apply(inst, cfg)
		if err != nil {
			return nil, nil, err
		}
	}
	return inst, cfg, nil
}

// OpenStorage opens the store the node configured in file keeps for a
// partition. The node must be stopped.
func OpenStorage(ctx context.Context, file, partition string) (keyvalue.Beginner, error) {
	inst, cfg, err := loadExpanded(ctx, file)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, s := range cfg.Services {
		s, ok := s.(*StorageService)
		if !ok {
			continue
		}
		if strings.EqualFold(s.Name, partition) {
			return s.Storage.open(inst)
		}
		names = append(names, s.Name)
	}
	return nil, errors.NotFound.WithFormat("%s has no storage for %s; it has %v", file, partition, names)
}

// NodeIdentity returns the network, libp2p identity and P2P listen addresses
// of the node configured in file.
func NodeIdentity(ctx context.Context, file string) (string, peer.ID, []multiaddr.Multiaddr, error) {
	inst, cfg, err := loadExpanded(ctx, file)
	if err != nil {
		return "", "", nil, err
	}
	if cfg.P2P == nil || cfg.P2P.Key == nil {
		return "", "", nil, errors.BadRequest.WithFormat("%s has no node key", file)
	}
	sk, err := getPrivateKey(cfg.P2P.Key, inst)
	if err != nil {
		return "", "", nil, err
	}
	k, _, err := crypto.KeyPairFromStdKey(&sk)
	if err != nil {
		return "", "", nil, err
	}
	id, err := peer.IDFromPrivateKey(k)
	if err != nil {
		return "", "", nil, err
	}
	return cfg.Network, id, cfg.P2P.Listen, nil
}
