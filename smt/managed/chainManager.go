package managed

import (
	"github.com/AccumulateNetwork/SMT/smt/storage"
	"github.com/AccumulateNetwork/SMT/smt/storage/database"
)

// ChainManager
// Represents a struct for maintaining a Merkle Tree.  Most chains have two
// Merkle Trees, the main chain and a shadow chain tracking blocks
type ChainManager struct {
	Manager *database.Manager
	MS      MerkleState
}

// AddHash
// Add a Hash to the Chain controlled by the ChainManager
func (m *ChainManager) AddHash(MarkMask int64, hash Hash) {
	// Keep the index of every element added to the Merkle Tree, but only of the first instance
	if m.Manager.GetIndex(hash[:]) < 0 { // So only if the hash is not yet added to the Merkle Tree
		_ = m.Manager.PutBatch("ElementIndex", "", hash[:], storage.Int64Bytes(m.MS.Count)) // Keep its index
	}

	if (m.MS.Count+1)&MarkMask == 0 { // If we are about to roll into a Mark
		MSCount := storage.Int64Bytes(m.MS.Count)
		MSState := m.MS.Marshal()
		_ = m.Manager.PutBatch("States", "", MSCount, MSState)      //   Save Merkle State at n*MarkFreq-1
		_ = m.Manager.PutBatch("NextElement", "", MSCount, hash[:]) //   Save Hash added at n*MarkFreq-1
		//
		m.MS.AddToMerkleTree(hash) //                                    Add the hash to the Merkle Tree
		//
		state := m.MS.Marshal()                                         // Create the marshaled Merkle State
		m.MS.HashList = m.MS.HashList[:0]                               // Clear the HashList
		MSCount = storage.Int64Bytes(m.MS.Count)                        // Update MainChain.MSCount
		_ = m.Manager.PutBatch("Element", "", []byte("Count"), MSCount) // Put the Element Count in DB
		_ = m.Manager.PutBatch("States", "", MSCount, state)            // Save Merkle State at n*MarkFreq
		//                   Saving the count in the database actually gives the index of the
		//                   next element in the merkle tree.  This is because counting is one based,
		//                   and indexing is zero based.  There is an implied count-1 going on here.
	} else { //
		m.MS.AddToMerkleTree(hash) //                                      Always add to the merkle tree
	}

	if len(m.Manager.TXList.List) > 1000 {
		m.Manager.EndBatch()
	}
}
