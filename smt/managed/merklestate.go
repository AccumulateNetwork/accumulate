package managed

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/encoding"
	"gitlab.com/accumulatenetwork/accumulate/smt/common"
)

// MerkleState
// A Merkle Dag State is the state kept while building a Merkle Tree.  Except where a Merkle Tree has a clean
// power of two number of elements as leaf nodes, there will be multiple Sub Merkle Trees that make up a
// dynamic Merkle Tree. The Merkle State is the list of the roots of these sub Merkle Trees, and the
// combination of these roots provides a Directed Acyclic Graph (DAG) to all the leaves.
//
//                                                       Merkle State
//  1  2   3  4   5  6   7  8   9 10  11 12  13 --->         13
//   1-2    3-4    5-6    7-8   0-10  11-12     --->         --
//      1-2-3-4       5-6-7-8    0-10-11-12     --->     0-10-11-12
//            1-2-3-4-5-6-7-8                   --->   1-2-3-4-5-6-7-8
//
// Interestingly, the state of building such a Merkle Tree looks just like counting in binary.  And the
// higher order bits set will correspond to where the binary roots must be kept in a Merkle state.
type MerkleState struct {
	Count    int64          // Count of hashes added to the Merkle tree
	Pending  SparseHashList // Array of hashes that represent the left edge of the Merkle tree
	HashList HashList       // List of Hashes in the order added to the chain
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
func (m MerkleState) Copy() *MerkleState {
	m.Pending = append(SparseHashList{}, m.Pending...) // New slice for Pending, but hashes are immutable
	// Extra paranoid, make the hashes new hashes in pending
	// (nobody should change a hash, but even if they do the copy will be independent
	for i, v := range m.Pending {
		if v != nil {
			m.Pending[i] = Hash(v).Copy()
		}
	}
	m.HashList = append([]Hash{}, m.HashList...) // copy the underlying storage under slice
	return &m
}

func (m *MerkleState) MarshalBinary() ([]byte, error) {
	return m.Marshal()
}

func (m *MerkleState) UnmarshalBinary(data []byte) error {
	return m.UnMarshal(data)
}

func (m *MerkleState) UnmarshalBinaryFrom(rd io.Reader) error {
	data, err := io.ReadAll(rd)
	if err != nil {
		return err
	}
	return m.UnMarshal(data)
}

func (m *MerkleState) CopyAsInterface() interface{} {
	return m.Copy()
}

// PadPending
// Make sure the Pending list ends in a nil.  This avoids some corner cases and simplifies adding elements
// to the merkle tree.  If Pending doesn't have a last entry with a nil value, then one is added.
func (m *MerkleState) PadPending() {
	PendingLen := len(m.Pending)
	if PendingLen == 0 || m.Pending[PendingLen-1] != nil {
		m.Pending = append(m.Pending, nil)
	}
}

// Trim
// Remove any trailing nils from Pending hashes
func (m *MerkleState) Trim() {
	for len(m.Pending) > 0 && m.Pending[len(m.Pending)-1] == nil {
		m.Pending = m.Pending[:len(m.Pending)-1]
	}
}

// Pad
// Add a nil to the end of the Pending list if one isn't there.
// We need to be able to add an element to Pending if needed while
// building receipts
func (m *MerkleState) Pad() {

	if len(m.Pending) == 0 || m.Pending[len(m.Pending)-1] != nil {
		m.Pending = append(m.Pending, nil)
	}
}

// Equal
// Compares one MerkleState to another, and returns true if they are the same
func (m *MerkleState) Equal(m2 *MerkleState) (isEqual bool) {
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

// Marshal
// Encodes the Merkle State so it can be embedded into the Merkle Tree
func (m *MerkleState) Marshal() (MSBytes []byte, err error) {
	MSBytes = append(MSBytes, common.Int64Bytes(m.Count)...) // Count

	// Write out the pending list. Each bit set in Count indicates a Sub Merkle
	// Tree root. For each bit, if the bit is set, record the hash.
	b, err := m.Pending.MarshalBinary(m.Count)
	if err != nil {
		return nil, err
	}
	MSBytes = append(MSBytes, b...)

	// Write out the hash list (never returns an error)
	b, _ = m.HashList.MarshalBinary()
	MSBytes = append(MSBytes, b...)
	return MSBytes, nil
}

// UnMarshal
// Take the state of an MSMarshal instance defined by MSBytes, and set all the values
// in this instance of MSMarshal to the state defined by MSBytes.  It is assumed that the
// hash function has been set by the caller.
func (m *MerkleState) UnMarshal(MSBytes []byte) (err error) {
	// Unmarshal the Count
	m.Count, err = encoding.VarintUnmarshalBinary(MSBytes)
	if err != nil {
		return err
	}
	MSBytes = MSBytes[encoding.VarintBinarySize(m.Count):]

	// Unmarshal the pending list
	err = m.Pending.UnmarshalBinary(m.Count, MSBytes)
	if err != nil {
		return err
	}
	MSBytes = MSBytes[m.Pending.BinarySize(m.Count):]

	// Unmarshal the hash list
	err = m.HashList.UnmarhsalBinary(MSBytes)
	if err != nil {
		return err
	}

	// Make a copy to avoid weird memory bugs
	for i, h := range m.Pending {
		m.Pending[i] = Hash(h).Copy()
	}
	for i, h := range m.HashList {
		m.HashList[i] = h.Copy()
	}

	return nil
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
	hash := Hash(hash_).Copy()

	m.HashList = append(m.HashList, hash) // Add the new Hash to the Hash List
	m.Count++                             // Increment our total Hash Count
	m.PadPending()                        // Pad Pending with a nil to remove corner cases
	for i, v := range m.Pending {         // Adding the hash is like incrementing a variable
		if v == nil { //                     Look for an empty slot, and
			m.Pending[i] = hash //               And put the Hash there if one is found
			return              //          Mission complete, so return
		}
		hash = Hash(v).Combine(Sha256, hash) // If this slot isn't empty, combine the hash with the slot
		m.Pending[i] = nil                   //   and carry the result to the next (clearing this one)
	}
}

// GetMDRoot
// Compute the Merkle Directed Acyclic Graph (Merkle DAG or MerkleState) for
// the MerkleState at this point We take any trailing hashes in MerkleState,
// hash them up and combine to create the Merkle Dag Root. Getting the closing
// ListMDRoot is non-destructive, which is useful for some use cases.
//
// Returns a nil if the MerkleSate is empty.
func (m *MerkleState) GetMDRoot() (MDRoot Hash) {
	// We go through m.MerkleState and combine any left over hashes in m.MerkleState with each other and the MR.
	// If this is a power of two, that's okay because we will pick up the MR (a balanced MerkleState) and
	// return that, the correct behavior
	if m.Count == 0 { // If the count is zero, we have no root.  Return a nil
		return MDRoot
	}
	for _, v := range m.Pending {
		if MDRoot == nil { // Pick up the first hash in m.MerkleState no matter what.
			MDRoot = Hash(v).Copy() // If a nil is assigned over a nil, no harm no foul.  Fewer cases to test this way.
		} else if v != nil { // If MDRoot isn't nil and v isn't nil, combine them.
			MDRoot = Hash(v).Combine(Sha256, MDRoot) // v is on the left, MDRoot candidate is on the right, for a new MDRoot
		}
	}
	// Drop out with a MDRoot unless m.MerkleState is zero length, in which case return a nil (correct)
	// If m.MerkleState has the entries for a power of two, then only one hash (the last) is in m.MerkleState, which
	//       is returned (correct)
	// If m.MerkleState has a railing nil, return the trailing entries combined with the last entry in m.MerkleState (correct)
	return MDRoot
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
	m.PadPending()                // Pad Pending with a nil to remove corner cases
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
