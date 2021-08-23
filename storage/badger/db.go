package badger

import (
	"errors"
	"os"

	"github.com/AccumulateNetwork/SMT/storage"
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
func (d *DB) Get(key [storage.KeyLength]byte) (value []byte) {
	err := d.badgerDB.View(func(txn *badger.Txn) error {
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
	// If anything goes wrong, return nil
	if err != nil {
		return nil
	}
	// If we didn't find the value, we will return a nil here.
	return value
}

// Put
// Put a key/value in the database.  We return an error if there was a problem
// writing the key/value pair to the database.
func (d *DB) Put(key [storage.KeyLength]byte, value []byte) error {
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
func (d *DB) PutBatch(TXList []storage.TX) error {
	txn := d.badgerDB.NewTransaction(true)
	for _, e := range TXList {
		if err := txn.Set(e.Key[:], e.Value); err != nil {
			panic(err)
		}

	}
	if err := txn.Commit(); err != nil {
		panic(err)
	}

	return nil
}
