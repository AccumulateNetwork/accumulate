// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package memory

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/internal/vmap"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
)

type Database struct {
	entries *vmap.Map[[32]byte, Entry]
	prefix  *record.Key
}

var _ keyvalue.Beginner = (*Database)(nil)

func New(prefix *record.Key) *Database {
	return &Database{prefix: prefix, entries: vmap.New[[32]byte, Entry]()}
}

// Begin begins a change set.
func (d *Database) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	v := d.entries.View()
	return NewChangeSet(ChangeSetOptions{
		Prefix:  prefix,
		Discard: v.Discard,
		Get: func(k *record.Key) ([]byte, error) {
			e, ok := v.Get(k.Hash())
			if !ok || e.Delete {
				return nil, (*database.NotFoundError)(k)
			}
			return e.Value, nil
		},
		ForEach: func(fn func(*record.Key, []byte) error) error {
			return v.ForEach(func(b [32]byte, e Entry) error {
				return fn(e.Key, e.Value)
			})
		},
		Commit: func(m map[[32]byte]Entry) error {
			for k, e := range m {
				v.Put(k, e)
			}
			return v.Commit()
		},
	})
}

// Export exports the database as a set of entries. Behavior is undefined if the
// database was created with a prefix. Export may return an error in the future.
// Export is not safe to use concurrently.
func (d *Database) Export() ([]Entry, error) {
	v := d.entries.View()
	defer v.Discard()
	var entries []Entry
	return entries, v.ForEach(func(k [32]byte, v Entry) error {
		entries = append(entries, v)
		return nil
	})
}

// Import imports a set of entries into the database. Behavior is undefined if
// the database was created with a prefix. Import is not safe to use
// concurrently.
func (d *Database) Import(entries []Entry) error {
	v := d.entries.View()
	defer v.Discard()

	for _, e := range entries {
		v.Put(e.Key.Hash(), e)
	}
	return v.Commit()
}
