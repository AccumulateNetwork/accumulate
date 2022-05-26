package database

import (
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
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
		return OpenEtcd(cfg.Accumulate.Network.LocalSubnetID, cfg.Accumulate.Storage.Etcd, logger)

	default:
		return nil, fmt.Errorf("unknown storage format %q", cfg.Accumulate.Storage.Type)
	}
}

// Close closes the database and the key-value store.
func (d *Database) Close() error {
	return d.store.Close()
}

// // BptRootHash returns the root hash of the BPT.
// func (b *Batch) BptRootHash() []byte {
// 	// Make a copy
// 	h := b.bpt.Bpt.RootHash
// 	return h[:]
// }

// Account returns an Account for the given URL.
func (b *Batch) Account(u *url.URL) *Account {
	return &Account{b, account(u), u}
}

// AccountByID returns an Account for the given ID.
//
// This is still needed in one place, so the deprecation warning is disabled in
// order to pass static analysis.
//
// Deprecated: Use Account.
func (b *Batch) AccountByID(id []byte) (*Account, error) {
	a := &Account{b, accountByID(id), nil}
	state, err := a.GetState()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}
	a.url = state.GetUrl()
	return a, nil
}

// Import imports values from another database.
func (b *Batch) Import(db interface{ Export() map[storage.Key][]byte }) error {
	return b.store.PutAll(db.Export())
}

func (b *Batch) GetMinorRootChainAnchor(network *config.Network) ([]byte, error) {
	ledger := b.Account(network.NodeUrl(protocol.Ledger))
	chain, err := ledger.ReadChain(protocol.MinorRootChain)
	if err != nil {
		return nil, err
	}
	return chain.Anchor(), nil
}
