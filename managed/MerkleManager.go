package managed

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"

	"github.com/AccumulateNetwork/SMT/storage"

	"github.com/AccumulateNetwork/SMT/storage/database"
)

type MerkleManager struct {
	DBManager  *database.Manager // Database for holding the Merkle Tree
	MS         MerkleState       // The Merkle State Managed
	BlockIndex int64             // Index of the highest completed block
	MarkPower  int64             // log2 of the MarkFreq
	MarkFreq   int64             // The count between Marks
	MarkMask   int64             // The binary mask to detect Mark boundaries
}

// NewMerkleManager
// Create a new MerkleManager given a DBManager and markPower.
// The DBManager handles the persistence for the Merkle Tree under management
// The markPower is the log 2 frequency used to collect states in the Merkle Tree
func NewMerkleManager(
	DBManager *database.Manager, //             database that can be shared with other MerkleManager instances
	markPower int64, //                         log 2 of the frequency of creating marks in the Merkle Tree
	initialSalt []byte) *MerkleManager { //     Initial Salt identifying a Merkle Tree

	mm := new(MerkleManager)
	mm.Init(DBManager, markPower)
	mm.DBManager.SetSalt(initialSalt)
	return mm
}

// Copy
// Create a copy of the MerkleManager.  The MerkleManager can be pointed to
// particular merkle trees by setting the Salt.
func (m MerkleManager) Copy(salt []byte) *MerkleManager {
	m.DBManager = m.DBManager.Copy(salt)       // Point a DBManager at the specified salt
	m.MS = *m.GetState(m.DBManager.GetCount()) // Get the Merkle State from the database
	m.BlockIndex = 0                           // Clear BlockIndex since elements could be zero
	if m.MS.Count > 0 {                        // If MS has elements, Get the BlockIndex
		m.BlockIndex = m.DBManager.GetInt64("BlockIndex", "", []byte{}) // Get the highest block index
	}
	m.MS.InitSha256() // Use Sha256 TODO: Actually update the library to be able to use various hash algorithms
	return &m
}

// CurrentSalt
// Return the current salt used by the MerkleManager
func (m *MerkleManager) CurrentSalt() (salt []byte) {
	salt = append(salt, m.DBManager.Salt...)
	return salt
}

// getElementCount()
// Return the number of elements in the Merkle Tree managed by this MerkleManager
func (m *MerkleManager) GetElementCount() (elementCount int64) {
	return m.MS.Count
}

// Init
// Create a Merkle Tree manager to collect hashes and build a Merkle Tree and a
// database behind it to validate hashes and create receipts
func (m *MerkleManager) Init(DBManager *database.Manager, markPower int64) {
	_ = DBManager.AddBucket("BlockIndex")               // Add a bucket to track block indexes
	m.DBManager = DBManager                             // Save the database
	m.DBManager.BeginBatch()                            // Start our batch mode
	m.MarkPower = markPower                             // Number of levels in the Merkle Tree to be indexed
	m.MarkFreq = int64(math.Pow(2, float64(markPower))) // The number of elements between indexes
	m.MarkMask = m.MarkFreq - 1                         // Mask to the index of the next mark (0 if at a mark)
	m.MS = *m.GetState(m.DBManager.GetCount())          // Get the Merkle State from the database
	if m.MS.Count > 0 {
		m.BlockIndex = m.DBManager.GetInt64("BlockIndex", "", []byte{}) // Get the highest block index
	}
	m.MS.InitSha256() // Use Sha256
}

// SetBlockIndex
// Keep track of where the blocks are in the Merkle Tree.
func (m *MerkleManager) SetBlockIndex() {
	holdSalt := m.CurrentSalt()                                                // Hold the current salt
	defer func() { m.DBManager.SetSalt(holdSalt) }()                           // and reset the salt when we are done
	m.DBManager.SetSalt([]byte{})                                              // BlockIndex is handled outside salts
	bi := new(BlockIndex)                                                      // Create a blockIndex to store
	bi.BlockIndex = m.BlockIndex                                               // Save the current BlockIndex
	bi.ElementIndex = m.MS.Count - 1                                           // Save the ElementIndex (count-1)
	m.DBManager.Put("BlockIndex", "", Int64Bytes(bi.BlockIndex), bi.Marshal()) // Put the BlockIndex in the database
	m.DBManager.Put("BlockIndex", "", []byte{}, bi.Marshal())                  // Mark this as the new highest block
	m.BlockIndex++                                                             // BlockIndex is now one greater
}

