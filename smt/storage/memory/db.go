package memory

import (
	"bytes"
	"encoding/json"
	"sort"
	"sync"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// DB
// Implements a key value store in memory.  Very basic, assumes no initial
// state for the database That must be handled by the caller, but see the
// notes on InitDB for future improvements.
type DB struct {
	DBOpen  atomicBool
	entries map[storage.Key][]byte
	mutex   sync.Mutex
	logger  storage.Logger
}

func NewDB() *DB {
	db := new(DB)
	_ = db.InitDB("", nil)
	return db
}

var _ storage.KeyValueStore = (*DB)(nil)

// Ready
// Returns true if the database is open and ready to accept reads/writes
func (m *DB) Ready() bool {
	return m.DBOpen.Load()
}

// EndBatch
// Takes all the key value pairs collected in a cache of mapped values and
// adds them to the database.  The assumption here is that the order in which
// the cache is applied to a key value store does not matter.
func (m *DB) EndBatch(txCache map[storage.Key][]byte) error {
	m.mutex.Lock()
	if !m.Ready() {
		return storage.ErrNotOpen
	}
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
	if !m.Ready() {
		return nil
	}
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
	if !m.Ready() {
		return nil
	}

	ex := make(map[storage.Key][]byte, len(m.entries))
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

// InitDB
// Initialize a database; a memory database has no existing state, so this
// routine does nothing with the filename.  We could save and restore
// a memory database from a file in the future.
//
// An existing memory database will be cleared by calling InitDB
func (m *DB) InitDB(_ string, logger storage.Logger) error {
	m.logger = logger

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
	m.DBOpen.Store(true)
	return nil
}

// Get
// Returns the value for a key from the database
func (m *DB) Get(key storage.Key) (value []byte, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.Ready() {
		return nil, storage.ErrNotOpen
	}

	v, ok := m.entries[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return append([]byte{}, v...), nil
}

// Put
// Takes a key and a value, and puts the pair into the database
func (m *DB) Put(key storage.Key, value []byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if !m.Ready() {
		return storage.ErrNotOpen
	}
	m.entries[key] = append([]byte{}, value...)
	return nil
}

func (db *DB) Begin(writable bool) storage.KeyValueTxn {
	b := &Batch{
		db:     db,
		mu:     new(sync.RWMutex),
		values: map[storage.Key][]byte{},
	}
	if db.logger == nil {
		return b
	}
	return &storage.DebugBatch{Batch: b, Logger: db.logger}
}

type jsonDB []jsonEntry

type jsonEntry struct {
	Key   [32]byte
	Value []byte
}

func (m *DB) MarshalJSON() ([]byte, error) {
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

	jdb := make(jsonDB, 0, size)
	for _, key := range keys {
		entry := m.entries[key]
		jdb = append(jdb, jsonEntry{key, entry})
	}
	return json.Marshal(jdb)
}

func (m *DB) UnmarshalJSON(b []byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var jdb jsonDB
	err := json.Unmarshal(b, &jdb)
	if err != nil {
		return err
	}

	// Delete all entries first?

	for _, e := range jdb {
		m.entries[storage.Key(e.Key)] = e.Value
	}
	return nil
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
