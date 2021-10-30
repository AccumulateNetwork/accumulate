package badger

import (
	"errors"
	"os"

	"github.com/AccumulateNetwork/accumulated/smt/storage"
	"github.com/dgraph-io/badger"
)

type DB struct {
	DBHome   string
	badgerDB *badger.DB
}

var _ storage.KeyValueDB = (*DB)(nil)

// Close
// Close the underlying database
func (d *DB) Close() error {
	return d.badgerDB.Close()
}

// InitDB
// Initialize the database by the given filename (includes directory path).
// This will certainly open an existing database, but will also initialize
// an new, empty database.
func (d *DB) InitDB(filepath string) error {
	// Make sure all directories exist
	err := os.MkdirAll(filepath, 0777)
	if err != nil {
		return errors.New("failed to create home directory")
	}
	d.DBHome = filepath
	// Open Badger
	d.badgerDB, err = badger.Open(badger.DefaultOptions(d.DBHome))
	if err != nil { // Panic if we can't open Badger
		return err
	}
	return nil
}

// Get
// Look in the given bucket, and return the key found.  Returns nil if no value
// is found for the given key
func (d *DB) Get(key storage.Key) (value []byte, err error) {
	err = d.badgerDB.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key[:])
		if err != nil {
			return err
		}
		err = item.Value(func(val []byte) error {
			value = append(value, val...)
			return nil
		})
		return err
	})

	// If we didn't find the value, return ErrNotFound
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, storage.ErrNotFound
	}

	// If anything goes wrong, return nil
	if err != nil {
		return nil, err
	}

	return value, nil
}

// Put
// Put a key/value in the database.  We return an error if there was a problem
// writing the key/value pair to the database.
func (d *DB) Put(key storage.Key, value []byte) error {
	// Update the key/value in the database
	err := d.badgerDB.Update(func(txn *badger.Txn) error {
		err := txn.Set(key[:], value)
		return err
	})
	return err
}

// PutBatch
// Write all the transactions in a given batch of pending transactions.  The
// batch is emptied, so it can be reused.
func (d *DB) EndBatch(TXCache map[storage.Key][]byte) error {
	txn := d.badgerDB.NewTransaction(true)
	defer txn.Discard()

	for k, v := range TXCache {
		// The statement below takes a copy of K. This is necessary because K is
		// `var k [32]byte`, a fixed-length array, and arrays in go are
		// pass-by-value. This means that range variable K is overwritten on
		// each loop iteration. Without this statement, `k[:]` creates a slice
		// that points to the range variable, so every call to `txn.Set` gets a
		// slice pointing to the same memory. Since the transaction defers the
		// actual write until `txn.Commit` is called, it saves the slice. And
		// since all of the slices are pointing to the same variable, and that
		// variable is overwritten on each iteration, the slices held by `txn`
		// all point to the same value. When the transaction is committed, every
		// value is written to the last key. Taking a copy solves this because
		// each loop iteration creates a new copy, and `k[:]` references that
		// copy instead of the original. See also:
		// https://github.com/golang/go/wiki/CommonMistakes#using-reference-to-loop-iterator-variable
		k := k
		if err := txn.Set(k[:], v); err != nil {
			return err
		}
	}

	if err := txn.Commit(); err != nil {
		return err
	}

	return nil
}
