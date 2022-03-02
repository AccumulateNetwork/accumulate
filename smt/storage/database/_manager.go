package database

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/badger"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

const debugKeys = false

// Manager
// The Manager as implemented cannot be accessed concurrently over go routines
type Manager struct {
	DB      storage.KeyValueStore  // Underlying database implementation
	txCache map[storage.Key][]byte // TX Cache:  Holds pending tx for the db
	cacheMu sync.RWMutex
}

// Equal
// Return true if the values in the manager are equal to the given manager
// mostly a testing function
func (m *Manager) Equal(m2 *Manager) bool {
	m.cacheMu.RLock()
	m2.cacheMu.RLock()
	defer m.cacheMu.RUnlock()
	defer m2.cacheMu.RUnlock()

	if len(m.txCache) != len(m2.txCache) {
		return false
	}
	for k, v := range m.txCache {
		if !bytes.Equal(v, m2.txCache[k]) {
			return false
		}
	}
	return true
}

// ClearCache
// Clear all the pending key values from the cache in the Databasae Manager
func (m *Manager) ClearCache() {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	m.resetCache()
}

var AppIDMutex sync.Mutex // Creating new AppIDs has to be atomic

// NewDBManager
// Create and initialize a new database manager
func NewDBManager(databaseTag, filename string, logger storage.Logger) (*Manager, error) {
	manager := new(Manager)
	if err := manager.Init(databaseTag, filename, logger); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *Manager) init() {
	m.txCache = make(map[storage.Key][]byte, 100) // Preallocate 100 slots
}

// Init
// Initialize the Manager with a specified underlying database. databaseTag
// can currently be either badger or memory.  The filename indicates where
// the database is persisted (ignored by memory).PendingChain
func (m *Manager) Init(databaseTag, filename string, logger storage.Logger) error {
	m.init()
	switch databaseTag { //                              match with a supported databaseTag
	case "badger": //                                    Badger database indicated
		m.DB = new(badger.DB)                                 // Create a badger struct
		if err := m.DB.InitDB(filename, logger); err != nil { // Initialize it with the given filename
			return err
		}
	case "memory": //                                    memory database indicated
		m.DB = new(memory.DB)             //                     Allocate the structure
		_ = m.DB.InitDB(filename, logger) //                     filename is ignored, but must allocate the underlying map
	default:
		return errors.New("undefined database type '" + databaseTag + "'")
	}
	return nil
}

func (m *Manager) InitWithDB(db storage.KeyValueStore) {
	m.init()
	m.DB = db
}

// Close
// Do any cleanup required to close the manager
func (m *Manager) Close() error {
	return m.DB.Close()
}

// Put
// Add a []byte value into the underlying database
func (m *Manager) Put(key storage.Key, value []byte) error {
	return m.DB.Put(key, value)
}

// Get
// Retrieve []byte value from the underlying database. Note that this Get will
// first check the cache before it checks the DB.
// Returns storage.ErrNotFound if not found.
func (m *Manager) Get(key storage.Key) ([]byte, error) {
	if debugKeys {
		fmt.Printf("Get %v\n", key)
	}
	m.cacheMu.RLock()
	defer m.cacheMu.RUnlock()
	if v, ok := m.txCache[key]; ok {
		// Return a copy. Otherwise the caller could change it, and that would
		// change what's in the cache.
		u := make([]byte, len(v))
		copy(u, v)
		return u, nil
	}
	// Assume the DB implementation correctly implements the interface
	return m.DB.Get(key)
}

func (m *Manager) PutBatch(key storage.Key, value []byte) {
	if debugKeys {
		fmt.Printf("Put %v => %X\n", key, value)
	}
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	// Save a copy. Otherwise the caller could change it, and that would change
	// what's in the cache.
	u := make([]byte, len(value))
	copy(u, value)
	m.txCache[key] = u
}

// EndBatch
// Flush anything in the batch list to the database.
func (m *Manager) EndBatch() {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	if len(m.txCache) == 0 { // If there is nothing to do, do nothing
		return
	}
	if err := m.DB.EndBatch(m.txCache); err != nil {
		panic(fmt.Errorf("batch failed to persist to the database: %v", err))
	}
	m.resetCache()
}

// BeginBatch
// initializes the batch list to empty.  Note that we really only support one level of batch processing.
func (m *Manager) BeginBatch() {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	m.resetCache()
}

func (m *Manager) resetCache() {
	// The compiler optimizes away the loop. This is demonstrably faster and
	// less memory intensive than re-making the map. The savings in GC presure
	// scale proportionally with the batch size.
	for k := range m.txCache {
		delete(m.txCache, k)
	}
}
