package database

import (
	"crypto/sha256"
	"fmt"
	"math"
	"sync"

	"github.com/AccumulateNetwork/SMT/common"

	"github.com/AccumulateNetwork/SMT/storage"
	"github.com/AccumulateNetwork/SMT/storage/badger"
	"github.com/AccumulateNetwork/SMT/storage/memory"
)

type Manager struct {
	DB      storage.KeyValueDB // Underlying database implementation
	Salt    []byte             // Used to share a database with other applications
	Buckets map[string]byte    // one byte to indicate a bucket
	Labels  map[string]byte    // one byte to indicate a label
	TXList  TXList             // Transaction List
}

// Copy
// Create a copy of the manager that uses the given salt.  Salts allow multiple Merkle Trees
// to be managed within the same database.  All keys for a particular merkle tree are salted
// with their particular salts.  Note that maps are pointers, so Buckets and Labels
// are shared over copies of the Manager
func (m Manager) Copy(Salt []byte) *Manager {
	m.SetSalt(Salt) // Set the Salt
	m.TXList.Init() // Make the TXList independent of the original TXList
	return &m       // Return a pointer
}

var SaltMutex sync.Mutex // Creating new Salts has to be atomic

// SetSalt
// Set a Salt (possibly replacing an existing salt) to this manager. This allows
// the use of the same database to be shared by applications/uses without overlap
//
// Note that Salts are managed in the database outside of salts.  First
// the salt must be cleared to access the Salt bucket and its labels.  Then
// the salt can be set so the appropriate buckets can be accessed under
// the salt
func (m Manager) SetSalt(salt []byte) {

	m.Salt = m.Salt[:0]                                 // clear the salt to get salt data
	saltIndex := m.GetInt64("Salt", "Salt2Index", salt) // Sort index for existing salt
	if saltIndex < 0 {                                  // A index < 0 => salt does not exist
		SaltMutex.Lock()                          //       Lock salt creation so creation is atomic
		count := m.GetInt64("Salt", "", []byte{}) //       Get the count of existing salts
		if count == 0 {                           //       The 0 index isn't used.
			count++ //                                     So if zero, increment the count
		}
		// Note that we don't batch updating of salts.  the batch processing does not need
		// to be flushed however.
		_ = m.Put("Salt", "Index2Salt", common.Int64Bytes(count), salt) // Index to Salt
		_ = m.PutInt64("Salt", "Salt2Index", salt, count)               // Salt to Index
		_ = m.PutInt64("Salt", "", []byte{}, count+1)                   // increment the count in the database.
		SaltMutex.Unlock()
	}
	m.Salt = append(m.Salt[:0], salt...) // copy the given salt over the current salt
}

