package managed

import (
	"encoding/hex"
	"fmt"
	"math"

	"github.com/AccumulateNetwork/accumulated/smt/common"
	"github.com/AccumulateNetwork/accumulated/smt/storage"
	"github.com/AccumulateNetwork/accumulated/smt/storage/database"
)

const PendingOff = byte(1) // Offset to the chain holding pending transactions for managed chains
const BlkIdxOff = byte(2)  // Offset to the chain holding the block indexes for the chain and the pending chain

type MerkleManager struct {
	Manager   *database.Manager // AppID based Manager
	MS        *MerkleState      // MerkleState managed by MerkleManager
	MarkPower int64             // log2 of the MarkFreq
	MarkFreq  int64             // The count between Marks
	MarkMask  int64             // The binary mask to detect Mark boundaries
}

// AddHash
// Add a Hash to the Chain controlled by the ChainManager
func (m *MerkleManager) AddHash(hash Hash) {
	// Keep the index of every element added to the Merkle Tree, but only of the first instance
	if m.Manager.GetIndex(hash[:]) < 0 { // So only if the hash is not yet added to the Merkle Tree
		m.Manager.PutBatch("ElementIndex", "", hash[:], common.Int64Bytes(m.MS.Count)) // Keep its index
	}

	switch m.MS.Count & m.MarkMask { // Mask to the bits counting up to a mark.
	case m.MarkMask: // This is the case just before rolling into a Mark (all bits set. +1 more will be zero)

		MSCount := common.Int64Bytes(m.MS.Count) // Get the current index as a varInt
		MSState, err := m.MS.Marshal()           // Get the current state
		if err != nil {                          // Panic if we cannot, because that should never happen.
			panic(fmt.Sprintf("could not marshal MerkleState: %v", err)) //
		}

		m.Manager.PutBatch("States", "", MSCount, MSState)      //     Save Merkle State at n*MarkFreq-1
		m.Manager.PutBatch("NextElement", "", MSCount, hash[:]) //     Save Hash added at n*MarkFreq-1

		m.MS.AddToMerkleTree(hash) //                                  Add the hash to the Merkle Tree

		state, err := m.MS.Marshal()                     // Create the marshaled Merkle State
		m.Manager.PutBatch("States", "", MSCount, state) // Save Merkle State at n*MarkFreq
		m.MS.HashList = m.MS.HashList[:0]                // We need to clear the HashList first

	default: // This is the case of not rolling into a mark; Just add the hash to the Merkle Tree
		m.MS.AddToMerkleTree(hash) //                                      Always add to the merkle tree
	}

	if err := m.WriteChainHead(); err != nil {
		panic(fmt.Sprintf("could not marshal MerkleState: %v", err))
	}
}

// WriteChainHead
// Save the current MerkleState as the head of the chain.  This does not flush
// the database, because all chains in an application need have their head of
// chain updated as one atomic transaction to prevent messing up the database
func (m *MerkleManager) WriteChainHead() error {
	state, err := m.MS.Marshal()
	if err != nil {
		return err
	}
	m.Manager.PutBatch("States", "", []byte("Head"), state)
	return nil
}

