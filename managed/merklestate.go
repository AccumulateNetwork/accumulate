package managed

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"github.com/AccumulateNetwork/SMT/storage"
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
	HashFunction func(data []byte) Hash // Hash function for this Merkle State
	Count        int64                  // Count of hashes added to the Merkle tree
	Pending      []*Hash                // Array of hashes that represent the left edge of the Merkle tree
	HashList     []Hash                 // List of Hashes in the order added to the chain
}

// String
// convert the MerkleState to a human readable string
func (m MerkleState) String() string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("%20s %d %x\n", "Count", m.Count, m.Count))
	b.WriteString(fmt.Sprintf("%20s %d\n", "Pending[] length:", len(m.Pending)))
	for i, v := range m.Pending {
		vp := "nil"
		if v != nil {
			vp = fmt.Sprintf("%x", *v)
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
// Make a completely independent copy of the Merkle State that removes all references to
// structures in the given Merkle State.  This means copying any entries in the Pending slice
func (m MerkleState) Copy() MerkleState {
	return *m.CopyAndPoint()
}

// CopyAndPoint
// Make a completely independent copy of the Merkle State that removes all references to
// structures in the given Merkle State.  This means copying any entries in the Pending slice
func (m MerkleState) CopyAndPoint() *MerkleState {
	// Must make a new slice for ms.Pending
	m.Pending = append(m.Pending[:0], m.Pending...)
	// Extra paranoid, make the hashes new hashes in pending
	// (nobody should change a hash, but even if they do the copy will be independent
	for i, v := range m.Pending {
		if v != nil {
			var hash = *v
			m.Pending[i] = &hash
		}
	}
	return &m
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

// Equal
// Compares one MerkleState to another, and returns true if they are the same
func (m MerkleState) Equal(m2 MerkleState) (isEqual bool) {
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

	// The Count drives what is compared, so even if one or the other Pending has a trailing nil, it won't
	// matter; the bit won't be set.  (A trailing nil is possible as a side effect of an odd number of elements
	// in the merkle tree).
	cnt := m.Count             // Only check that we have entries in Pending that have a bit set in the Count.
	for i := 0; cnt > 0; i++ { // When cnt goes to zero, we are done, even if a nil might exist in the Pending array
		// If the Merkle State has a nil, m2 must have a nil.  If we later compare m to m2 and m2 has a nil,
		// we will panic. That's okay, because we catch it and declare them unequal.
		if m.Pending[i] == nil && m2.Pending[i] != nil {
			return false
		}

		// Each element of Pending must be equal
		if cnt&1 == 1 && !bytes.Equal(m.Pending[i][:], m2.Pending[i][:]) {
			return false
		}
		cnt = cnt >> 1 // Shift a bit out of Count; When cnt is zero, we are done.
	}

	// Each must have the same number of elements in the HashList
	if len(m.HashList) != len(m2.HashList) {
		return false
	} else {
		// Each element in the HashLists must be equal
		for i, v := range m.HashList {
			if !bytes.Equal(v[:], m2.HashList[i][:]) {
				return false
			}
		}
	}
	// If we made it here, all is golden.
	return true
}

// Marshal
// Encodes the Merkle State so it can be embedded into the Merkle Tree
func (m *MerkleState) Marshal() (MSBytes []byte) {
	MSBytes = append(MSBytes, storage.Int64Bytes(m.Count)...) // Count
	cnt := m.Count                                            // Each bit set in Count, indicates a Sub Merkle Tree root
	for i := 0; cnt > 0; i++ {                                // For each bit in cnt,
		if cnt&1 > 0 { //                                         if the bit is set in cnt, record the hash
			MSBytes = append(MSBytes, m.Pending[i][:]...)
		} //                                                   If the bit is not set, ignore (it is nil anyway)
		cnt = cnt >> 1 //                                      Shift cnt so we can check the next bit
	}
	MSBytes = append(MSBytes, storage.Int64Bytes(int64(len(m.HashList)))...) // Write out the HashList
	for _, v := range m.HashList {                                           // For every Hash
		MSBytes = append(MSBytes, v[:]...) // Add it to MSBytes
	}
	return MSBytes
}

// UnMarshal
// Take the state of an MSMarshal instance defined by MSBytes, and set all the values
// in this instance of MSMarshal to the state defined by MSBytes.  It is assumed that the
// hash function has been set by the caller.
func (m *MerkleState) UnMarshal(MSBytes []byte) {
	m.Count, MSBytes = storage.BytesInt64(MSBytes) // Extract the Count
	m.Pending = m.Pending[:0]                      // Set Pending to zero, then use the bits of Count
	cnt := m.Count                                 //   to guide the extraction of the List of Sub Merkle State roots
	for i := 0; cnt > 0; i++ {                     // To do this, go through the count
		m.Pending = append(m.Pending, nil) //         Make a spot, which will leave nil if the bit in count is zero
		if cnt&1 > 0 {                     //         If the bit is set, then extract the next hash and put it here
			m.Pending[i] = new(Hash)            //    Add the storage for the hash
			copy(m.Pending[i][:], MSBytes[:32]) //    Copy in its value
			MSBytes = MSBytes[32:]              //    And advance MSBytes by the hash size
		}
		cnt = cnt >> 1 //                             Shift cnt to the right to look at the next bit
	}

	m.HashList = m.HashList[:0]                    // Clear any possible existing HashList
	length, MSBytes := storage.BytesInt64(MSBytes) // Extract the length of the HashList
	for i := int64(0); i < length; i++ {           // Extract each Hash
		m.HashList = append(m.HashList, Hash{}) //    Add the storage for the Hash, then
		copy(m.HashList[i][:], MSBytes[:32])    //      copy over its value
		MSBytes = MSBytes[32:]                  //    Advance MSBytes by the hash size
	}
}

// GetSha256
// Get the a Sha256 function that can be used to create hashes compatible with a Stateful
// Merkle tree using sha256
func GetSha256() func(data []byte) Hash {
	return func(data []byte) Hash { return sha256.Sum256(data) }
}

// InitSha256
// Set the hashing function of this Merkle State to Sha256
// TODO: Actually update the library to be able to use various hash algorithms
func (m *MerkleState) InitSha256() {
	m.HashFunction = GetSha256()
}

// AddToMerkleTree
// Add a Hash to the merkle tree and incrementally build the MerkleState
func (m *MerkleState) AddToMerkleTree(hash_ [32]byte) {
	hash := Hash(hash_)

	m.HashList = append(m.HashList, hash) // Add the new Hash to the Hash List
	m.Count++                             // Increment our total Hash Count
	m.PadPending()                        // Pad Pending with a nil to remove corner cases
	for i, v := range m.Pending {         // Adding the hash is like incrementing a variable
		if v == nil { //                     Look for an empty slot, and
			m.Pending[i] = &hash //               And put the Hash there if one is found
			return               //          Mission complete, so return
		}
		hash = v.Combine(m.HashFunction, hash) // If this slot isn't empty, combine the hash with the slot
		m.Pending[i] = nil                     //   and carry the result to the next (clearing this one)
	}
}

// GetMDRoot
// Compute the Merkle Directed Acyclic Graph (Merkle DAG or MerkleState) for
// the MerkleState at this point We take any trailing hashes in MerkleState,
// hash them up and combine to create the Merkle Dag Root. Getting the closing
// ListMDRoot is non-destructive, which is useful for some use cases.
func (m *MerkleState) GetMDRoot() (MDRoot *Hash) {
	// We go through m.MerkleState and combine any left over hashes in m.MerkleState with each other and the MR.
	// If this is a power of two, that's okay because we will pick up the MR (a balanced MerkleState) and
	// return that, the correct behavior
	for _, v := range m.Pending {
		if MDRoot == nil { // Pick up the first hash in m.MerkleState no matter what.
			MDRoot = v // If a nil is assigned over a nil, no harm no foul.  Fewer cases to test this way.
		} else if v != nil { // If MDRoot isn't nil and v isn't nil, combine them.
			combine := v.Combine(m.HashFunction, *MDRoot) // v is on the left, MDRoot candidate is on the right, for a new MDRoot
			MDRoot = &combine
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
