package managed

import (
	"encoding/hex"
	"fmt"
	"math"

	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/smt/storage/database"
)

type MerkleManager struct {
	cid       []byte            // ChainID (some operations require this)
	Manager   *database.Manager // AppID based Manager
	MS        *MerkleState      // MerkleState managed by MerkleManager
	MarkPower int64             // log2 of the MarkFreq
	MarkFreq  int64             // The count between Marks
	MarkMask  int64             // The binary mask to detect Mark boundaries
}

// AddHash
// Add a Hash to the Chain controlled by the ChainManager
func (m *MerkleManager) AddHash(hash Hash) {
	hash = hash.Copy() // Just to make sure hash doesn't get changed

	// Keep the index of every element added to the Merkle Tree, but only of the first instance
	i, err := m.GetElementIndex(hash)
	_ = i
	if err == nil { // So only if the hash is not yet added to the Merkle Tree
		m.Manager.Key(m.cid, "ElementIndex", hash).PutBatch(common.Int64Bytes(m.MS.Count)) // Keep its index
		m.Manager.Key(m.cid, "Element", m.MS.Count).PutBatch(hash)
	}

	switch m.MS.Count & m.MarkMask { // Mask to the bits counting up to a mark.
	case m.MarkMask: // This is the case just before rolling into a Mark (all bits set. +1 more will be zero)

		MSCount := common.Int64Bytes(m.MS.Count) // Get the current index as a varInt
		MSState, err := m.MS.Marshal()           // Get the current state
		if err != nil {                          // Panic if we cannot, because that should never happen.
			panic(fmt.Sprintf("could not marshal MerkleState: %v", err)) //
		}

		m.Manager.Key(m.cid, "States", MSCount).PutBatch(MSState)   //     Save Merkle State at n*MarkFreq-1
		m.Manager.Key(m.cid, "NextElement", MSCount).PutBatch(hash) //     Save Hash added at n*MarkFreq-1

		m.MS.AddToMerkleTree(hash) //                                  Add the hash to the Merkle Tree

		state, err := m.MS.Marshal()                            // Create the marshaled Merkle State
		m.Manager.Key(m.cid, "States", MSCount).PutBatch(state) // Save Merkle State at n*MarkFreq
		m.MS.HashList = m.MS.HashList[:0]                       // We need to clear the HashList first

	default: // This is the case of not rolling into a mark; Just add the hash to the Merkle Tree
		m.MS.AddToMerkleTree(hash) //                                      Always add to the merkle tree
	}

	if err := m.WriteChainHead(m.cid); err != nil {
		panic(fmt.Sprintf("could not marshal MerkleState: %v", err))
	}
}

// GetElementIndex
// Get an Element of a Merkle Tree from the database
func (m *MerkleManager) GetElementIndex(hash []byte) (i int64, err error) {
	if len(hash) != 32 {
		return 0, fmt.Errorf("invalid length %d for hash (should be 32)", len(hash))
	}
	data, e := m.Manager.Key(m.cid, "ElementIndex", hash).Get()
	if e != nil {
		return 0, err
	}
	i, _ = common.BytesInt64(data)
	return i, nil
}

// SetChainID
// adds the chainID to the MerkleManager for storing some chain specific key
// value pairs where the key is an index.  Before any hash is added to a
// MerkleState, the ChainID should be set
func (m *MerkleManager) SetChainID(chainID []byte) (err error) {
	m.cid = chainID
	m.MS, err = m.ReadChainHead(chainID)
	return err
}

// WriteChainHead
// Save the current MerkleState as the head of the chain.  This does not flush
// the database, because all chains in an application need have their head of
// chain updated as one atomic transaction to prevent messing up the database
func (m *MerkleManager) WriteChainHead(chainID []byte) error {
	state, err := m.MS.Marshal()
	if err != nil {
		return err
	}
	m.Manager.Key(chainID, "Head").PutBatch(state)
	return nil
}

// ReadChainHead
// Retrieve the current MerkleState from the given database, and set
// that state as the MerkleState of this MerkleManager.  Note that the cache
// will be cleared.
func (m *MerkleManager) ReadChainHead(chainID []byte) (ms *MerkleState, err error) {
	ms = new(MerkleState)
	ms.HashFunction = m.MS.HashFunction
	state, e := m.Manager.Key(chainID, "Head").Get() //   Get the state for the Merkle Tree
	if e == nil {                                    //   If the State exists
		if err := ms.UnMarshal(state); err != nil { //     Set that as our state
			return nil, fmt.Errorf("database is corrupt; failed to unmarshal %x",
				chainID) // Blow up of the database is bad
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
	m.MS, err = m.ReadChainHead(m.cid) //                  Set the MerkleState
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

	data, e := m.Manager.Key(m.cid, "States", element).Get() // Get the data at this height
	if e != nil {                                            // If nil, there is no state saved
		return nil //                                                   return nil, as no state exists
	}
	ms := new(MerkleState)                     //                                 Get a fresh new MerkleState
	if err := ms.UnMarshal(data); err != nil { //                                 set it the MerkleState
		panic(fmt.Sprintf("corrupted database; invalid state. error: %v", err)) // panic if the database is corrupted.
	}
	return ms // return it
}

// GetNext
// Get the next hash to be added to a state at this height
func (m *MerkleManager) GetNext(element int64) (hash Hash) {
	data, err := m.Manager.Key(m.cid, "NextElement", element).Get()
	if err != nil || len(data) != storage.KeyLength {
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
