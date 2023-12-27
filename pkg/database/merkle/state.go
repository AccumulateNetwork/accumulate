// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"bytes"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
)

// State
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

func (m *State) MarshalBinary() ([]byte, error) {
	return m.Marshal()
}

func (m *State) UnmarshalBinary(data []byte) error {
	return m.UnMarshal(data)
}

func (m *State) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return m.UnMarshal(data)
}

// Equal
// Compares one State to another, and returns true if they are the same
func (m *State) Equal(m2 *State) (isEqual bool) {
	// Any errors indicate at m is not the same as m2, or either m or m2 or both is malformed.
	defer func() {
		if recover() != nil {
			isEqual = false
			return
		}
	}()
	// Count is how many elements in the merkle tree.  should be the same
	if m.Count != m2.Count {
		return false
	}

	for i, v := range m.Pending { // First check if all non nil elements of m.Pending == m2.Pending
		if v == nil && len(m2.Pending) <= i { // If m1 has trailing nils where m mas nils, that's okay
			continue
		}
		if v != nil && len(m2.Pending) <= i { // If m1 has trailing nils where m has values, not okay
			return false
		}
		if !bytes.Equal(v, m2.Pending[i]) { //  all values in both lists need to be equal
			return false
		}
	}
	idx := len(m2.Pending)     // Index of the last entry of m2.Pending
	for len(m.Pending) < idx { // Check that if m.Pending is shorter, that all of m1's extra entries are nil
		if m2.Pending[idx-1] != nil {
			return false
		}
		idx--
	}

	// Each must have the same number of elements in the HashList
	if len(m.HashList) != len(m2.HashList) {
		return false
	} else {
		// Each element in the HashLists must be equal
		for i, v := range m.HashList {
			if !bytes.Equal(v, m2.HashList[i]) {
				return false
			}
		}
	}
	// If we made it here, all is golden.
	return true
}

// Pad
// Add a nil to the end of the Pending list if one isn't there.
// We need to be able to add an element to Pending if needed while
// building receipts
func (m *State) Pad() {
	if len(m.Pending) == 0 || m.Pending[len(m.Pending)-1] != nil {
		m.Pending = append(m.Pending, nil)
	}
}

// Trim
// Remove any trailing nils from Pending hashes
func (m *State) Trim() {
	for len(m.Pending) > 0 && m.Pending[len(m.Pending)-1] == nil {
		m.Pending = m.Pending[:len(m.Pending)-1]
	}
}

// Marshal
// Encodes the Merkle State so it can be embedded into the Merkle Tree
func (m *State) Marshal() (MSBytes []byte, err error) {
	MSBytes = append(MSBytes, common.Int64Bytes(m.Count)...) // Count

	// Write out the pending list. Each bit set in Count indicates a Sub Merkle
	// Tree root. For each bit, if the bit is set, record the hash.
	b, err := sparseHashList(m.Pending).MarshalBinary(m.Count)
	if err != nil {
		return nil, err
	}
	MSBytes = append(MSBytes, b...)

	// Write out the hash list (never returns an error)
	b, _ = hashList(m.HashList).MarshalBinary()
	MSBytes = append(MSBytes, b...)
	return MSBytes, nil
}

// UnMarshal
// Take the state of an MSMarshal instance defined by MSBytes, and set all the values
// in this instance of MSMarshal to the state defined by MSBytes.  It is assumed that the
// hash function has been set by the caller.
func (m *State) UnMarshal(MSBytes []byte) (err error) {
	// Unmarshal the Count
	m.Count, err = encoding.UnmarshalInt(MSBytes)
	if err != nil {
		return err
	}
	MSBytes = MSBytes[len(encoding.MarshalInt(m.Count)):]

	// Unmarshal the pending list
	err = (*sparseHashList)(&m.Pending).UnmarshalBinary(m.Count, MSBytes)
	if err != nil {
		return err
	}
	MSBytes = MSBytes[sparseHashList(m.Pending).BinarySize(m.Count):]

	// Unmarshal the hash list
	err = (*hashList)(&m.HashList).UnmarhsalBinary(MSBytes)
	if err != nil {
		return err
	}

	// Make a copy to avoid weird memory bugs
	for i, h := range m.Pending {
		m.Pending[i] = copyHash(h)
	}
	for i, h := range m.HashList {
		m.HashList[i] = copyHash(h)
	}

	return nil
}

// Add a Hash to the merkle tree and incrementally build the ChainHead
func (m *State) Add(hash_ []byte) {
	hash := copyHash(hash_)

	m.HashList = append(m.HashList, hash) // Add the new Hash to the Hash List
	m.Count++                             // Increment our total Hash Count
	m.Pad()                               // Pad Pending with a nil to remove corner cases
	for i, v := range m.Pending {         // Adding the hash is like incrementing a variable
		if v == nil { //                     Look for an empty slot, and
			m.Pending[i] = hash //               And put the Hash there if one is found
			return              //          Mission complete, so return
		}
		hash = combineHashes(v, hash) // If this slot isn't empty, combine the hash with the slot
		m.Pending[i] = nil            //   and carry the result to the next (clearing this one)
	}
}

// Anchor
// Compute the Merkle Directed Acyclic Graph (Merkle DAG or ChainHead) for
// the ChainHead at this point We take any trailing hashes in ChainHead,
// hash them up and combine to create the Merkle Dag Root. Getting the closing
// ListMDRoot is non-destructive, which is useful for some use cases.
//
// Returns a nil if the MerkleSate is empty.
func (m *State) Anchor() (anchor []byte) {
	// We go through m.ChainHead and combine any left over hashes in m.ChainHead with each other and the MR.
	// If this is a power of two, that's okay because we will pick up the MR (a balanced ChainHead) and
	// return that, the correct behavior
	if m.Count == 0 { // If the count is zero, we have no root.  Return a nil
		return anchor
	}
	for _, v := range m.Pending {
		if anchor == nil { // Pick up the first hash in m.ChainHead no matter what.
			anchor = copyHash(v) // If a nil is assigned over a nil, no harm no foul.  Fewer cases to test this way.
		} else if v != nil { // If MDRoot isn't nil and v isn't nil, combine them.
			anchor = combineHashes(v, anchor) // v is on the left, MDRoot candidate is on the right, for a new MDRoot
		}
	}
	// Drop out with a MDRoot unless m.ChainHead is zero length, in which case return a nil (correct)
	// If m.ChainHead has the entries for a power of two, then only one hash (the last) is in m.ChainHead, which
	//       is returned (correct)
	// If m.ChainHead has a railing nil, return the trailing entries combined with the last entry in m.ChainHead (correct)
	return anchor
}
