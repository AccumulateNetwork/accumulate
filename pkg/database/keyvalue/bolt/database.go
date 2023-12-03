// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bolt

import (
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	bolt "go.etcd.io/bbolt"
)

type Database struct {
	opts
	bolt  *bolt.DB
	ready bool
}

type opts struct {
	plainKeys bool
}

type Option func(*opts) error

func WithPlainKeys(o *opts) error {
	o.plainKeys = true
	return nil
}

func Open(filepath string, o ...Option) (*Database, error) {
	d := new(Database)
	var err error
	for _, o := range o {
		err = o(&d.opts)
		if err != nil {
			return nil, errors.UnknownError.Wrap(err)
		}
	}

	d.ready = true

	// Open
	d.bolt, err = bolt.Open(filepath, 0600, nil)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Database) bucket(tx *bolt.Tx, key *record.Key, create bool) (*bolt.Bucket, []byte, error) {
	if key.Len() == 0 {
		return nil, nil, errors.InternalError.With("invalid key (1)")
	}

	s, ok := key.Get(0).(string)
	if !ok {
		return nil, nil, errors.InternalError.With("invalid key (2)")
	}

	b := tx.Bucket([]byte(s))
	if b == nil {
		if !create {
			// No reason to do more work
			return nil, nil, nil
		}

		var err error
		b, err = tx.CreateBucket([]byte(s))
		if err != nil {
			return nil, nil, err
		}
	}

	if !d.plainKeys {
		h := key.SliceI(1).Hash()
		return b, h[:], nil
	}

	k, err := key.SliceI(1).MarshalBinary()
	if err != nil {
		return nil, nil, errors.InternalError.WithFormat("invalid key (3): %w", err)
	}
	return b, k, nil
}

// Begin begins a change set.
func (d *Database) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	// Use a read-only transaction for reading
	rd, err := d.bolt.Begin(false)
	if err != nil {
		// Should only occur if the database is closed
		panic(err)
	}

	// Discard the transaction
	discard := func() {
		_ = rd.Rollback()
	}

	// Read from the transaction
	get := func(key *record.Key) ([]byte, error) {
		b, k, err := d.bucket(rd, key, false)
		if err != nil {
			return nil, err
		}
		if b == nil {
			return nil, (*database.NotFoundError)(key)
		}

		v := b.Get(k)
		if v == nil {
			return nil, (*database.NotFoundError)(key)
		}

		u := make([]byte, len(v))
		copy(u, v)
		return u, nil
	}

	// Commit to the write batch
	var commit memory.CommitFunc
	if writable {
		commit = func(entries map[[32]byte]memory.Entry) error {
			// Discard the read transaction to unlock the database
			_ = rd.Rollback()

			return d.bolt.Update(func(tx *bolt.Tx) error {
				for _, e := range entries {
					b, k, err := d.bucket(tx, e.Key, true)
					if err != nil {
						return err
					}

					if e.Delete {
						err = b.Delete(k)
					} else {
						err = b.Put(k, e.Value)
					}
					if err != nil {
						return err
					}
				}
				return nil
			})
		}
	}

	// The memory changeset caches entries in a map so Get will see values
	// updated with Put, regardless of the underlying transaction and write
	// batch behavior
	return memory.NewChangeSet(prefix, get, commit, discard)
}

func (d *Database) Close() error {
	return d.bolt.Close()
}
