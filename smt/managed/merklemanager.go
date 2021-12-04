package managed

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"

	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/database"
)

type MerkleManager struct {
	key       []interface{}     // ChainID (some operations require this)
	Manager   *database.Manager // AppID based Manager
	MS        *MerkleState      // MerkleState managed by MerkleManager
	MarkPower int64             // log2 of the MarkFreq
	MarkFreq  int64             // The count between Marks
	MarkMask  int64             // The binary mask to detect Mark boundaries
}

func (m *MerkleManager) dbKey(key ...interface{}) database.KeyRef {
	return m.Manager.Key(append(m.key, key...)...)
}

// AddHash
// Add a Hash to the Chain controlled by the ChainManager
func (m *MerkleManager) AddHash(hash Hash) {
	hash = hash.Copy() // Just to make sure hash doesn't get changed

	// Keep the index of every element added to the Merkle Tree, but only of the first instance
	i, err := m.GetElementIndex(hash)
	_ = i
	if errors.Is(err, storage.ErrNotFound) { // So only if the hash is not yet added to the Merkle Tree
		m.dbKey("ElementIndex", hash).PutBatch(common.Int64Bytes(m.MS.Count)) // Keep its index
	} else if err != nil {
		panic(err) // TODO Panics are bad, but I don't want to change the signature now
	}

	m.dbKey("Element", m.MS.Count).PutBatch(hash)

	switch m.MS.Count & m.MarkMask { // Mask to the bits counting up to a mark.
	case m.MarkMask: // This is the case just before rolling into a Mark (all bits set. +1 more will be zero)

		MSState, err := m.MS.Marshal() // Get the current state
		if err != nil {                // Panic if we cannot, because that should never happen.
			panic(fmt.Sprintf("could not marshal MerkleState: %v", err)) //
		}

		count := m.MS.Count
		m.dbKey("States", count).PutBatch(MSState)   //     Save Merkle State at n*MarkFreq-1
		m.dbKey("NextElement", count).PutBatch(hash) //     Save Hash added at n*MarkFreq-1

		m.MS.AddToMerkleTree(hash) //                                  Add the hash to the Merkle Tree

		// TODO This overwrites the states entry. Either it should write to
		// `m.MS.Count` instead of `count` or the previous write should be
		// removed.

		state, err := m.MS.Marshal()             // Create the marshaled Merkle State
		m.dbKey("States", count).PutBatch(state) // Save Merkle State at n*MarkFreq
		m.MS.HashList = m.MS.HashList[:0]        // We need to clear the HashList first

	default: // This is the case of not rolling into a mark; Just add the hash to the Merkle Tree
		m.MS.AddToMerkleTree(hash) //                                      Always add to the merkle tree
	}

	if err := m.WriteChainHead(m.key...); err != nil {
		panic(fmt.Sprintf("could not marshal MerkleState: %v", err))
	}
}

// GetElementIndex
// Get an Element of a Merkle Tree from the database
func (m *MerkleManager) GetElementIndex(hash []byte) (i int64, err error) {
	data, err := m.dbKey("ElementIndex", hash).Get()
	if err != nil {
		return 0, err
	}
	i, _ = common.BytesInt64(data)
	return i, nil
}

// SetKey
// adds the chainID to the MerkleManager for storing some chain specific key
// value pairs where the key is an index.  Before any hash is added to a
// MerkleState, the ChainID should be set
func (m *MerkleManager) SetKey(key ...interface{}) (err error) {
	m.key = key
	m.MS, err = m.ReadChainHead(key...)
	return err
}

// WriteChainHead
// Save the current MerkleState as the head of the chain.  This does not flush
// the database, because all chains in an application need have their head of
// chain updated as one atomic transaction to prevent messing up the database
func (m *MerkleManager) WriteChainHead(key ...interface{}) error {
	state, err := m.MS.Marshal()
	if err != nil {
		return err
	}
	m.Manager.Key(append(key, "Head")...).PutBatch(state)
	return nil
}

// ReadChainHead
// Retrieve the current MerkleState from the given database, and set
// that state as the MerkleState of this MerkleManager.  Note that the cache
// will be cleared.
func (m *MerkleManager) ReadChainHead(key ...interface{}) (ms *MerkleState, err error) {
	ms = new(MerkleState)
	ms.HashFunction = m.MS.HashFunction
	state, e := m.Manager.Key(append(key, "Head")...).Get() //   Get the state for the Merkle Tree
	if e == nil {                                           //   If the State exists
		if err := ms.UnMarshal(state); err != nil { //     Set that as our state
			return nil, fmt.Errorf("database is corrupt; failed to unmarshal %v",
				key) // Blow up of the database is bad
		}
	}
	return ms, nil
}

