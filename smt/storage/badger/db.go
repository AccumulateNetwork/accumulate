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
	cache    map[[32]byte][]byte
}

var _ storage.KeyValueStore = (*DB)(nil)

func New(filepath string, logger storage.Logger) (*DB, error) {
	// Make sure all directories exist
	err := os.MkdirAll(filepath, 0700)
	if err != nil {
		return nil, fmt.Errorf("open badger: create %q: %w", filepath, err)
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
	d := new(DB)
	d.badgerDB, err = badger.Open(opts)
	if err != nil {
		return nil, err
	}

	d.logger = logger
	d.ready = true
	d.readyMu = new(sync.RWMutex)

	// Run GC every hour
	go d.gc()

	return d, nil
}

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
		err = d.GC(0.5)
		if d.logger != nil && err != nil && !errors.Is(err, badger.ErrNoRewrite) {
			d.logger.Error("Badger GC failed", "error", err)
		}

		// Release the lock
		l.Unlock()
	}
}

var ErrNoRewrite = badger.ErrNoRewrite

func (d *DB) GC(discardRatio float64) error {
	return d.badgerDB.RunValueLogGC(discardRatio)
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
