package database

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/etcd"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const markPower = 8

// Database is an Accumulate database.
type Database struct {
	store       storage.KeyValueStore
	logger      log.Logger
	nextBatchId int
}

// New creates a new database using the given key-value store.
func New(store storage.KeyValueStore, logger log.Logger) *Database {
	d := new(Database)
	d.store = store

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

// Close closes the database and the key-value store.
func (d *Database) Close() error {
	return d.store.Close()
}

// Account returns an Account for the given URL.
func (b *Batch) Account(u *url.URL) *Account {
	key := record.Key{"Account", u}
	return getOrCreateMap(&b.accounts, key, func() *Account {
		v := new(Account)
		v.logger = b.logger
		v.store = b.recordStore
		v.key = key
		v.batch = b
		return v
	})
}

func (b *Batch) getAccountUrl(key record.Key) (*url.URL, error) {
	v, err := record.NewValue(
		b.logger.L,
		b.recordStore,
		// This must match the key used for the account's main state
		key.Append("Main"),
		"account %[1]v",
		false,
		record.Union(protocol.UnmarshalAccount),
	).Get()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	return v.GetUrl(), nil
}

// AccountByID returns an Account for the given ID.
//
// This is still needed in one place, so the deprecation warning is disabled in
// order to pass static analysis.
//
// Deprecated: Use Account.
func (b *Batch) AccountByID(id []byte) (*Account, error) {
	u, err := b.getAccountUrl(record.Key{"Account", id})
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	return b.Account(u), nil
}

// Import imports values from another database.
func (b *Batch) Import(db interface{ Export() map[storage.Key][]byte }) error {
	return b.store.PutAll(db.Export())
}

func (b *Batch) GetMinorRootChainAnchor(describe *config.Describe) ([]byte, error) {
	ledger := b.Account(describe.NodeUrl(protocol.Ledger))
	chain, err := ledger.ReadChain(protocol.MinorRootChain)
	if err != nil {
		return nil, err
	}
	return chain.Anchor(), nil
}
