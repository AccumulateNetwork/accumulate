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

	"github.com/dgraph-io/badger/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"golang.org/x/exp/slog"
)

type DatabaseV3 struct {
	badger *badger.DB
	ready  bool
	mu     sync.RWMutex
}

func NewV3(filepath string) (*DatabaseV3, error) {
	// Make sure all directories exist
	err := os.MkdirAll(filepath, 0700)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open badger: create %q: %w", filepath, err)
	}

	opts := badger.DefaultOptions(filepath)
	opts = opts.WithLogger(slogger{})

	d := new(DatabaseV3)
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

// Begin begins a change set.
func (d *DatabaseV3) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	// Use a read-only transaction for reading
	rd := d.badger.NewTransaction(false)

	// The memory changeset caches entries in a map so Get will see values
	// updated with Put, regardless of the underlying transaction and write
	// batch behavior
	return memory.NewChangeSet(prefix,
		// Read from the transaction
		func(key *record.Key) ([]byte, error) {
			kh := key.Hash()
			item, err := rd.Get(kh[:])
			switch {
			case err == nil:
				// Ok
			case errors.Is(err, badger.ErrKeyNotFound):
				return nil, errors.NotFound.WithFormat("%v not found", key)
			default:
				return nil, err
			}

			// If we didn't find the value, return ErrNotFound
			v, err := item.ValueCopy(nil)
			switch {
			case err == nil:
				return v, nil
			case errors.Is(err, badger.ErrKeyNotFound):
				return nil, errors.NotFound.WithFormat("%v not found", key)
			default:
				return nil, errors.UnknownError.WithFormat("get %v: %w", key, err)
			}
		},

		// Commit to the write batch
		func(entries map[[32]byte]memory.Entry) error {
			l, err := d.lock(false)
			if err != nil {
				return err
			}
			defer l.Unlock()

			// Use a write batch for writing to work around Badger's limitations
			wr := d.badger.NewWriteBatch()

			for _, e := range entries {
				kh := e.Key.Hash()
				if e.Delete {
					err = wr.Delete(kh[:])
				} else {
					err = wr.Set(kh[:], e.Value)
				}
				if err != nil {
					return err
				}
			}

			return wr.Flush()
		})
}

// Close
// Close the underlying database
func (d *DatabaseV3) Close() error {
	if l, err := d.lock(true); err != nil {
		return err
	} else {
		defer l.Unlock()
	}

	d.ready = false
	return d.badger.Close()
}

func (d *DatabaseV3) gc() {
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
func (d *DatabaseV3) lock(closing bool) (sync.Locker, error) {
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
