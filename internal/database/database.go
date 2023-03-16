// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"fmt"
	"io"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/etcd"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage/memory"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const markPower = 8

// Database is an Accumulate database.
type Database struct {
	store       storage.Beginner
	logger      log.Logger
	nextBatchId int64
	observer    Observer
}

// New creates a new database using the given key-value store.
func New(store storage.Beginner, logger log.Logger) *Database {
	d := new(Database)
	d.store = store
	d.observer = unsetObserver{}

	if logger != nil {
		d.logger = logger.With("module", "database")
	}

	return d
}

func OpenInMemory(logger log.Logger) *Database {
	var storeLogger log.Logger
	if logger != nil {
		storeLogger = logger.With("module", "storage")
	}

	store := memory.New(storeLogger)
	return New(store, logger)
}

func OpenBadger(filepath string, logger log.Logger) (*Database, error) {
	var storeLogger log.Logger
	if logger != nil {
		storeLogger = logger.With("module", "storage")
	}

	store, err := badger.New(filepath, storeLogger)
	if err != nil {
		return nil, err
	}
	return New(store, logger), nil
}

func OpenEtcd(prefix string, config *clientv3.Config, logger log.Logger) (*Database, error) {
	var storeLogger log.Logger
	if logger != nil {
		storeLogger = logger.With("module", "storage")
	}

	store, err := etcd.New(prefix, config, storeLogger)
	if err != nil {
		return nil, err
	}
	return New(store, logger), nil
}

// Open opens a key-value store and creates a new database with it.
func Open(cfg *config.Config, logger log.Logger) (*Database, error) {
	switch cfg.Accumulate.Storage.Type {
	case config.MemoryStorage:
		return OpenInMemory(logger), nil

	case config.BadgerStorage:
		return OpenBadger(config.MakeAbsolute(cfg.RootDir, cfg.Accumulate.Storage.Path), logger)

	case config.EtcdStorage:
		return OpenEtcd(cfg.Accumulate.PartitionId, cfg.Accumulate.Storage.Etcd, logger)

	default:
		return nil, fmt.Errorf("unknown storage format %q", cfg.Accumulate.Storage.Type)
	}
}

// SetObserver sets the database observer.
func (d *Database) SetObserver(observer Observer) {
	if observer == nil {
		observer = unsetObserver{}
	}
	d.observer = observer
}

// Close closes the database and the key-value store.
func (d *Database) Close() error {
	if c, ok := d.store.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// Import imports values from another database.
func (b *Batch) Import(db interface{ Export() map[storage.Key][]byte }) error {
	return b.kvstore.PutAll(db.Export())
}

func (b *Batch) GetMinorRootChainAnchor(describe *config.Describe) ([]byte, error) {
	ledger := b.Account(describe.NodeUrl(protocol.Ledger))
	chain, err := ledger.RootChain().Get()
	if err != nil {
		return nil, err
	}
	return chain.Anchor(), nil
}

type Observer interface {
	DidChangeAccount(batch *Batch, account *Account) (hash.Hasher, error)
}

type unsetObserver struct{}

func (unsetObserver) DidChangeAccount(batch *Batch, account *Account) (hash.Hasher, error) {
	return nil, errors.NotReady.WithFormat("cannot modify account - observer is not set")
}
