package database

import (
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/smt/pmt"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/badger"
	"github.com/AccumulateNetwork/accumulate/smt/storage/memory"
	"github.com/tendermint/tendermint/libs/log"
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

// Open opens a key-value store and creates a new database with it.
func Open(file string, useMemDB bool, logger log.Logger) (*Database, error) {
	var store storage.KeyValueStore = new(badger.DB)
	if useMemDB {
		store = new(memory.DB)
	}

	var storeLogger log.Logger
	if logger != nil {
		storeLogger = logger.With("module", "storage")
	}

	err := store.InitDB(file, storeLogger)
	if err != nil {
		return nil, err
	}

	return New(store, logger), nil
}

func (d *Database) logDebug(msg string, keyVals ...interface{}) {
	if d.logger != nil {
		d.logger.Debug(msg, keyVals...)
	}
}

func (d *Database) logInfo(msg string, keyVals ...interface{}) {
	if d.logger != nil {
		d.logger.Info(msg, keyVals...)
	}
}

// Close closes the database and the key-value store.
func (d *Database) Close() error {
	return d.store.Close()
}

// Begin starts a new batch.
func (d *Database) Begin() *Batch {
	tx := new(Batch)
	tx.store = d.store.Begin()
	tx.bpt = pmt.NewBPTManager(tx.store)
	return tx
}

// Batch batches database writes.
type Batch struct {
	store storage.KeyValueTxn
	bpt   *pmt.Manager
}

func (b *Batch) putBpt(key storage.Key, hash [32]byte) {
	b.bpt.Bpt.Insert(key, hash)
}

// UpdateBpt updates the Patricia Tree hashes with the values from the updates
// since the last update.
func (b *Batch) UpdateBpt() {
	b.bpt.Bpt.Update()
}

// RootHash returns the root hash of the BPT.
func (b *Batch) RootHash() []byte {
	// Make a copy
	h := b.bpt.Bpt.Root.Hash
	return h[:]
}

// Record returns a Record for the given URL.
func (b *Batch) Record(u *url.URL) *Record {
	return &Record{b, record(u)}
}

// RecordByID returns a Record for the given resource chain ID.
//
// Deprecated: Use Record.
func (b *Batch) RecordByID(id []byte) *Record {
	return &Record{b, recordFromChain(id)}
}

// Transaction returns a Transaction for the given transaction ID.
func (b *Batch) Transaction(id []byte) *Transaction {
	return &Transaction{b, transaction(id)}
}

// Import imports values from another database.
func (b *Batch) Import(db interface{ Export() map[storage.Key][]byte }) {
	b.bpt.Bpt.Update()
	b.store.PutAll(db.Export())
	b.bpt = pmt.NewBPTManager(b.store)
}

// Commit commits pending writes to the key-value store. Attempting to use the
// Batch after calling Commit will result in a panic.
func (b *Batch) Commit() error {
	b.UpdateBpt()
	return b.store.Commit()
}

// Discard discards pending writes. Attempting to use the Batch after calling
// Discard will result in a panic.
func (b *Batch) Discard() {
	b.store.Discard()
}
