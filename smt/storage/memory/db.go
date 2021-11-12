package memory

import (
	"sync"

	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

// DB
// Implements a key value store in memory.  Very basic, assumes no initial
// state for the database That must be handled by the caller, but see the
// notes on InitDB for future improvements.
type DB struct {
	Filename string
	Entries  map[storage.Key][]byte
	mutex    sync.Mutex
}

// EndBatch
// Takes all the key value pairs collected in a cache of mapped values and
// adds them to the database.  The assumption here is that the order in which
// the cache is applied to a key value store does not matter.
func (m *DB) EndBatch(txCache map[storage.Key][]byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for k, v := range txCache {
		m.Entries[k] = v
	}
	return nil
}

// GetKeys
// Return the keys in the DB.
func (m *DB) GetKeys() [][32]byte {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	keys := make([][32]byte, len(m.Entries))
	idx := 0
	for k := range m.Entries {
		keys[idx] = k
		idx++
	}
	return keys
}

// Copy
// Make a copy of the database; a useful function for testing
func (m *DB) Copy() *DB {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	db := new(DB)
	db.Filename = m.Filename
	db.Entries = make(map[storage.Key][]byte)
	for k, v := range m.Entries {
		db.Entries[k] = v
	}

	return db
}

// Close
// Nothing really to do but to clear the Entries map
func (m *DB) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for k := range m.Entries {
		delete(m.Entries, k)
	}
	return nil
}

// InitDB
// Initialize a database; a memory database has no existing state, so this
// routine does nothing with the filename.  We could save and restore
// a memory database from a file in the future.
//
// An existing memory database will be cleared by calling InitDB
func (m *DB) InitDB(filename string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.Filename = filename

	// Either allocate a new map, or clear an existing one.
	if m.Entries == nil {
		m.Entries = make(map[storage.Key][]byte)
	} else {
		for k := range m.Entries {
			delete(m.Entries, k)
		}
	}
	return nil
}

// Get
// Returns the value for a key from the database
func (m *DB) Get(key storage.Key) (value []byte, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	v, ok := m.Entries[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return v, nil
}

// Put
// Takes a key and a value, and puts the pair into the database
func (m *DB) Put(key storage.Key, value []byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.Entries[key] = value
	return nil
}
