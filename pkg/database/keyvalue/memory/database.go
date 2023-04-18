// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package memory

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type Database struct {
	mu      sync.RWMutex
	entries map[[32]byte]Entry
	prefix  *record.Key
}

var _ keyvalue.Beginner = (*Database)(nil)

func New(prefix *record.Key) *Database {
	return &Database{prefix: prefix}
}

// Begin begins a change set.
func (d *Database) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	return NewChangeSet(prefix, d.get, d.put)
}

// Export exports the database as a set of entries. Behavior is undefined if the
// database was created with a prefix. Export may return an error in the future.
// Export is not safe to use concurrently.
func (d *Database) Export() ([]Entry, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	entries := make([]Entry, 0, len(d.entries))
	for _, e := range d.entries {
		entries = append(entries, e)
	}
	return entries, nil
}

// Import imports a set of entries into the database. Behavior is undefined if
// the database was created with a prefix. Import is not safe to use
// concurrently.
func (d *Database) Import(entries []Entry) error {
	m := make(map[[32]byte]Entry, len(entries))
	for _, e := range entries {
		m[e.Key.Hash()] = e
	}
	return d.put(m)
}

func (d *Database) get(key *record.Key) ([]byte, error) {
	// Prefix the key
	key = d.prefix.AppendKey(key)

	d.mu.RLock()
	defer d.mu.RUnlock()
	entry, ok := d.entries[key.Hash()]
	if ok {
		return entry.Value, nil
	}

	// Not found
	return nil, errors.NotFound.WithFormat("%v not found", key)
}

func (d *Database) put(entries map[[32]byte]Entry) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.entries == nil {
		d.entries = make(map[[32]byte]Entry, len(entries))
	}

	for _, e := range entries {
		// Prefix the key
		key := d.prefix.AppendKey(e.Key)

		if e.Delete {
			delete(d.entries, key.Hash())
		} else {
			d.entries[key.Hash()] = e
		}
	}
	return nil
}
