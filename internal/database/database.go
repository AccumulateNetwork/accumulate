package database

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/etcd"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Database is an Accumulate database.
type Database struct {
	store  storage.KeyValueStore
	logger logging.OptionalLogger
	nextId uint64
}

// New creates a new database using the given key-value store.
func New(store storage.KeyValueStore, logger log.Logger) *Database {
	d := new(Database)
	d.store = store
	d.logger.Set(logger, "module", "database")

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
		return OpenEtcd(cfg.Accumulate.SubnetId, cfg.Accumulate.Storage.Etcd, logger)

	default:
		return nil, fmt.Errorf("unknown storage format %q", cfg.Accumulate.Storage.Type)
	}
}

func (d *Database) Close() error {
	return d.store.Close()
}

// Begin starts a new nested changeset.
func (d *Database) Begin(writable bool) *ChangeSet {
	d.nextId++
	return newChangeSet(fmt.Sprint(d.nextId), writable, d.store.Begin(writable), nil, d.logger.L)
}

// View runs the function with a read-only transaction.
func (d *Database) View(fn func(cs *ChangeSet) error) error {
	cs := d.Begin(false)
	defer cs.Discard()
	return fn(cs)
}

// Update runs the function with a writable transaction and commits if the
// function succeeds.
func (d *Database) Update(fn func(cs *ChangeSet) error) error {
	cs := d.Begin(true)
	defer cs.Discard()
	err := fn(cs)
	if err != nil {
		return err
	}
	return cs.Commit()
}