// Equal
// Compares the MerkleManager to the given MerkleManager and returns false if
// the fields in the MerkleManager are different from m2
func (m *MerkleManager) Equal(m2 *MerkleManager) bool {
	if !m.Manager.Equal(m2.Manager) {
		return false
	}
	if !m.MS.Equal(m2.MS) {
		return false
	}
	if m.MarkPower != m2.MarkPower {
		return false
	}
	if m.MarkFreq != m2.MarkFreq {
		return false
	}
	if m.MarkMask != m2.MarkMask {
		return false
	}
	return true
}

// NewMerkleManager
// Create a new MerkleManager given a MainChain.Manager and markPower.
// The MainChain.Manager handles the persistence for the Merkle Tree under management
// The markPower is the log 2 frequency used to collect states in the Merkle Tree
func NewMerkleManager(
	DBManager *database.Manager, //      database that can be shared with other MerkleManager instances
	markPower int64) (*MerkleManager, error) { // log 2 of the frequency of creating marks in the Merkle Tree

	mm := new(MerkleManager)
	if err := mm.init(DBManager, markPower); err != nil {
		return nil, err
	}

	return mm, nil
}

// ManageChain returns a copy of the manager with the key set to the given
// value.
func (m *MerkleManager) ManageChain(key ...interface{}) (*MerkleManager, error) {
	n := new(MerkleManager)
	err := n.init(m.Manager, m.MarkPower)
	if err != nil {
		return nil, err
	}

	err = n.SetKey(key...)
	if err != nil {
		return nil, err
	}

	return n, nil
}

// GetElementCount
// Return number of elements in the Merkle Tree managed by this MerkleManager
func (m *MerkleManager) GetElementCount() (elementCount int64) {
	return m.MS.Count
}

// init
// Create a Merkle Tree manager to collect hashes and build a Merkle Tree and a
// database behind it to validate hashes and create receipts.
//
// This is an internal routine; calling init outside of constructing the first
// reference to the MerkleManager doesn't make much sense.
func (m *MerkleManager) init(DBManager *database.Manager, markPower int64) (err error) {

	if markPower >= 20 { // 2^20 is 1,048,576 and is too big for the collection of elements in memory
		return fmt.Errorf("A power %d is greater than 2^29, and is unreasonable", markPower)
	}
	m.Manager = DBManager // Manager for writing the Merkle states
	if m.MS == nil {      //                                    Allocate an MS if we don't have one
		m.MS = new(MerkleState) //
		m.MS.InitSha256()       //
	}
	m.MS, err = m.ReadChainHead(m.key...) //                  Set the MerkleState
	if err != nil {
		return err
	}
	m.MarkPower = markPower                             // # levels in Merkle Tree to be indexed
	m.MarkFreq = int64(math.Pow(2, float64(markPower))) // The number of elements between indexes
	m.MarkMask = m.MarkFreq - 1                         // Mask to index of next mark (0 if at a mark)

	return nil
}

// GetState
// Get a MerkleState for a given index, i.e. the state stored for the given
// index.  Note that not every element in the Merkle Tree has a stored state;
// states are stored at the frequency indicated by the Mark Power.
//
// If no state exists for the given element, GetState returns nil
func (m *MerkleManager) GetState(element int64) *MerkleState {
	if element == 0 {
		return new(MerkleState)
	}

	data, e := m.dbKey("States", element).Get() // Get the data at this height
	if e != nil {                               // If nil, there is no state saved
		return nil //                                                   return nil, as no state exists
	}
	ms := new(MerkleState)                     //                                 Get a fresh new MerkleState
	if err := ms.UnMarshal(data); err != nil { //                                 set it the MerkleState
		panic(fmt.Sprintf("corrupted database; invalid state. error: %v", err)) // panic if the database is corrupted.
	}
	return ms // return it
}

// Get the nth leaf node
func (m *MerkleManager) Get(element int64) (Hash, error) {
	data, err := m.dbKey("Element", element).Get()
	if err != nil {
		return nil, err
	}

	return Hash(data).Copy(), nil
}

// GetNext
// Get the next hash to be added to a state at this height
func (m *MerkleManager) GetNext(element int64) (hash Hash) {
	data, err := m.dbKey("NextElement", element).Get()
	if err != nil {
		return nil
	}
	return Hash(data).Copy()
}

// AddHashString
// Often instead of a hash, we have a hex string, but that's okay too.
func (m *MerkleManager) AddHashString(hash string) {
	if h, err := hex.DecodeString(hash); err != nil { //                             Convert to a binary slice
		panic(fmt.Sprintf("failed to decode a hash %s with error %v", hash, err)) // If this fails, panic; no recovery
	} else {
		m.AddHash(h) // Add the hash
	}
}
