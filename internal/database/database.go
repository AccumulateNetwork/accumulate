package database

import (
	"encoding"
	"fmt"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/etcd"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const markPower = 8

// Database is an Accumulate database.
type Database struct {
	store  storage.KeyValueStore
	logger log.Logger
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

// Begin starts a new batch.
func (d *Database) Begin(writable bool) *Batch {
	tx := new(Batch)
	tx.store = d.store.Begin(writable)
	tx.bpt = pmt.NewBPTManager(tx.store)
	return tx
}

// View runs the function with a read-only transaction.
func (d *Database) View(fn func(*Batch) error) error {
	batch := d.Begin(false)
	defer batch.Discard()
	return fn(batch)
}

// Update runs the function with a writable transaction and commits if the
// function succeeds.
func (d *Database) Update(fn func(*Batch) error) error {
	batch := d.Begin(true)
	defer batch.Discard()
	err := fn(batch)
	if err != nil {
		return err
	}
	return batch.Commit()
}

// Batch batches database writes.
type Batch struct {
	store storage.KeyValueTxn
	bpt   *pmt.Manager
}

func (b *Batch) getAs(key storage.Key, value encoding.BinaryUnmarshaler) error {
	data, err := b.store.Get(key)
	if err != nil {
		return err
	}

	return value.UnmarshalBinary(data)
}

func (b *Batch) putAs(key storage.Key, value encoding.BinaryMarshaler) error {
	data, err := value.MarshalBinary()
	if err != nil {
		return err
	}

	return b.store.Put(key, data)
}

// UpdateBpt updates the Patricia Tree hashes with the values from the updates
// since the last update.
func (b *Batch) UpdateBpt() error {
	return b.bpt.Bpt.Update()
}

// BptRootHash returns the root hash of the BPT.
func (b *Batch) BptRootHash() []byte {
	// Make a copy
	h := b.bpt.Bpt.RootHash
	return h[:]
}

// Account returns an Account for the given URL.
func (b *Batch) Account(u *url.URL) *Account {
	return &Account{b, account(u)}
}

// AccountByID returns an Account for the given ID.
//
// Deprecated: Use Account.
func (b *Batch) AccountByID(id []byte) *Account {
	return &Account{b, accountByID(id)}
}

//
// This is still needed in one place, so the deprecation warning is disabled in
// order to pass static analysis.

// AccountByKey returns an Account for the given storage key. This is used for
// creating snapshots from the BPT.
func (b *Batch) AccountByKey(key storage.Key) *Account {
	return &Account{b, accountBucket{objectBucket(key)}}
}

// Transaction returns a Transaction for the given transaction ID.
func (b *Batch) Transaction(id []byte) *Transaction {
	return &Transaction{b, transaction(id)}
}

// Import imports values from another database.
func (b *Batch) Import(db interface{ Export() map[storage.Key][]byte }) error {
	err := b.bpt.Bpt.Update()
	if err != nil {
		return err
	}
	err = b.store.PutAll(db.Export())
	if err != nil {
		return err
	}
	b.bpt = pmt.NewBPTManager(b.store)
	return nil
}

// Commit commits pending writes to the key-value store. Attempting to use the
// Batch after calling Commit will result in a panic.
func (b *Batch) Commit() error {
	err := b.UpdateBpt()
	if err != nil {
		return err
	}
	return b.store.Commit()
}

// Discard discards pending writes. Attempting to use the Batch after calling
// Discard will result in a panic.
func (b *Batch) Discard() {
	b.store.Discard()
}

func (b *Batch) GetMinorRootChainAnchor(network *config.Network) ([]byte, error) {
	ledger := b.Account(network.NodeUrl(protocol.Ledger))
	chain, err := ledger.ReadChain(protocol.MinorRootChain)
	if err != nil {
		return nil, err
	}
	return chain.Anchor(), nil
}
