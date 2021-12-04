package protocol

import (
	"crypto/sha256"

	"github.com/AccumulateNetwork/accumulate/smt/managed"
)

// ComputeEntryHash
// returns the entry hash given external id's and data associated with an entry
func ComputeEntryHash(data [][]byte) []byte {
	smt := managed.MerkleState{}
	//add the external id's to the merkle tree
	for i := range data {
		h := sha256.Sum256(data[i])
		smt.AddToMerkleTree(h[:])
	}
	//return the entry hash
	return smt.GetMDRoot().Bytes()
}
