package badger

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/batch"
	"github.com/dgraph-io/badger"
)

// TruncateBadger controls whether Badger is configured to truncate corrupted
// data. Especially on Windows, if the node is terminated abruptly, setting this
// may be necessary to recovering the state of the system.
//
// However, Accumulate is not robust against this kind of interruption. If the
// node is terminated abruptly and restarted with this flag, some functions may
// break, such as synthetic transactions and anchoring.
var TruncateBadger = false

type DB struct {
	ready    bool
	readyMu  *sync.RWMutex
	badgerDB *badger.DB
	logger   storage.Logger
}

var _ storage.KeyValueStore = (*DB)(nil)

// Close
// Close the underlying database
func (d *DB) Close() error {
	if l, err := d.lock(true); err != nil {
		return err
	} else {
		defer l.Unlock()
	}

	d.ready = false
	return d.badgerDB.Close()
}

// InitDB
// Initialize the database by the given filename (includes directory path).
// This will certainly open an existing database, but will also initialize
// an new, empty database.
func (d *DB) InitDB(filepath string, logger storage.Logger) error {
	// Make sure all directories exist
	err := os.MkdirAll(filepath, 0700)
	if err != nil {
		return errors.New("failed to create home directory")
	}

	opts := badger.DefaultOptions(filepath)

	// Truncate corrupted data
	if TruncateBadger {
		opts = opts.WithTruncate(true)
	}

	// Add logger
	if logger != nil {
		opts = opts.WithLogger(badgerLogger{logger})
	}

	// Open Badger
	d.badgerDB, err = badger.Open(opts)
	if err != nil {
		return err
	}

	d.logger = logger
	d.ready = true
	d.readyMu = new(sync.RWMutex)

	// Run GC every hour
	go d.gc()

	return nil
}

func (d *DB) gc() {
	for {
		// GC every hour
		time.Sleep(time.Hour)

		// Still open?
		l, err := d.lock(false)
		if err != nil {
			return
		}

		// Run GC if 50% space could be reclaimed
		err = d.badgerDB.RunValueLogGC(0.5)
		if d.logger != nil && err != nil && !errors.Is(err, badger.ErrNoRewrite) {
			d.logger.Error("Badger GC failed", "error", err)
		}

		// Release the lock
		l.Unlock()
	}
}

// lock acquires a lock on the ready mutex and checks for readiness. This
// prevents race conditions between Get/Put and Close, which can cause panics.
func (d *DB) lock(closing bool) (sync.Locker, error) {
	var l sync.Locker = d.readyMu
	if !closing {
		l = d.readyMu.RLocker()
	}

	l.Lock()
	if !d.ready {
		l.Unlock()
		return nil, storage.ErrNotOpen
	}

	return l, nil
}

// Get
// Look in the given bucket, and return the key found.  Returns nil if no value
// is found for the given key
func (d *DB) Get(key storage.Key) (value []byte, err error) {
	if l, err := d.lock(false); err != nil {
		return nil, err
	} else {
		defer l.Unlock()
	}

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
	if l, err := d.lock(false); err != nil {
		return err
	} else {
		defer l.Unlock()
	}

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

func (db *DB) Begin() storage.KeyValueTxn {
	return batch.New(db, db.logger)
}

type badgerLogger struct {
	storage.Logger
}

func (l badgerLogger) format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return strings.TrimRight(s, "\n")
}

func (l badgerLogger) Errorf(format string, args ...interface{}) {
	l.Error(l.format(format, args...))
}

func (l badgerLogger) Warningf(format string, args ...interface{}) {
	l.Error(l.format(format, args...))
}

func (l badgerLogger) Infof(format string, args ...interface{}) {
	l.Info(l.format(format, args...))
}

func (l badgerLogger) Debugf(format string, args ...interface{}) {
	l.Debug(l.format(format, args...))
}
