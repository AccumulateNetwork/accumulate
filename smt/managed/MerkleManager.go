package managed

import (
	"crypto/sha256"
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
	RootDBManager *database.Manager // AppID-less Manager for writing to the general DBState
	MainChain     ChainManager      // The used to hold entries to be kept forever
	PendingChain  ChainManager      // Managed pend entries unsigned by validators in this Scratch chain to be signed
	BlkIdxChain   ChainManager      // Tracks the indexes of the minor blocks
	MarkPower     int64             // log2 of the MarkFreq
	MarkFreq      int64             // The count between Marks
	MarkMask      int64             // The binary mask to detect Mark boundaries
	blkidx        *BlockIndex       // The Last BlockIndex seen
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
	markPower int64) *MerkleManager { // log 2 of the frequency of creating marks in the Merkle Tree

	if len(appID) != 32 { // Panic: appID is bad
		panic(fmt.Sprintf("appID must be 32 bytes long. got %x", appID))
	}

	mm := new(MerkleManager)
	mm.init(DBManager, appID, markPower)
	return mm
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
	bAppID := Add2AppID(appID, BlkIdxOff)                              // appID for block Index shadow chain
	pAppID := Add2AppID(appID, PendingOff)                             // appID for pending shadow chain
	m.MainChain.Manager = m.MainChain.Manager.Copy(appID)              // MainChain.Manager uses appID
	m.MainChain.MS = *m.GetState(m.MainChain.Manager.GetCount())       // Get Main Merkle State from the database
	m.MainChain.MS.InitSha256()                                        // Use Sha256
	m.BlkIdxChain.Manager = m.BlkIdxChain.Manager.Copy(bAppID)         // BlkIdxChain.Manager uses bAppID
	m.BlkIdxChain.MS = *m.GetState(m.BlkIdxChain.Manager.GetCount())   // Get block Merkle State from the database
	m.BlkIdxChain.MS.InitSha256()                                      // Use Sha256
	m.PendingChain.Manager = m.PendingChain.Manager.Copy(pAppID)       // PendingChain uses pAppID
	m.PendingChain.MS = *m.GetState(m.PendingChain.Manager.GetCount()) // Get Pending Merkle State from the database
	m.PendingChain.MS.InitSha256()                                     // Use Sha256
	return &m
}

// getElementCount()
// Return number of elements in the Merkle Tree managed by this MerkleManager
func (m *MerkleManager) GetElementCount() (elementCount int64) {
	return m.MainChain.MS.Count
}

// init
// Create a Merkle Tree manager to collect hashes and build a Merkle Tree and a
// database behind it to validate hashes and create receipts.
//
// This is an internal routine; calling init outside of constructing the first
// reference to the MerkleManager doesn't make much sense.
func (m *MerkleManager) init(DBManager *database.Manager, appID []byte, markPower int64) {
	m.RootDBManager = DBManager.Copy(nil) // Add a Database manager without a appID
	_ = DBManager.AddBucket("BlockIndex") // Add a bucket to track block indexes
	_ = DBManager.AddBucket("Transactions")
	m.MainChain.Manager = DBManager.Copy(appID)                           // Save the database
	m.PendingChain.Manager = DBManager.Copy(Add2AppID(appID, PendingOff)) //
	m.BlkIdxChain.Manager = DBManager.Copy(Add2AppID(appID, BlkIdxOff))   //
	m.MainChain.Manager.BeginBatch()                                      // Start our batch mode
	m.MarkPower = markPower                                               // # levels in Merkle Tree to be indexed
	m.MarkFreq = int64(math.Pow(2, float64(markPower)))                   // The number of elements between indexes
	m.MarkMask = m.MarkFreq - 1                                           // Mask to index of next mark (0 if at a mark)
	m.MainChain.MS = *m.GetState(m.MainChain.Manager.GetCount())          // Get the main Merkle State from the database
	m.MainChain.MS.InitSha256()                                           // Use Sha256
	m.BlkIdxChain.MS = *m.GetState(m.BlkIdxChain.Manager.GetCount())      // Get block Merkle State from the database
	m.BlkIdxChain.MS.InitSha256()                                         // Use Sha256
	m.PendingChain.MS = *m.GetState(m.PendingChain.Manager.GetCount())    // Get pending Merkle State from the database
	m.PendingChain.MS.InitSha256()                                        // Use Sha256

}

// SetBlockIndex
// Keep track of where the blocks are in the Merkle Tree.
func (m *MerkleManager) SetBlockIndex(blockIndex int64) {
	if m.blkidx == nil { //                                              Load cache if first use
		m.blkidx = new(BlockIndex)                                    // Create blockIndex to store
		data := m.BlkIdxChain.Manager.Get("BlockIndex", "", []byte{}) // Look see the current bbi
		if data == nil {                                              // Then this is a new merkle tree
			m.blkidx.MainIndex = -1    //                                and no indexes exist
			m.blkidx.PendingIndex = -1 //                                so set them all to -1
			m.blkidx.BlockIndex = -1   //
		} else { //                                                      Otherwise,
			m.blkidx.UnMarshal(data) //                                  get all the values from the database
		}
	}
	if m.blkidx.BlockIndex >= blockIndex { //                            Block Index must increment
		panic("should not have block indexes that go backwards or stay constant")
	}
	if m.blkidx.MainIndex > m.MainChain.MS.Count-1 { //                  Must move MainChain or stay same
		panic("should not have main indexes that go backwards")
	}
	if m.blkidx.PendingIndex > m.PendingChain.MS.Count-1 { //            Must move Pending chain or stay same
		panic("should not have pending indexes that go backwards")
	}
	m.blkidx.BlockIndex = blockIndex                    //               Save blockIndex
	m.blkidx.MainIndex = m.MainChain.MS.Count - 1       //               Update MainIndex (count is index+1)
	m.blkidx.PendingIndex = m.PendingChain.MS.Count - 1 //               Update PendingIndex
	bbi := m.blkidx.Marshal()                           //               Marshal the Block Index record
	biHash := sha256.Sum256(bbi)                        //               Get the Hash of the bi
	m.BlkIdxChain.MS.AddToMerkleTree(biHash)            //               Add the bi hash to the BlkIdxChain
	blkIdx := m.BlkIdxChain.MS.Count - 1                //               Use a variable to make tidy
	_ = m.BlkIdxChain.Manager.Put(                      //
		"BlockIndex", "", common.Int64Bytes(blkIdx), bbi) //            blkIdx -> bbi struct
	_ = m.BlkIdxChain.Manager.Put("BlockIndex", "", []byte{}, bbi) //    Mark as highest block
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
	data := m.MainChain.Manager.Get("States", "", common.Int64Bytes(element)) // Get the data at this height
	if data == nil {                                                          //         If nil, there is no state saved
		return nil //                                                             return nil, as no state exists
	}
	ms := new(MerkleState) //                                                     Get a fresh new MerkleState
	ms.UnMarshal(data)     //                                                     set it up
	return ms              //                                                     return it
}

// GetNext
// Get the next hash to be added to a state at this height
func (m *MerkleManager) GetNext(element int64) (hash *Hash) {
	data := m.MainChain.Manager.Get("NextElement", "", common.Int64Bytes(element))
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
// Add a Hash to the MainChain
func (m *MerkleManager) AddHash(hash Hash) {
	m.MainChain.AddHash(m.MarkMask, hash)
}

// AddPendingHash
// Pending transactions for managed chains or suggested transactions go into the Pending Chain
func (m *MerkleManager) AddPendingHash(hash Hash) {
	m.PendingChain.AddHash(m.MarkMask, hash)
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
