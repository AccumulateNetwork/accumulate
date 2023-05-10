// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package badger

import (
	"os"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// TruncateBadger controls whether Badger is configured to truncate corrupted
// data. Especially on Windows, if the node is terminated abruptly, setting this
// may be necessary to recovering the state of the system.
//
// However, Accumulate is not robust against this kind of interruption. If the
// node is terminated abruptly and restarted with this flag, some functions may
// break, such as synthetic transactions and anchoring.
var TruncateBadger = false

type Database struct {
	badger *badger.DB
	logger logging.OptionalLogger
	ready  bool
	mu     sync.RWMutex
}

func New(filepath string, logger log.Logger) (*Database, error) {
	// Make sure all directories exist
	err := os.MkdirAll(filepath, 0700)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open badger: create %q: %w", filepath, err)
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

	d := new(Database)
	d.ready = true
	d.logger.Set(logger)

	// Open Badger
	d.badger, err = badger.Open(opts)
	if err != nil {
		return nil, err
	}

	// Run GC every hour
	go d.gc()

	return d, nil
}

// Begin begins a change set.
func (d *Database) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	c := new(ChangeSet)
	c.prefix = prefix
	c.db = d
	c.writable = writable
	c.badger = d.badger.NewTransaction(writable)
	return c
}

// Close
// Close the underlying database
func (d *Database) Close() error {
	if l, err := d.lock(true); err != nil {
		return err
	} else {
		defer l.Unlock()
	}

	d.ready = false
	return d.badger.Close()
}

func (d *Database) gc() {
	for {
		// GC every hour
		time.Sleep(time.Hour)

		// Still open?
		l, err := d.lock(false)
		if err != nil {
			return
		}

		// Run GC if 50% space could be reclaimed
		err = d.badger.RunValueLogGC(0.5)
		if err != nil && !errors.Is(err, badger.ErrNoRewrite) {
			d.logger.Error("Badger GC failed", "error", err)
		}

		// Release the lock
		l.Unlock()
	}
}

// lock acquires a lock on the ready mutex and checks for readiness. This
// prevents race conditions between Get/Put and Close, which can cause panics.
//
// This logic was built via trial and error and lots and lots of pain. The
// shutdown process of a Tendermint node is fairly non-deterministic, which lead
// to a lot of hard-to-reproduce issues showing up in CI tests. Weeks of
// guesswork lead to this solution.
func (d *Database) lock(closing bool) (sync.Locker, error) {
	var l sync.Locker = &d.mu
	if !closing {
		l = d.mu.RLocker()
	}

	l.Lock()
	if !d.ready {
		l.Unlock()
		return nil, errors.NotReady
	}

	return l, nil
}
