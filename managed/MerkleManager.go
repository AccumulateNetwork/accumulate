package managed

import (
	"encoding/hex"
	"fmt"
	"math"

	"github.com/AccumulateNetwork/SMT/storage"

	"github.com/AccumulateNetwork/SMT/storage/database"
)

const PendingOff = byte(1) // Offset to the chain holding pending transactions for managed chains
const BlkIdxOff = byte(2)  // Offset to the chain holding the block indexes for the chain and the pending chain

type MerkleManager struct {
	RootDBManager *database.Manager // Salt-less Manager for writing to the general DBState
	MainChain     ChainManager      // The used to hold entries to be kept forever
	PendingChain  ChainManager      // Managed pend entries unsigned by validators in this Scratch chain to be signed
	BlkIdxChain   ChainManager      // Tracks the indexes of the minor blocks
	MarkPower     int64             // log2 of the MarkFreq
	MarkFreq      int64             // The count between Marks
	MarkMask      int64             // The binary mask to detect Mark boundaries
}

// add2Salt
// Compute the Salt for a shadow chain for a Merkle Tree. Simply a convenience
// function, but an opportunity to talk about shadow chains here
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
func add2Salt(Salt []byte, offset byte) (salt []byte) {
	salt = append(salt, Salt...) // Make a copy of the salt
	salt[31] += offset           // Add offset to low order byte -> new salt
	return salt                  // and return new salt
}

// NewMerkleManager
// Create a new MerkleManager given a MainChain.Manager and markPower.
// The MainChain.Manager handles the persistence for the Merkle Tree under management
// The markPower is the log 2 frequency used to collect states in the Merkle Tree
func NewMerkleManager(
	DBManager *database.Manager, //      database that can be shared with other MerkleManager instances
	initialSalt []byte, //               Initial Salt identifying a Merkle Tree
	markPower int64) *MerkleManager { // log 2 of the frequency of creating marks in the Merkle Tree

	mm := new(MerkleManager)
	mm.init(DBManager, initialSalt, markPower)
	return mm
}

// Copy
// Create a copy of the MerkleManager.  The MerkleManager can be pointed to
// particular merkle trees by setting the Salt.  This is a brand new MerkleManager pointed
// towards a particular chain and its shadow chains
func (m MerkleManager) Copy(salt []byte) *MerkleManager {
	m.MainChain.Manager = m.MainChain.Manager.Copy(salt)         // Point a MainChain.Manager at the specified salt
	m.MainChain.MS = *m.GetState(m.MainChain.Manager.GetCount()) // Get the Merkle State from the database
	m.MainChain.MS.InitSha256()                                  // Use Sha256
	m.MainChain.Manager = m.MainChain.Manager.Copy(salt)         // Point a MainChain.Manager at the specified salt
	m.MainChain.MS = *m.GetState(m.MainChain.Manager.GetCount()) // Get the Merkle State from the database
	m.MainChain.MS.InitSha256()                                  // Use Sha256
	return &m
}

// getElementCount()
// Return the number of elements in the Merkle Tree managed by this MerkleManager
func (m *MerkleManager) GetElementCount() (elementCount int64) {
	return m.MainChain.MS.Count
}

// init
// Create a Merkle Tree manager to collect hashes and build a Merkle Tree and a
// database behind it to validate hashes and create receipts.
//
// This is an internal routine; calling init outside of constructing the first
// reference to the MerkleManager doesn't make much sense.
func (m *MerkleManager) init(DBManager *database.Manager, salt []byte, markPower int64) {
	m.RootDBManager = DBManager.Copy(nil)                               // Add a Database manager without a salt
	_ = DBManager.AddBucket("BlockIndex")                               // Add a bucket to track block indexes
	m.MainChain.Manager = DBManager.Copy(salt)                          // Save the database
	m.PendingChain.Manager = DBManager.Copy(add2Salt(salt, PendingOff)) //
	m.BlkIdxChain.Manager = DBManager.Copy(add2Salt(salt, BlkIdxOff))   //
	m.MainChain.Manager.BeginBatch()                                    // Start our batch mode
	m.MarkPower = markPower                                             // Number of levels in the Merkle Tree to be indexed
	m.MarkFreq = int64(math.Pow(2, float64(markPower)))                 // The number of elements between indexes
	m.MarkMask = m.MarkFreq - 1                                         // Mask to the index of the next mark (0 if at a mark)
	m.MainChain.MS = *m.GetState(m.MainChain.Manager.GetCount())        // Get the Merkle State from the database
	m.MainChain.MS.InitSha256()                                         // Use Sha256
}

// SetBlockIndex
// Keep track of where the blocks are in the Merkle Tree.
// ToDo: We have to actually enforce the block over all the chains.  Blocks remain broken.
func (m *MerkleManager) SetBlockIndex(blockIndex int64) {
	bi := new(BlockIndex)                                                           // Create blockIndex to store
	bi.BlockIndex = blockIndex                                                      // Save current BlockIndex
	bi.MainIndex = m.MainChain.MS.Count - 1                                         // Save MainIndex (count-1)
	_ = m.RootDBManager.Put("BlockIndex", "", Int64Bytes(blockIndex), bi.Marshal()) // BlockIndex -> database
	_ = m.RootDBManager.Put("BlockIndex", "", []byte{}, bi.Marshal())               // Mark as highest block
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
	data := m.MainChain.Manager.Get("States", "", Int64Bytes(element)) // Get the data at this height
	if data == nil {                                                   // If we get a nil, there is no state saved
		return nil //                                                     return nil, as no state exists
	}
	ms := new(MerkleState) // Get a fresh new MerkleState
	ms.UnMarshal(data)     // set it up
	return ms              // return it
}

// GetNext
// Get the next hash to be added to a state at this height
func (m *MerkleManager) GetNext(element int64) (hash *Hash) {
	data := m.MainChain.Manager.Get("NextElement", "", Int64Bytes(element))
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
	return m.MainChain.Manager.GetIndex(element)
}

// AddHash
// Pull from the HashFeed channel and add to the Merkle Tree managed by the MerkleManager
func (m *MerkleManager) AddHash(hash Hash) {
	m.MainChain.AddHash(m.MarkMask, hash)
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
