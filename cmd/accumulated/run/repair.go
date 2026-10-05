// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"io"
	"path/filepath"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// OpenPartitionStore opens, for an operator, the store a stopped node keeps a
// partition in: the storage service of that name in the node's accumulate.toml
// under dataDir, opened as the node opens it. It also returns the partition's
// repair log, where the node records the failures it names (executor spec,
// "Sync", §3b), and a func that closes the store.
func OpenPartitionStore(ctx context.Context, dataDir, partition string) (keyvalue.Beginner, string, func() error, error) {
	cfg := new(Config)
	err := cfg.LoadFrom(filepath.Join(dataDir, "accumulate.toml"))
	if err != nil {
		return nil, "", nil, errors.UnknownError.WithFormat("load the node's config: %w", err)
	}
	inst, err := New(ctx, cfg)
	if err != nil {
		return nil, "", nil, errors.UnknownError.Wrap(err)
	}

	// A node's config may describe it by a configuration (coreValidator,
	// follower, devnet) that is expanded into services when the node starts:
	// the storage is looked for in the expansion.
	expanded, err := inst.Verify()
	if err != nil {
		return nil, "", nil, errors.UnknownError.WithFormat("expand the node's config: %w", err)
	}
	svc := findStorage(expanded.Services, partition)
	if svc == nil || svc.Storage == nil {
		return nil, "", nil, errors.NotFound.WithFormat("the node under %s has no storage for %s", dataDir, partition)
	}
	store, err := svc.Storage.open(inst)
	if err != nil {
		return nil, "", nil, errors.UnknownError.WithFormat("open %s's store: %w", partition, err)
	}
	closeStore := func() error {
		if c, ok := store.(io.Closer); ok {
			return c.Close()
		}
		return nil
	}
	return store, inst.path("consensus", strings.ToLower(partition), "repairs.jsonl"), closeStore, nil
}

// findStorage is the storage service named partition, in services or in a
// sub-node's.
func findStorage(services []Service, partition string) *StorageService {
	for _, s := range services {
		switch s := s.(type) {
		case *StorageService:
			if strings.EqualFold(s.Name, partition) {
				return s
			}
		case *SubnodeService:
			if found := findStorage(s.Services, partition); found != nil {
				return found
			}
		}
	}
	return nil
}
