// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package badger

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"golang.org/x/exp/slog"
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
	opts
	badger *badger.DB
	ready  bool
	mu     sync.RWMutex
}

type opts struct {
	plainKeys bool
}

type Option func(*opts) error

func WithPlainKeys(o *opts) error {
	o.plainKeys = true
	return nil
}

func New(filepath string, o ...Option) (*Database, error) {
	// Make sure all directories exist
	err := os.MkdirAll(filepath, 0700)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open badger: create %q: %w", filepath, err)
	}

	opts := badger.DefaultOptions(filepath)
	opts = opts.WithLogger(Slogger{})

	// Truncate corrupted data
	if TruncateBadger {
		opts = opts.WithTruncate(true)
	}

	d := new(Database)
	for _, o := range o {
		err = o(&d.opts)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	d.ready = true

	// Open Badger
	d.badger, err = badger.Open(opts)
	if err != nil {
		return nil, err
	}

	// Run GC every hour
	go d.gc()

	return d, nil
}

func (d *Database) key(key *record.Key) []byte {
	if d.plainKeys {
		b, err := key.MarshalBinary()
		if err != nil {
			panic(err)
		}
		return b
	}
	h := key.Hash()
	return h[:]
}

// Begin begins a change set.
func (d *Database) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	// Use a read-only transaction for reading
	rd := d.badger.NewTransaction(false)

	// Read from the transaction
	get := func(key *record.Key) ([]byte, error) {
		item, err := rd.Get(d.key(key))
		switch {
		case err == nil:
			// Ok
		case errors.Is(err, badger.ErrKeyNotFound):
			return nil, (*database.NotFoundError)(key)
		default:
			return nil, err
		}

		// If we didn't find the value, return ErrNotFound
		v, err := item.ValueCopy(nil)
		switch {
		case err == nil:
			return v, nil
		case errors.Is(err, badger.ErrKeyNotFound):
			return nil, (*database.NotFoundError)(key)
		default:
			return nil, errors.UnknownError.WithFormat("get %v: %w", key, err)
		}
	}

	// Commit to the write batch
	var commit memory.CommitFunc
	if writable {
		commit = func(entries map[[32]byte]memory.Entry) error {
			l, err := d.lock(false)
			if err != nil {
				return err
			}
			defer l.Unlock()

			// Use a write batch for writing to work around Badger's limitations
			wr := d.badger.NewWriteBatch()

			for _, e := range entries {
				if e.Delete {
					err = wr.Delete(d.key(e.Key))
				} else {
					err = wr.Set(d.key(e.Key), e.Value)
				}
				if err != nil {
					return err
				}
			}

			return wr.Flush()
		}
	}

	// Discard the transaction
	discard := func() {
		// Fix https://discuss.dgraph.io/t/badgerdb-consume-too-much-disk-space/17070?
		rd.Discard()
	}

	// The memory changeset caches entries in a map so Get will see values
	// updated with Put, regardless of the underlying transaction and write
	// batch behavior
	return memory.NewChangeSet(prefix, get, commit, discard)
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
			slog.Error("Badger GC failed", "error", err, "module", "badger")
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

type Slogger struct{}

func (l Slogger) format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return strings.TrimRight(s, "\n")
}

func (l Slogger) Errorf(format string, args ...interface{}) {
	slog.Error(l.format(format, args...), "module", "badger")
}

func (l Slogger) Warningf(format string, args ...interface{}) {
	slog.Warn(l.format(format, args...), "module", "badger")
}

func (l Slogger) Infof(format string, args ...interface{}) {
	slog.Info(l.format(format, args...), "module", "badger")
}

func (l Slogger) Debugf(format string, args ...interface{}) {
	slog.Debug(l.format(format, args...), "module", "badger")
}
