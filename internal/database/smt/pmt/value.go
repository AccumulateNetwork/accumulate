// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pmt

import "bytes"

// Value
// holds the key / hash mapping for the BPT. With Accumulate, the key
// represents a ChainID and a state Hash for a chain in the protocol
type Value struct {
	Key  [32]byte // The key for the Patricia Tree value
	Hash [32]byte // The current value for the key
}

// Node
// Returns true if this is a Node, otherwise it is a value
func (v *Value) T() int {
	return TValue
}

// GetHash
// Returns the hash as a slice
func (v *Value) GetHash() []byte {
	return append([]byte{}, v.Hash[:]...)
}

// Marshal
// Return the concatenation of the Key and Hash of the value
func (v *Value) Marshal() []byte {
	return append(v.Key[:], v.Hash[:]...) // Return the key and hash concatenated together
}

// UnMarshal
// Load the Value with the state marshalled to the given data slice
func (v *Value) UnMarshal(data []byte) []byte {
	copy(v.Key[:], data[:32])  // ManageAppID the Key
	data = data[32:]           // move the data slice
	copy(v.Hash[:], data[:32]) // ManageAppID the Hash
	data = data[32:]           // Move the data slice
	return data                // Return the updated data slice
}

// Equal
// Return true if a given Entry is a Value instance, and has the same
// Key and Hash as this Value
func (v *Value) Equal(entry Entry) (equal bool) {

	defer func() { //                          If we access a nil, it is because something is missing
		if err := recover(); err != nil { //
			equal = false
		}
	}()

	// We compare only down the BPT.  If we compare both up and down the tree,
	// then the code would loop infinitely.  Certainly we could avoid retracing
	// paths, but if we wish to compare entire BPT trees, we can compare their
	// roots.
	value := entry.(*Value) //                           The entry we are considering must be a node
	switch {
	case !bytes.Equal(v.Key[:], value.Key[:]):
		return false
	case !bytes.Equal(v.Hash[:], value.Hash[:]):
		return false
	}
	return true
}
