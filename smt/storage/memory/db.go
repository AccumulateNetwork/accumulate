package memory

import (
	"bytes"
	"sort"
	"sync"

	"github.com/AccumulateNetwork/accumulate/internal/encoding"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

// DB
// Implements a key value store in memory.  Very basic, assumes no initial
// state for the database That must be handled by the caller, but see the
// notes on InitDB for future improvements.
type DB struct {
	entries map[storage.Key][]byte
	mutex   sync.Mutex
}

// EndBatch
// Takes all the key value pairs collected in a cache of mapped values and
// adds them to the database.  The assumption here is that the order in which
// the cache is applied to a key value store does not matter.
func (m *DB) EndBatch(txCache map[storage.Key][]byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for k, v := range txCache {
		m.entries[k] = v
	}
	return nil
}

// GetKeys
// Return the keys in the DB.
func (m *DB) GetKeys() [][32]byte {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	keys := make([][32]byte, len(m.entries))
	idx := 0
	for k := range m.entries {
		keys[idx] = k
		idx++
	}
	return keys
}

// Export writes the database to a map
func (m *DB) Export() map[storage.Key][]byte {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	ex := make(map[storage.Key][]byte)
	for k, v := range m.entries {
		ex[k] = v
	}
	return ex
}

// Copy
// Make a copy of the database; a useful function for testing
func (m *DB) Copy() *DB {
	db := new(DB)
	db.entries = m.Export()
	return db
}

// Close
// Nothing really to do but to clear the Entries map
func (m *DB) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for k := range m.entries {
		delete(m.entries, k)
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

	// Either allocate a new map, or clear an existing one.
	if m.entries == nil {
		m.entries = make(map[storage.Key][]byte)
	} else {
		for k := range m.entries {
			delete(m.entries, k)
		}
	}
	return nil
}

// Get
// Returns the value for a key from the database
func (m *DB) Get(key storage.Key) (value []byte, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	v, ok := m.entries[key]
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
	m.entries[key] = value
	return nil
}

func (m *DB) MarshalBinary() ([]byte, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var size uint64
	var keys []storage.Key
	for key, entry := range m.entries {
		keys = append(keys, key)
		n := uint64(len(entry))
		size += 32
		size += uint64(encoding.UvarintBinarySize(n))
		size += n
	}
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) < 0
	})

	b := make([]byte, 0, size)
	for _, key := range keys {
		entry := m.entries[key]
		b = append(b, key[:]...)
		b = append(b, encoding.UvarintMarshalBinary(uint64(len(entry)))...)
		b = append(b, entry...)
	}
	return b, nil
}

func (m *DB) UnmarshalBinary(b []byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Delete all entries first?

	for len(b) > 0 {
		key, err := encoding.ChainUnmarshalBinary(b)
		if err != nil {
			return err
		}
		b = b[encoding.ChainBinarySize(&key):]

		n, err := encoding.UvarintUnmarshalBinary(b)
		if err != nil {
			return err
		}
		b = b[encoding.UvarintBinarySize(n):]

		if uint64(len(b)) < n {
			return encoding.ErrNotEnoughData
		}
		entry := b[:n]
		b = b[n:]

		m.entries[key] = entry
	}
	return nil
}
