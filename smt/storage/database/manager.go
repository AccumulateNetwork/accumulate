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
	AppID   []byte                             // Allows apps to share a database
	TXCache map[[storage.KeyLength]byte][]byte // TX Cache:  Holds pending tx for the db
}

// Equal
// Return true if the values in the manager are equal to the given manager
// mostly a testing function
func (m *Manager) Equal(m2 *Manager) bool {
	if !bytes.Equal(m.AppID, m2.AppID) {
		return false
	}
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

// ManageAppID
// Create a manager to manage the given AppID.  The AppID salt allows
// multiple Merkle Trees to be managed within the same database.  All keys
// for a particular merkle tree are salted with their particular AppID.
func (m Manager) ManageAppID(AppID []byte) *Manager {
	m.SetAppID(AppID) // Set the AppID
	return &m         // Return a pointer
}

var AppIDMutex sync.Mutex // Creating new AppIDs has to be atomic

// SetAppID
// Set a AppID (possibly replacing an existing AppID) to this manager. This
// allows the use of the same database to be shared by applications/uses
// without overlap
//
// Note the use of an AppID is managed in the database outside of manager.
// First the AppID must be cleared to access the AppID tracking key/values,
// i.e. the AppID bucket and its labels.  Then the AppID can be set so the
// appropriate buckets can be accessed under the AppID
func (m *Manager) SetAppID(appID []byte) {
	if appID == nil { // If the appID is nil, then this manager is going
		m.AppID = nil //  to be used to write to the root level.  No need to
		return        //  register it.
	}

	m.AppID = nil                                         // clear the appID to get AppID data
	appIDIdx := m.getInt64("AppID", "AppID2Index", appID) // Sort index for existing AppID
	if appIDIdx < 0 {                                     // A index < 0 => AppID does not exist
		AppIDMutex.Lock()          //       Lock AppID creation so creation is atomic
		count := m.getInt64(appID) //       Get the count of existing AppIDs
		if count == 0 {            //       The 0 index isn't used.
			count++ //                                      So if zero, increment the count
		}
		// Note that we don't batch updating of appIDs.  the batch processing does not need
		// to be flushed however.
		_ = m.Key("AppID", "Index2AppID", count).Put(appID)                    // Index to AppID
		_ = m.Key("AppID", "AppID2Index", appID).Put(common.Int64Bytes(count)) // AppID to Index
		_ = m.Key("AppID").Put(common.Int64Bytes(count + 1))                   // increment the count in the database.
		AppIDMutex.Unlock()
	}
	m.AppID = make([]byte, len(appID))
	copy(m.AppID, appID) // copy the given appID over the current AppID
}

func (m *Manager) getInt64(keys ...interface{}) int64 {
	b := m.Key(keys...).Get()
	if b == nil {
		return 0
	}
	v, _ := common.BytesInt64(b)
	return v
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
// Put a []byte value into the underlying database
func (k KeyRef) Put(value []byte) error {
	return k.M.DB.Put(k.K, value)
}

// Get
// Get a []byte value from the underlying database. Note that this Get will
// first check the cache before it checks the DB.
// Returns a nil if not found, or on an error
func (k KeyRef) Get() []byte {
	if v, ok := k.M.TXCache[k.K]; ok {
		return v
	}
	return k.M.DB.Get(k.K)
}

func (k KeyRef) PutBatch(value []byte) {
	k.M.TXCache[k.K] = value
}

// GetIndex
// Return the int64 value tied to the element hash in the ElementIndex bucket
func (m *Manager) GetIndex(element []byte) int64 {
	data := m.Key("ElementIndex", element).Get() // Look for the first index of a hash that might exist
	if data == nil {                             // in the merkle tree.  Note that nil means it does not yet exist
		return -1 //                              in which case, return an invalid index (-1)
	}
	v, _ := common.BytesInt64(data) //           Convert the index to an int64
	return v
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
