package protocol

import (
	"crypto/sha256"

	"github.com/AccumulateNetwork/accumulate/smt/managed"
)

// ComputeEntryHash
// returns the entry hash given external id's and data associated with an entry
func ComputeEntryHash(root []byte, extIds [][]byte, data []byte) []byte {
	smt := managed.MerkleState{}
	//add the external id's to the merkle tree
	for i := range extIds {
		h := sha256.Sum256(extIds[i])
		smt.AddToMerkleTree(h[:])
	}
	//add the data to the merkle tree
	h := sha256.Sum256(data)
	smt.AddToMerkleTree(h[:])
	//return the entry hash
	return smt.GetMDRoot().Bytes()
}