// ReadChainHead
// Retrieve the current MerkleState from the given database, and set
// that state as the MerkleState of this MerkleManager.  Note that the cache
// will be cleared.
func (m *MerkleManager) ReadChainHead() {
	state := m.Manager.Get("States", "", []byte("Head")) // Get the state for the Merkle Tree
	if state != nil {                                    // If the State exists
		if err := m.MS.UnMarshal(state); err != nil { //     Set that as our state
			panic(fmt.Sprintf("database is corrupt. AppID: %x", m.Manager.AppID)) // Blow up of the database is bad
		}
	}
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

// Add2AppID
// Compute the AppID for a shadow chain for a Merkle Tree. Note that bumping
// the index of the AppID allows shadow chains to exist within a different
// application space, and allows the underlying Patricia Tree to maintain
// its balanced nature.
//
// The reason for shadow chains is to track information about the construction
// and validation of entries in a main chain that we do not wish to persist
// forever.  We track two shadow chains at present.  The PendingOff implements
// managed chains where the rules for entries must be evaluated by the Identity
// before they are added to the chain. So other identities can write to
// the chain, but those entries get redirected to the PendingOff shadow
// chain until they are authorized by the Identity.
//
// The BlkIdxOff collects the block indexes to track where the minor
// blocks end so that entries can be recognized as timestamped at this finer
// resolution even after the minor blocks are rewritten into major blocks.
// Parties interested in this finer resolution can preserve these timestamps
// by simply getting a receipt and preserving it off chain or on chain.
func Add2AppID(AppID []byte, offset byte) (appID []byte) {
	appID = append(appID, AppID...) // Make a copy of the appID
	appID[0] += offset              // Add offset to the first byte -> new appID
	return appID                    // and return new appID
}

// NewMerkleManager
// Create a new MerkleManager given a MainChain.Manager and markPower.
// The MainChain.Manager handles the persistence for the Merkle Tree under management
// The markPower is the log 2 frequency used to collect states in the Merkle Tree
func NewMerkleManager(
	DBManager *database.Manager, //      database that can be shared with other MerkleManager instances
	appID []byte, //                     AppID identifying an application space in the DBManager
	markPower int64) (*MerkleManager, error) { // log 2 of the frequency of creating marks in the Merkle Tree

	if len(appID) != 32 { // Panic: appID is bad
		return nil, fmt.Errorf("appID must be 32 bytes long. got %x", appID)
	}

	mm := new(MerkleManager)
	if err := mm.init(DBManager, appID, markPower); err != nil {
		return nil, err
	}

	return mm, nil
}

// Copy
// Create a copy of the MerkleManager.  The MerkleManager can be pointed to
// particular merkle trees by setting the AppID. While the AppID is generally
// the management of a particular merkle tree, it can be used to define any
// application context within the database.
//
// Copy creates a MerkleManager that points it to a particular ChainID (use
// the chainID as the appID)
//
// Copy creates a brand new MerkleManager pointed towards a particular
// chain and its shadow chains (chains that track information about the
// main chain, i.e. the pending chain and the block index chain)
func (m MerkleManager) Copy(appID []byte) *MerkleManager {
	m.Manager = m.Manager.ManageAppID(appID) // MainChain.Manager uses appID
	m.MS = m.MS.Copy()                       // Make a copy of the Merkle State
	m.MS.InitSha256()                        // Use Sha256
	return &m
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
func (m *MerkleManager) init(DBManager *database.Manager, appID []byte, markPower int64) error {

	if markPower >= 20 { // 2^20 is 1,048,576 and is too big for the collection of elements in memory
		return fmt.Errorf("A power %d is greater than 2^29, and is unreasonable", markPower)
	}

	if m.MS == nil { //                                    Allocate a MS if we don't have one
		m.MS = new(MerkleState) //
		m.MS.InitSha256()       //
	}
	m.Manager = DBManager.ManageAppID(appID) //               Manager for writing the Merkle states
	m.ReadChainHead()                        //               Set the MerkleState

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

	data := m.Manager.Get("States", "", common.Int64Bytes(element)) // Get the data at this height
	if data == nil {                                                // If nil, there is no state saved
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
func (m *MerkleManager) GetNext(element int64) (hash *Hash) {
	data := m.Manager.Get("NextElement", "", common.Int64Bytes(element))
	if data == nil || len(data) != storage.KeyLength {
		return nil
	}
	hash = new(Hash)
	copy(hash[:], data)
	return hash
}

// GetIndex
// Get the index of a given element
func (m *MerkleManager) GetIndex(element []byte) (index int64) {
	return m.Manager.GetIndex(element)
}

// AddHashString
// Often instead of a hash, we have a hex string, but that's okay too.
func (m *MerkleManager) AddHashString(hash string) {
	if h, err := hex.DecodeString(hash); err != nil { //                             Convert to a binary slice
		panic(fmt.Sprintf("failed to decode a hash %s with error %v", hash, err)) // If this fails, panic; no recovery
	} else {
		var h2 [32]byte // Make the slice into a 32 byte array
		copy(h2[:], h)  // copy the data
		m.AddHash(h2)   // Add the hash
	}
}
