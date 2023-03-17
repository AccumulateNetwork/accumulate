// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package memory

import (
	"encoding/hex"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// DB
// Implements a key value store in memory.  Very basic, assumes no initial
// state for the database That must be handled by the caller, but see the
// notes on InitDB for future improvements.
type DB struct {
	DBOpen        atomicBool
	entries       *store
	logger        storage.Logger
	debugWriteLog []writeLogEntry
	prefix        string
}

func New(logger storage.Logger) *DB {
	m := new(DB)
	m.logger = logger
	m.entries = new(store)
	m.DBOpen.Store(true)
	return m
}

func NewDB() *DB {
	return New(nil)
}

var _ storage.KeyValueStore = (*DB)(nil)

func (m *DB) WithPrefix(prefix string) storage.Beginner {
	n := new(DB)
	*n = *m
	n.prefix = prefix
	return n
}

// Ready
// Returns true if the database is open and ready to accept reads/writes
func (m *DB) Ready() bool {
	return m.DBOpen.Load()
}

// commit
// Takes all the key value pairs collected in a cache of mapped values and
// adds them to the database.  The assumption here is that the order in which
// the cache is applied to a key value store does not matter.
func (m *DB) commit(values map[storage.Key][]byte) error {
	if !m.Ready() {
		return storage.ErrNotOpen
	}
	m.entries.putAll(m.prefix, values)
	if debugLogWrites {
		for k, v := range values {
			m.debugWriteLog = append(m.debugWriteLog, writeLogEntry{
				key:    k,
				keyStr: k.String(),
				value:  hex.EncodeToString(v),
			})
		}
	}
	return nil
}

// Copy
// Make a copy of the database; a useful function for testing
func (m *DB) Copy() *DB {
	db := new(DB)
	db.entries = m.entries.copy()
	db.DBOpen.Store(true)
	return db
}

// Close
// Nothing really to do but to clear the Entries map
func (m *DB) Close() error {
	if !m.Ready() { // not an error to try and close a db twice
		return nil
	}

	m.DBOpen.Store(false) // When close is done, mark the database as such

	m.entries = nil
	return nil
}

func (m *DB) get(key storage.Key) (value []byte, err error) {
	if !m.Ready() {
		return nil, storage.ErrNotOpen
	}

	v, ok := m.entries.get(m.prefix, key)
	if !ok {
		return nil, errors.NotFound.WithFormat("key %v not found", key)
	}
	return append([]byte{}, v...), nil
}

type atomicBool int32

func (a *atomicBool) Store(x bool) {
	var v = 0
	if x {
		v = 1
	}
	atomic.StoreInt32((*int32)(a), int32(v))
}

func (a *atomicBool) Load() (v bool) {
	if atomic.LoadInt32((*int32)(a)) != 0 {
		v = true
	}
	return v
}
