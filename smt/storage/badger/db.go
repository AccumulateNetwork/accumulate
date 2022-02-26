package badger

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
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

func (db *DB) Begin(writable bool) storage.KeyValueTxn {
	b := new(Batch)
	b.txn = db.badgerDB.NewTransaction(writable)
	if db.logger == nil {
		return b
	}
	return &storage.DebugBatch{Batch: b, Logger: db.logger}
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
