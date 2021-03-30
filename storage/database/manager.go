package database

import (
	"crypto/sha256"
	"fmt"

	"github.com/AccumulateNetwork/SMT/storage"
	"github.com/AccumulateNetwork/SMT/storage/badger"
	"github.com/AccumulateNetwork/SMT/storage/memory"
)

type Manager struct {
	DB      storage.KeyValueDB // Underlying database implementation
	Buckets map[string]byte    // two bytes indicating a bucket
	TXList  TXList             // Transaction List
}

// Init
// Initialize the Manager with a specified underlying database
func (m *Manager) Init(database, filename string) error {
	// Set up Buckets for use by the Stateful Merkle Trees
	m.Buckets = make(map[string]byte)
	//  Bucket and Label use by Stateful Merkle Trees
	//  Bucket             --          key/value
	//  ElementIndex       -- element hash / element index
	//  States             -- element index / merkle state
	//  NextElement        -- element index / next element
	//  Element            -- "Count" / highest element

	// element hash / element index
	m.AddBucket("ElementIndex")
	// element index / element state
	m.AddBucket("States")
	// element index / next element
	m.AddBucket("NextElement")
	// count of element in the database
	m.AddBucket("Element")

	// match with a supported database
	switch database {
	case "badger": // Badger
		m.DB = new(badger.DB)
		if err := m.DB.InitDB(filename); err != nil {
			return err
		}
	case "memory": // DB databases can't fail
		m.DB = new(memory.DB)
		_ = m.DB.InitDB(filename)
	}
	return nil
}

// Close
// Do any cleanup required to close the manager
func (m *Manager) Close() {
	m.DB.Close()
}

// AddBucket
// Add a bucket to be used in the database.  Initializing a database requires
// that buckets and labels be added in the correct order.
func (m *Manager) AddBucket(bucket string) {
	if _, ok := m.Buckets[bucket]; ok {
		panic(fmt.Sprintf("the bucket '%s' is already defined", bucket))
	}
	idx := len(m.Buckets) + 1
	if idx > 255 {
		panic("too many buckets")
	}
	m.Buckets[bucket] = byte(idx)
}

// GetKey
// Given a Bucket Name, a Label name, and a key, GetKey returns a single
// key to be used in a key/value database
func (m *Manager) GetKey(Bucket string, key []byte) (DBKey [storage.KeyLength]byte) {
	var ok bool
	if _, ok = m.Buckets[Bucket]; !ok { //                          Make sure we have a bucket
		panic(fmt.Sprintf("bucket %s undefined or invalid", Bucket)) //    and panic if we don't
	}
	keyHash := sha256.Sum256(key)      // Use the Hash of the given key
	copy(DBKey[:], keyHash[:])         //
	DBKey[0] = byte(m.Buckets[Bucket]) // Replace the first byte with the bucket index
	return DBKey
}

// Put
// Put a []byte value into the underlying database
func (m *Manager) Put(Bucket string, key []byte, value []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	k := m.GetKey(Bucket, key)
	err = m.DB.Put(k, value)
	return err
}

// PutString
// Put a String value into the underlying database
func (m *Manager) PutString(Bucket string, key []byte, value string) error {
	return m.Put(Bucket, key, []byte(value))
}

// PutInt64
// Put a int64 value into the underlying database
func (m *Manager) PutInt64(Bucket string, key []byte, value int64) error {
	return m.Put(Bucket, key, storage.Int64Bytes(value))
}

// Get
// Get a []byte value from the underlying database.  Returns a nil if not found, or on an error
func (m *Manager) Get(Bucket string, key []byte) (value []byte) {
	return m.DB.Get(m.GetKey(Bucket, key))
}

// GetString
// Get a string value from the underlying database.  Returns a nil if not found, or on an error
func (m *Manager) GetString(Bucket string, key []byte) (value string) {
	return string(m.DB.Get(m.GetKey(Bucket, key)))
}

// GetInt64
// Get a string value from the underlying database.  Returns a nil if not found, or on an error
func (m *Manager) GetInt64(Bucket string, key []byte) (value int64) {
	v, _ := storage.BytesInt64(m.DB.Get(m.GetKey(Bucket, key)))
	return v
}

// GetIndex
// Return the int64 value tied to the element hash in the ElementIndex bucket
func (m *Manager) GetIndex(element []byte) int64 {
	data := m.Get("ElementIndex", element)
	if data == nil {
		return -1
	}
	v, _ := storage.BytesInt64(data)
	return v
}

func (m *Manager) PutBatch(Bucket string, key []byte, value []byte) error {
	theKey := m.GetKey(Bucket, key)
	return m.TXList.Put(theKey, value)
}

func (m *Manager) EndBatch() {
	if err := m.DB.PutBatch(m.TXList.List); err != nil {
		panic("batch failed to persist to the database")
	}
	m.TXList.List = m.TXList.List[:0] // Reset the List to allow it to be reused
}

func (m *Manager) BeginBatch() {
	m.TXList.List = m.TXList.List[:0]
}