// CurrentBlockIndex
// Return the current block index

// AddHash
// Pull from the HashFeed channel and add to the Merkle Tree managed by the MerkleManager
func (m *MerkleManager) AddHash(hash Hash) {
	// Keep the index of every element added to the Merkle Tree, but only of the first instance
	if m.DBManager.GetIndex(hash[:]) < 0 { // So only if the hash is not yet added to the Merkle Tree
		_ = m.DBManager.PutBatch("ElementIndex", "", hash[:], Int64Bytes(m.MS.Count)) // Keep its index
	}

	if (m.MS.Count+1)&m.MarkMask == 0 { // If we are about to roll into a Mark
		MSCount := Int64Bytes(m.MS.Count)
		MSState := m.MS.Marshal()
		_ = m.DBManager.PutBatch("States", "", MSCount, MSState)      //     Save Merkle State at n*MarkFreq-1
		_ = m.DBManager.PutBatch("NextElement", "", MSCount, hash[:]) //     Save Hash added at n*MarkFreq-1
		//
		m.MS.AddToMerkleTree(hash) //                                    Add the hash to the Merkle Tree
		//
		state := m.MS.Marshal()                                           // Create the marshaled Merkle State
		m.MS.HashList = m.MS.HashList[:0]                                 // Clear the HashList
		MSCount = Int64Bytes(m.MS.Count)                                  // Update MSCount
		_ = m.DBManager.PutBatch("Element", "", []byte("Count"), MSCount) // Put the Element Count in DB
		_ = m.DBManager.PutBatch("States", "", MSCount, state)            // Save Merkle State at n*MarkFreq
		//                   Saving the count in the database actually gives the index of the
		//                   next element in the merkle tree.  This is because counting is one based,
		//                   and indexing is zero based.  There is an implied count-1 going on here.
	} else { //
		m.MS.AddToMerkleTree(hash) //                                            Always add to the merkle tree
	}

	if len(m.DBManager.TXList.List) > 1000 {
		m.DBManager.EndBatch()
	}
}

// GetState
// Get a MerkleState for a given index
func (m *MerkleManager) GetState(element int64) *MerkleState {
	if element == 0 {
		return new(MerkleState)
	}
	data := m.DBManager.Get("States", "", Int64Bytes(element)) // Get the data at this height
	if data == nil {                                           // If we get a nil, there is no state saved
		return nil //                                            return nil, as no state exists
	}
	ms := new(MerkleState) // Get a fresh new merklestate
	ms.UnMarshal(data)     // set it up
	return ms              // return it
}

// GetNext
// Get the next hash to be added to a state at this height
func (m *MerkleManager) GetNext(element int64) (hash *Hash) {
	data := m.DBManager.Get("NextElement", "", Int64Bytes(element))
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
	return m.DBManager.GetIndex(element)
}

// Receipt
// Take a receipt and validate that the
func (r Receipt) Validate() bool {
	anchor := r.Element // To begin with, we start with the object as the anchor
	// Now apply all the path hashes to the anchor
	for _, node := range r.Nodes {
		// Need a [32]byte to slice
		hash := [32]byte(node.Hash)
		if node.Right {
			// If this hash comes from the right, apply it that way
			anchor = sha256.Sum256(append(anchor[:], hash[:]...))
		} else {
			// If this hash comes from the left, apply it that way
			anchor = sha256.Sum256(append(hash[:], anchor[:]...))
		}
	}
	// In the end, anchor should be the same hash the receipt expects.
	return anchor == r.Anchor
}

// AddHashString
// Often instead of a hash, we have a hex string, but that's okay too.
func (m *MerkleManager) AddHashString(hash string) {
	if h, err := hex.DecodeString(hash); err != nil {
		panic(fmt.Sprintf("failed to decode a hash %s with error %v", hash, err))
	} else {
		var h2 [32]byte
		copy(h2[:], h)
		m.AddHash(h2)
	}
}
