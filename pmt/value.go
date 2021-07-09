package pmt

import "crypto/sha256"

// Value
// holds the key / hash mapping for the BPT. With Accumulate, the key
// represents a ChainID and a state Hash for a chain in the protocol
type Value struct {
	Key  [32]byte // The key for the Patricia Tree value
	Hash [32]byte // The current value for the key
}

// Node
// Returns true if this is a Node, otherwise it is a value
func (v *Value) T() bool {
	return false
}

// GetHash
// Returns the combination hash of the Key and the Hash.  This is the
// state that really must be proven to users
func (v *Value) GetHash() []byte {
	h := sha256.Sum256(append(v.Key[:], v.Hash[:]...))
	return h[:]
}
