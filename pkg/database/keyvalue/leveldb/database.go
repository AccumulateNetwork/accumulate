// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package leveldb

import (
	"os"
	"sync"
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

type Database struct {
	opts
	leveldb *leveldb.DB
	closing atomic.Bool
	open    *sync.WaitGroup
}

type opts struct {
}

type Option func(*opts)

func Open(filepath string, o ...Option) (*Database, error) {
	// Make sure all directories exist
	err := os.MkdirAll(filepath, 0700)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("create %q: %w", filepath, err)
	}

	db, err := leveldb.OpenFile(filepath, nil)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open %q: %w", filepath, err)
	}

	return New(db, o...), nil
}

func New(db *leveldb.DB, o ...Option) *Database {
	d := new(Database)
	d.leveldb = db
	d.open = new(sync.WaitGroup)
	for _, o := range o {
		o(&d.opts)
	}

	return d
}

func (d *Database) key(key *record.Key) []byte {
	h := key.Hash()
	return h[:]
}

// Begin begins a change set.
func (d *Database) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	var snap *leveldb.Snapshot
	var err error

	if d.closing.Load() {
		err = errors.Conflict.With("closed")
	} else {
		snap, err = d.leveldb.GetSnapshot()
	}

	// Read from the transaction
	get := func(key *record.Key) ([]byte, error) {
		return d.get(snap, err, key)
	}

	// Commit to the write batch
	var commit memory.CommitFunc
	if writable {
		commit = d.commit
	}

	forEach := func(fn func(*record.Key, []byte) error) error {
		return d.forEach(snap, err, fn)
	}

	discard := func() {}
	if err == nil {
		d.open.Add(1)
		var once sync.Once
		discard = func() {
			defer once.Do(d.open.Done)
			snap.Release()
		}
	}

	// The memory changeset caches entries in a map so Get will see values
	// updated with Put, regardless of the underlying transaction and write
	// batch behavior
	return memory.NewChangeSet(memory.ChangeSetOptions{
		Prefix:  prefix,
		Get:     get,
		Commit:  commit,
		ForEach: forEach,
		Discard: discard,
	})
}

func (d *Database) commit(entries map[[32]byte]memory.Entry) error {
	batch := new(leveldb.Batch)
	for _, e := range entries {
		if e.Delete {
			batch.Delete(d.key(e.Key))
		} else {
			batch.Put(d.key(e.Key), e.Value)
		}
	}

	return d.leveldb.Write(batch, nil)
}

func (d *Database) get(snap *leveldb.Snapshot, err error, key *record.Key) ([]byte, error) {
	if err != nil {
		return nil, err
	}

	v, err := snap.Get(d.key(key), nil)
	switch {
	case err == nil:
		u := make([]byte, len(v))
		copy(u, v)
		return u, nil
	case errors.Is(err, leveldb.ErrNotFound):
		return nil, (*database.NotFoundError)(key)
	default:
		return nil, err
	}
}

func (d *Database) forEach(snap *leveldb.Snapshot, err error, fn func(*record.Key, []byte) error) error {
	if err != nil {
		return err
	}

	it := snap.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		key := record.KeyFromHash(*(*[32]byte)(it.Key()))
		value := make([]byte, len(it.Value()))
		copy(value, it.Value())
		err = fn(key, value)
		if err != nil {
			return err
		}
	}
	it.Release()
	return it.Error()
}

// Close the database.
func (d *Database) Close() error {
	// Stop new batches
	d.closing.Store(true)

	// Wait for existing batches to resolve
	d.open.Wait()

	// Close the database
	return d.leveldb.Close()
}
