package managed

import "crypto/sha256"

// RandHash
// Creates a deterministic stream of hashes.  We need this all over our tests
// and though this is late to the party, it is very useful in testing in
// SMT
type RandHash struct {
	seed [32]byte
	List [][]byte
}

// SetSeed
// If the tester wishes a different sequence of hashes, a seed can be
// specified
func (n *RandHash) SetSeed(seed []byte) {
	n.seed = sha256.Sum256(seed)
}

// Next
// Returns the next hash in a deterministic sequence of hashes
func (n *RandHash) Next() []byte {
	n.seed = sha256.Sum256(n.seed[:])
	return append([]byte{}, n.seed[:]...)
}

// Next
// Just like Next, but each hash is logged in RandHash.List
func (n *RandHash) NextList() []byte {
	h := n.Next()
	n.List = append(n.List, h)
	return h
}
