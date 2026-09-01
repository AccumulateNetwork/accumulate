// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/hash"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const markPower = 8

// Database is an Accumulate database.
type Database struct {
	store       keyvalue.Beginner
	logger      logging.Logger
	nextBatchId int64
	observer    Observer
}

// New creates a new database using the given key-value store.
func New(store keyvalue.Beginner, logger logging.Logger) *Database {
	d := new(Database)
	d.store = store
	d.observer = NewDatabaseObserver()

	if logger != nil {
		d.logger = logger.With("module", "database")
	}

	return d
}

// Deep returns a view of this database whose batches read the store's
// whole history, not just the recent window it answers protocol reads
// from.
//
// BlockchainDB answers a permanent read from the last N to 2N blocks
// and calls anything older absent: probing history per segment on
// every miss was 23% of a validator's CPU and grew with the chain. The
// data is still there. A reader that knowingly looks back -- the API
// serving an explorer, healing re-proving an old range -- takes this
// once, at construction, and every batch it begins reaches history
// without any of its call sites changing. The executor keeps the
// windowed database, so the cost stays with the callers that need it.
//
// On a store with no window this returns an equivalent database: their
// batches already read everything.
func (d *Database) Deep() *Database {
	e := *d
	e.store = keyvalue.Deep(d.store)
	return &e
}

func OpenInMemory(logger logging.Logger) *Database {
	store := memory.New(nil)
	return New(store, logger)
}

func OpenBadger(filepath string, logger logging.Logger) (*Database, error) {
	store, err := badger.New(filepath)
	if err != nil {
		return nil, err
	}
	return New(store, logger), nil
}

func OpenLevelDB(filepath string, logger logging.Logger) (*Database, error) {
	store, err := leveldb.Open(filepath)
	if err != nil {
		return nil, err
	}
	return New(store, logger), nil
}

// Open opens a key-value store and creates a new database with it.
func Open(cfg *config.Config, logger logging.Logger) (*Database, error) {
	switch cfg.Accumulate.Storage.Type {
	case config.MemoryStorage:
		return OpenInMemory(logger), nil

	case config.BadgerStorage:
		return OpenBadger(config.MakeAbsolute(cfg.RootDir, cfg.Accumulate.Storage.Path), logger)

	case config.LevelDBStorage:
		return OpenLevelDB(config.MakeAbsolute(cfg.RootDir, cfg.Accumulate.Storage.Path), logger)

	default:
		return nil, fmt.Errorf("unknown storage format %q", cfg.Accumulate.Storage.Type)
	}
}

// Store returns the underlying key-value store. Store may return an error in
// the future.
func (d *Database) Store() (keyvalue.Beginner, error) {
	return d.store, nil
}

// SetObserver sets the database observer.
//
// Deprecated: The production observer is now the default. This method should
// only be used for testing with specialized observers.
func (d *Database) SetObserver(observer Observer) {
	if observer == nil {
		panic("SetObserver called with nil")
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
