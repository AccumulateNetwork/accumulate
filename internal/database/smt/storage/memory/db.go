// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package memory

import (
	"encoding/hex"
	"sync"
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
	entries       map[PrefixedKey][]byte
	mutex         sync.Mutex
	logger        storage.Logger
	debugWriteLog []writeLogEntry
}

func New(logger storage.Logger) *DB {
	m := new(DB)
	m.logger = logger
	m.entries = make(map[PrefixedKey][]byte)
	m.DBOpen.Store(true)
	return m
}

func NewDB() *DB {
	return New(nil)
}

var _ storage.KeyValueStore = (*DB)(nil)

// Ready
// Returns true if the database is open and ready to accept reads/writes
func (m *DB) Ready() bool {
	return m.DBOpen.Load()
}

// commit
// Takes all the key value pairs collected in a cache of mapped values and
// adds them to the database.  The assumption here is that the order in which
// the cache is applied to a key value store does not matter.
func (m *DB) commit(txCache map[PrefixedKey][]byte) error {
	m.mutex.Lock()
	if !m.Ready() {
		return storage.ErrNotOpen
	}
	defer m.mutex.Unlock()
	for k, v := range txCache {
		m.entries[k] = v
		if debugLogWrites {
			m.debugWriteLog = append(m.debugWriteLog, writeLogEntry{
				key:    k,
				keyStr: k.String(),
				value:  hex.EncodeToString(v),
			})
		}
	}
	return nil
}

// export writes the database to a map
func (m *DB) export() map[PrefixedKey][]byte {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.Ready() {
		return nil
	}

	ex := make(map[PrefixedKey][]byte, len(m.entries))
	for k, v := range m.entries {
		ex[k] = v
	}
	return ex
}

// Copy
// Make a copy of the database; a useful function for testing
func (m *DB) Copy() *DB {
	db := new(DB)
	db.entries = m.export()
	db.DBOpen.Store(true)
	return db
}

// Close
// Nothing really to do but to clear the Entries map
func (m *DB) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.Ready() { // not an error to try and close a db twice
		return nil
	}

	m.DBOpen.Store(false) // When close is done, mark the database as such

	for k := range m.entries {
		delete(m.entries, k)
	}
	return nil
}

func (m *DB) get(prefix string, key storage.Key) (value []byte, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.Ready() {
		return nil, storage.ErrNotOpen
	}

	v, ok := m.entries[PrefixedKey{prefix, key}]
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
