package types

import (
	"bytes"
	"fmt"
)

var zeroHash [32]byte

// String
// convert the State to a human readable string
func (m *MerkleState) String() string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("%20s %d\n", "Count", m.Count))
	b.WriteString(fmt.Sprintf("%20s %d\n", "Pending[] length:", len(m.Pending)))
	for i, v := range m.Pending {
		vp := "nil"
		if v != zeroHash {
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

// PadPending
// Make sure the Pending list ends in a nil.  This avoids some corner cases and simplifies adding elements
// to the merkle tree.  If Pending doesn't have a last entry with a nil value, then one is added.
func (m *MerkleState) PadPending() {
	PendingLen := len(m.Pending)
	if PendingLen == 0 || m.Pending[PendingLen-1] != zeroHash {
		m.Pending = append(m.Pending, zeroHash)
	}
}

// Trim
// Remove any trailing nils from Pending hashes
func (m *MerkleState) Trim() {
	for len(m.Pending) > 0 && m.Pending[len(m.Pending)-1] == zeroHash {
		m.Pending = m.Pending[:len(m.Pending)-1]
	}
}

// Pad
// Add a nil to the end of the Pending list if one isn't there.
// We need to be able to add an element to Pending if needed while
// building receipts
func (m *MerkleState) Pad() {
	if len(m.Pending) == 0 || m.Pending[len(m.Pending)-1] != zeroHash {
		m.Pending = append(m.Pending, zeroHash)
	}
}

// AddToMerkleTree
// Add a Hash to the merkle tree and incrementally build the State
func (m *MerkleState) AddToMerkleTree(hash Hash) {
	hash = hash.Copy()

	m.HashList = append(m.HashList, hash.As32()) // Add the new Hash to the Hash List
	m.Count++                                    // Increment our total Hash Count
	m.PadPending()                               // Pad Pending with a nil to remove corner cases
	for i, v := range m.Pending {                // Adding the hash is like incrementing a variable
		if v == zeroHash { //                     Look for an empty slot, and
			m.Pending[i] = hash.As32() //               And put the Hash there if one is found
			return                     //          Mission complete, so return
		}
		v := v                          // See docs/developer/rangevarref.md
		hash = Hash(v[:]).Combine(hash) // If this slot isn't empty, combine the hash with the slot
		m.Pending[i] = zeroHash         //   and carry the result to the next (clearing this one)
	}
}

// GetMDRoot
// Compute the Merkle Directed Acyclic Graph (Merkle DAG or State) for
// the State at this point We take any trailing hashes in State,
// hash them up and combine to create the Merkle Dag Root. Getting the closing
// ListMDRoot is non-destructive, which is useful for some use cases.
//
// Returns a nil if the MerkleSate is empty.
func (m *MerkleState) GetMDRoot() (MDRoot Hash) {
	// We go through m.State and combine any left over hashes in m.State with each other and the MR.
	// If this is a power of two, that's okay because we will pick up the MR (a balanced State) and
	// return that, the correct behavior
	if m.Count == 0 { // If the count is zero, we have no root.  Return a nil
		return MDRoot
	}
	for _, v := range m.Pending {
		if v == zeroHash {
			continue
		}
		v := v             // See docs/developer/rangevarref.md
		if MDRoot == nil { // Pick up the first hash in m.State no matter what.
			MDRoot = Hash(v[:]) // If a nil is assigned over a nil, no harm no foul.  Fewer cases to test this way.
		} else if v != zeroHash { // If MDRoot isn't nil and v isn't nil, combine them.
			MDRoot = Hash(v[:]).Combine(MDRoot) // v is on the left, MDRoot candidate is on the right, for a new MDRoot
		}
	}
	// Drop out with a MDRoot unless m.State is zero length, in which case return a nil (correct)
	// If m.State has the entries for a power of two, then only one hash (the last) is in m.State, which
	//       is returned (correct)
	// If m.State has a railing nil, return the trailing entries combined with the last entry in m.State (correct)
	return MDRoot
}

// PrintMR
// For debugging purposes, it is nice to get a string that shows the nil and non nil entries in c.State
// Note that the "low order" entries are first in the string, so the binary is going from low order on the left to
// high order going right in the string rather than how binary is normally represented.
func (m *MerkleState) PrintMR() (mr string) {
	for _, v := range m.Pending {
		if v != zeroHash {
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
func (m *MerkleState) GetIntermediate(h Hash, height uint64) (left, right Hash, err error) {
	m.PadPending()                // Pad Pending with a nil to remove corner cases
	for i, v := range m.Pending { // Adding the hash is like incrementing a variable
		if v == zeroHash { //               Look for an empty slot; should not encounter one
			return nil, nil, fmt.Errorf("should not encounter a nil at height %d", height)
		}
		v := v                     // See docs/developer/rangevarref.md
		if uint64(i+1) == height { // Found the height
			left = v[:]             // Get the left and right
			right = h               //
			return left, right, nil // return them
		}
		h = Hash(v[:]).Combine(h) // If this slot isn't empty, combine the hash with the slot
	}
	return nil, nil, fmt.Errorf("no values found at height %d", height)
}