// CurrentSalt
// Return the current salt used by the MerkleManager
func (m *Manager) CurrentSalt() (salt []byte) {
	if salt == nil {
		return nil
	}
	salt = append(salt, m.Salt...)
	return salt
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

// Init
// Initialize the Manager with a specified underlying database. databaseTag
// can currently be either badger or memory.  The filename indicates where
// the database is persisted (ignored by memory).PendingChain
func (m *Manager) Init(databaseTag, filename string) error {
	// Set up Buckets for use by the Stateful Merkle Trees
	m.Buckets = make(map[string]byte) // Buckets hold sets of key value pairs
	m.Labels = make(map[string]byte)  // Labels hold subsets of key value pairs within a bucket

	m.AddBucket("Salt")      //                          Maintains the maximum index count   /count
	m.AddLabel("Index2Salt") //                          Given a salt index returns a salt   index/salt
	m.AddLabel("Salt2Index") //                          Given a salt, provides an index     salt/index

	m.AddBucket("ElementIndex") //                       element hash / element index
	m.AddBucket("States")       //                       element index / merkle state
	m.AddBucket("NextElement")  //                       element index / next element to be added to merkle tree
	m.AddBucket("Element")      //                       count of elements in the merkle tree
	m.AddBucket("BPT")          //                       Binary Patricia Tree Byte Blocks (blocks of BPT nodes)
	m.AddLabel("Root")          //                       The Root node of the BPT

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

// GetCount
// The number of elements as recorded in the Database.  As a side effect, the
// any pending transactions in the batch process will be flushed to the database
func (m *Manager) GetCount() int64 {
	m.EndBatch()                                               // Flush any pending transactions to the database.
	data := m.DB.Get(m.GetKey("Element", "", []byte("Count"))) // Look and see if there is any element count recorded
	if data == nil {                                           // If not, nothing is there
		return 0 //                                                 return zero
	}
	v, _ := common.BytesInt64(data) //                           Return the recorded count
	return v
}

// Close
// Do any cleanup required to close the manager
func (m *Manager) Close() {
	if err := m.DB.Close(); err != nil {
		panic(err)
	}
}

// AddBucket
// Add a bucket to be used in the database.  Initializing a database requires
// that buckets and labels be added in the correct order.
// Returns true if the bucket is added
// Returns false if the bucket exists
// Panics if the bucket limit is reached
func (m *Manager) AddBucket(bucket string) bool {
	if _, ok := m.Buckets[bucket]; ok { // If the bucket exists, return false
		return false //                    this prevents "changing" the bucket index
	}
	idx := len(m.Buckets) + 1 //          Calculate the next index for the new bucket.  Indexes start at 1 (no zero)
	if idx > 255 {            //          We use a byte for the index, so can't be greater than 255
		panic("too many buckets") //       If we have no more room for buckets, pannic
	}
	m.Buckets[bucket] = byte(idx) //      Create the bucket
	return true                   //      Return success on adding bucket
}

// AddLabel
// Add a Label to be used in the database.  Initializing a database requires
// that buckets and labels be added in the correct order.
func (m *Manager) AddLabel(label string) bool {
	if _, ok := m.Labels[label]; ok { // If the Label exists, return false
		return false //                  this prevents changing a label index
	}
	idx := len(m.Labels) + 1 //          Compute next label index.  Indexes start at 1
	if idx > 255 {           //          If no room for a new label, panic
		panic("too many labels")
	}
	m.Labels[label] = byte(idx) //       Create the new label
	return true                 //       Return success on adding the label
}

// GetKey
// Given a Bucket Name, a Label name, and a key, GetKey returns a single
// key to be used in a key/value database.
// Note that the use of the salt means parallel managers for different
// Merkle Trees can still share a database.
func (m *Manager) GetKey(Bucket, Label string, key []byte) (DBKey [storage.KeyLength]byte) {
	var ok bool
	if _, ok = m.Buckets[Bucket]; !ok { //                                Is the bucket defined?
		panic(fmt.Sprintf("bucket %s undefined or invalid", Bucket)) //      Panic if not
	}
	if _, ok = m.Labels[Label]; len(Label) > 0 && !ok { //                If a label is specified, is it defined?
		panic(fmt.Sprintf("label %s undefined or invalid", Label)) //        Panic if not.
	}
	DBKey = sha256.Sum256(append(key, m.Salt...)) // To get a fixed length key, hash the key and salt together
	//                                               A hash is very secure so losing two bytes won't hurt anything
	DBKey[0] = m.Buckets[Bucket] //                  Replace the first byte with the bucket index
	DBKey[1] = 0                 //                  Assume no label (0 -- means no label
	if len(Label) > 0 {          //                  But if a label is specified, (zero labels not allowed) then
		DBKey[1] = m.Labels[Label] //                   set the label's byte value.
	}

	return DBKey
}

// Put
// Put a []byte value into the underlying database
func (m *Manager) Put(Bucket, Label string, key []byte, value []byte) (err error) {
	defer func() { //                    Catch any errors
		if r := recover(); r != nil { // and return an nice error message
			err = fmt.Errorf("%v", r)
		}
	}()
	k := m.GetKey(Bucket, Label, key) // Calculate the key
	err = m.DB.Put(k, value)          // put the key value in the database
	return err                        // return any reported errors
}

// PutString
// Put a String value into the underlying database
func (m *Manager) PutString(Bucket, Label string, key []byte, value string) error {
	return m.Put(Bucket, Label, key, []byte(value)) // Do the conversion of strings to bytes
}

// PutInt64
// Put a int64 value into the underlying database
func (m *Manager) PutInt64(Bucket, Label string, key []byte, value int64) error {
	return m.Put(Bucket, Label, key, common.Int64Bytes(value)) // Do the conversion of int64 to bytes
}

// Get
// Get a []byte value from the underlying database.  Returns a nil if not found,
// or on an error
func (m *Manager) Get(Bucket, Label string, key []byte) (value []byte) {
	m.EndBatch()                                  // Flush any pending writes to the database, so we can get a value
	return m.DB.Get(m.GetKey(Bucket, Label, key)) // that might not quite yet have been written to the database
}

// GetString
// Get a string value from the underlying database.  Returns a nil if not
// found, or on an error
func (m *Manager) GetString(Bucket, Label string, key []byte) (value string) {
	return string(m.DB.Get(m.GetKey(Bucket, Label, key))) // Do the bytes to string conversion
}

// GetInt64
// Get a string value from the underlying database.  Returns a MinInt64
// if not found, or on an error
func (m *Manager) GetInt64(Bucket, Label string, key []byte) (value int64) {
	bv := m.DB.Get(m.GetKey(Bucket, Label, key))
	if bv == nil {
		return math.MinInt64
	}
	v, _ := common.BytesInt64(bv) // Do the bytes to int64 conversion
	return v
}

// GetIndex
// Return the int64 value tied to the element hash in the ElementIndex bucket
func (m *Manager) GetIndex(element []byte) int64 {
	data := m.Get("ElementIndex", "", element) // Look for the first index of a hash that might exist
	if data == nil {                           // in the merkle tree.  Note that nil means it does not yet exist
		return -1 //                              in which case, return an invalid index (-1)
	}
	v, _ := common.BytesInt64(data) //           Convert the index to an int64
	return v
}

// PutBatch
// put the write of a key value into the pending batch.  These will all be
// written to the database together.
func (m *Manager) PutBatch(Bucket, Label string, key []byte, value []byte) error {
	theKey := m.GetKey(Bucket, Label, key) // Put a key value pair into the batch list
	return m.TXList.Put(theKey, value)     // Return any error that might occur
}

// EndBatch
// Flush anything in the batch list to the database.
func (m *Manager) EndBatch() {
	if len(m.TXList.List) == 0 { // If there is nothing to do, do nothing
		return
	}
	if err := m.DB.PutBatch(m.TXList.List); err != nil {
		panic("batch failed to persist to the database")
	}
	m.TXList.List = m.TXList.List[:0] // Reset the List to allow it to be reused
}

// BeginBatch
// initializes the batch list to empty.  Note that we really only support one level of batch processing.
func (m *Manager) BeginBatch() {
	m.TXList.List = m.TXList.List[:0]
}
