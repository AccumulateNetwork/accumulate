// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package leveldb

import (
	"os"

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
}

type opts struct {
}

type Option func(*opts) error

func OpenFile(filepath string, o ...Option) (*Database, error) {
	// Make sure all directories exist
	err := os.MkdirAll(filepath, 0700)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("create %q: %w", filepath, err)
	}

	db, err := leveldb.OpenFile(filepath, nil)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open %q: %w", filepath, err)
	}

	d := new(Database)
	d.leveldb = db
	for _, o := range o {
		err = o(&d.opts)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	return d, nil
}

func (d *Database) key(key *record.Key) []byte {
	h := key.Hash()
	return h[:]
}

// Begin begins a change set.
func (d *Database) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	snap, err := d.leveldb.GetSnapshot()

	// Read from the transaction
	get := func(key *record.Key) ([]byte, error) {
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

	// Commit to the write batch
	var commit memory.CommitFunc
	if writable {
		commit = func(entries map[[32]byte]memory.Entry) error {
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
	}

	// Discard the transaction
	discard := func() {
		snap.Release()
	}

	// The memory changeset caches entries in a map so Get will see values
	// updated with Put, regardless of the underlying transaction and write
	// batch behavior
	return memory.NewChangeSet(prefix, get, commit, discard)
}

// Close
// Close the underlying database
func (d *Database) Close() error {
	return d.leveldb.Close()
}
