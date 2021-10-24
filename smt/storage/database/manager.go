package database

import (
	"bytes"
	"sync"

	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/smt/storage"
	"github.com/AccumulateNetwork/accumulated/smt/storage/badger"
	"github.com/AccumulateNetwork/accumulated/smt/storage/memory"
)

// Manager
// The Manager as implemented cannot be accessed concurrently over go routines
// ToDo: Add Mutex to make conccurency safe
type Manager struct {
	DB      storage.KeyValueDB                 // Underlying database implementation
	TXCache map[[storage.KeyLength]byte][]byte // TX Cache:  Holds pending tx for the db
}

// Equal
// Return true if the values in the manager are equal to the given manager
// mostly a testing function
func (m *Manager) Equal(m2 *Manager) bool {
	if len(m.TXCache) != len(m2.TXCache) {
		return false
	}
	for k, v := range m.TXCache {
		if !bytes.Equal(v, m2.TXCache[k]) {
			return false
		}
	}
	return true
}

// ClearCache
// Clear all the pending key values from the cache in the Databasae Manager
func (m *Manager) ClearCache() {
	for k := range m.TXCache { // Clear all elements from the cache
		delete(m.TXCache, k) //    golang optimizer will make loop good
	}
}

var AppIDMutex sync.Mutex // Creating new AppIDs has to be atomic

func (m *Manager) getInt64(keys ...interface{}) (int64, error) {
	b, err := m.Key(keys...).Get()
	if err != nil {
		return 0, err
	}
	v, _ := common.BytesInt64(b)
	return v, nil
}

// NewDBManager
// Create and initialize a new database manager
func NewDBManager(databaseTag, filename string) (*Manager, error) {
	manager := new(Manager)
	if err := manager.Init(databaseTag, filename); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *Manager) init() {
	m.TXCache = make(map[[storage.KeyLength]byte][]byte, 100) // Preallocate 100 slots
}

// Init
// Initialize the Manager with a specified underlying database. databaseTag
// can currently be either badger or memory.  The filename indicates where
// the database is persisted (ignored by memory).PendingChain
func (m *Manager) Init(databaseTag, filename string) error {
	m.init()
	switch databaseTag { //                              match with a supported databaseTag
	case "badger": //                                    Badger database indicated
		m.DB = new(badger.DB)                         // Create a badger struct
		if err := m.DB.InitDB(filename); err != nil { // Initialize it with the given filename
			return err
		}
	case "memory": //                                    memory database indicated
		m.DB = new(memory.DB)     //                     Allocate the structure
		_ = m.DB.InitDB(filename) //                     filename is ignored, but must allocate the underlying map
	}
	return nil
}

func (m *Manager) InitWithDB(db storage.KeyValueDB) {
	m.init()
	m.DB = db
}

// Close
// Do any cleanup required to close the manager
func (m *Manager) Close() error {
	return m.DB.Close()
}

func (m *Manager) Key(keys ...interface{}) KeyRef {
	return KeyRef{m, storage.ComputeKey(keys...)}
}

type KeyRef struct {
	M *Manager
	K storage.Key
}

// Put
// Add a []byte value into the underlying database
func (k KeyRef) Put(value []byte) error {
	return k.M.DB.Put(k.K, value)
}

// Get
// Retrieve []byte value from the underlying database. Note that this Get will
// first check the cache before it checks the DB.
// Returns a nil if not found, or on an error
func (k KeyRef) Get() ([]byte, error) {
	if v, ok := k.M.TXCache[k.K]; ok {
		return v, nil
	}
	if v, e := k.M.DB.Get(k.K); e != nil {
		return nil, e
	} else {
		return v, nil
	}

}

func (k KeyRef) PutBatch(value []byte) {
	k.M.TXCache[k.K] = value
}

// EndBatch
// Flush anything in the batch list to the database.
func (m *Manager) EndBatch() {
	if len(m.TXCache) == 0 { // If there is nothing to do, do nothing
		return
	}
	if err := m.DB.EndBatch(m.TXCache); err != nil {
		panic("batch failed to persist to the database")
	}
	m.resetCache()
}

// BeginBatch
// initializes the batch list to empty.  Note that we really only support one level of batch processing.
func (m *Manager) BeginBatch() {
	m.resetCache()
}

func (m *Manager) resetCache() {
	// The compiler optimizes away the loop. This is demonstrably faster and
	// less memory intensive than re-making the map. The savings in GC presure
	// scale proportionally with the batch size.
	for k := range m.TXCache {
		delete(m.TXCache, k)
	}
}
