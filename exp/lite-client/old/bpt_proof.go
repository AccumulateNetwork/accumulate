package liteclient

import (
	"bytes"
	"crypto/sha256"
)

// BptProof holds components of a Merkle inclusion proof
type BptProof struct {
	LeafHash []byte   // Hash of the account (or value) being proven
	Siblings [][]byte // Merkle path from leaf to root
	RootHash []byte   // Trusted root hash of the BPT
}

// FetchProof simulates retrieving a BPT inclusion proof for a given account URL.
// This mock builds a fake proof using dummy sibling hashes and recomputes the root.
func FetchProof(accountUrl string) *BptProof {
	// Step 1: Simulate the leaf hash (e.g., hash of account URL)
	leaf := []byte(accountUrl)
	leafHash := sha256.Sum256(leaf)

	// Step 2: Simulate sibling hashes (Merkle path)
	sibling1 := sha256.Sum256([]byte("sibling1"))
	sibling2 := sha256.Sum256([]byte("sibling2"))
	siblings := [][]byte{sibling1[:], sibling2[:]}

	// Step 3: Manually compute the Merkle root
	current := leafHash[:]
	for _, sib := range siblings {
		h := sha256.New()
		if bytes.Compare(current, sib) < 0 {
			h.Write(current)
			h.Write(sib)
		} else {
			h.Write(sib)
			h.Write(current)
		}
		current = h.Sum(nil)
	}

	return &BptProof{
		LeafHash: leafHash[:],
		Siblings: siblings,
		RootHash: current,
	}
}

// VerifyBptProof verifies a BPT inclusion proof using the leaf hash, siblings, and expected root.
func VerifyBptProof(leafHash []byte, siblings [][]byte, rootHash []byte) bool {
	if len(leafHash) != 32 || len(rootHash) != 32 {
		return false
	}

	current := leafHash
	for _, sib := range siblings {
		if len(sib) != 32 {
			return false
		}
		h := sha256.New()
		if bytes.Compare(current, sib) < 0 {
			h.Write(current)
			h.Write(sib)
		} else {
			h.Write(sib)
			h.Write(current)
		}
		current = h.Sum(nil)
	}
	return bytes.Equal(current, rootHash)
}

// Verify runs the BPT proof verification using the BptProof struct.
func (p *BptProof) Verify() bool {
	return VerifyBptProof(p.LeafHash, p.Siblings, p.RootHash)
}
