package database

import (
	"encoding"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/smt/pmt"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
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

	b.store.Put(key, data)
	return nil
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
