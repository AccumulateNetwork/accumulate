// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

// MerkleState
// A Merkle Dag State is the state kept while building a Merkle Tree.  Except where a Merkle Tree has a clean
// power of two number of elements as leaf nodes, there will be multiple Sub Merkle Trees that make up a
// dynamic Merkle Tree. The Merkle State is the list of the roots of these sub Merkle Trees, and the
// combination of these roots provides a Directed Acyclic Graph (DAG) to all the leaves.
//
//	                                                     Merkle State
//	1  2   3  4   5  6   7  8   9 10  11 12  13 --->         13
//	 1-2    3-4    5-6    7-8   0-10  11-12     --->         --
//	    1-2-3-4       5-6-7-8    0-10-11-12     --->     0-10-11-12
//	          1-2-3-4-5-6-7-8                   --->   1-2-3-4-5-6-7-8
//
// Interestingly, the state of building such a Merkle Tree looks just like counting in binary.  And the
// higher order bits set will correspond to where the binary roots must be kept in a Merkle state.
type MerkleState struct {
	merkle.State
}

// String
// convert the MerkleState to a human readable string
func (m MerkleState) String() string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("%20s %d\n", "Count", m.Count))
	b.WriteString(fmt.Sprintf("%20s %d\n", "Pending[] length:", len(m.Pending)))
	for i, v := range m.Pending {
		vp := "nil"
		if v != nil {
			vp = fmt.Sprintf("%x", v)
		}
		b.WriteString(fmt.Sprintf("%20s [%3d] %s\n", "", i, vp))
	}
	b.WriteString(fmt.Sprintf("%20s %d\n", "HashList Length:", len(m.HashList)))
	for i, v := range m.HashList {
		vp := fmt.Sprintf("%x", v)
		b.WriteString(fmt.Sprintf("%20s [%3d] %s\n", "", i, vp))
	}
	return b.String()
}

// Copy
// Make a completely independent copy of the Merkle State that removes all
// references to the structures in the given Merkle State.  This means copying
// any entries in the Pending slice
func (m *MerkleState) Copy() *MerkleState {
	return &MerkleState{*m.State.Copy()}
}

func (m *MerkleState) UnmarshalBinary(data []byte) error {
	return m.State.UnMarshal(data)
}

func (m *MerkleState) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return m.State.UnMarshal(data)
}

func (m *MerkleState) CopyAsInterface() interface{} {
	return m.Copy()
}

// Equal
// Compares one MerkleState to another, and returns true if they are the same
func (m *MerkleState) Equal(n *MerkleState) bool {
	return m.State.Equal(&n.State)
}

// UnMarshal
// Take the state of an MSMarshal instance defined by MSBytes, and set all the values
// in this instance of MSMarshal to the state defined by MSBytes.  It is assumed that the
// hash function has been set by the caller.
func (m *MerkleState) UnMarshal(MSBytes []byte) (err error) {
	return m.State.UnMarshal(MSBytes)
}

// GetSha256
// Get the a Sha256 function that can be used to create hashes compatible with a Stateful
// Merkle tree using sha256
func GetSha256() func(data []byte) Hash {
	return func(data []byte) Hash {
		h := sha256.Sum256(data)
		return h[:]
	}
}

// InitSha256
// Set the hashing function of this Merkle State to Sha256
// TODO: Actually update the library to be able to use various hash algorithms
func (m *MerkleState) InitSha256() {
}

// AddToMerkleTree
// Add a Hash to the merkle tree and incrementally build the MerkleState
func (m *MerkleState) AddToMerkleTree(hash_ []byte) {
	m.Add(hash_)
}

// GetMDRoot
// Compute the Merkle Directed Acyclic Graph (Merkle DAG or MerkleState) for
// the MerkleState at this point We take any trailing hashes in MerkleState,
// hash them up and combine to create the Merkle Dag Root. Getting the closing
// ListMDRoot is non-destructive, which is useful for some use cases.
//
// Returns a nil if the MerkleSate is empty.
func (m *MerkleState) GetMDRoot() (MDRoot Hash) {
	return m.Anchor()
}

// PrintMR
// For debugging purposes, it is nice to get a string that shows the nil and non nil entries in c.MerkleState
// Note that the "low order" entries are first in the string, so the binary is going from low order on the left to
// high order going right in the string rather than how binary is normally represented.
func (m *MerkleState) PrintMR() (mr string) {
	for _, v := range m.Pending {
		if v != nil {
			mr += "O"
			continue
		}
		mr += "_"
	}
	return mr
}

// GetIntermediate returns the last two hashes that were combined to create the
// local Merkle Root at the given index. The element Pending List must be fully
// populated up to height specified.
func (m *MerkleState) GetIntermediate(hash Hash, height int64) (left, right Hash, err error) {
	m.Pad()                       // Pad Pending with a nil to remove corner cases
	for i, v := range m.Pending { // Adding the hash is like incrementing a variable
		if v == nil { //               Look for an empty slot; should not encounter one
			return nil, nil, fmt.Errorf("should not encounter a nil at height %d", height)
		}
		if i+1 == int(height) { // Found the height
			left = Hash(v).Copy()   // Get the left and right
			right = hash.Copy()     //
			return left, right, nil // return them
		}
		hash = Hash(v).Combine(Sha256, hash) // If this slot isn't empty, combine the hash with the slot
	}
	return nil, nil, fmt.Errorf("no values found at height %d", height)
}
