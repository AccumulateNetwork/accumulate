package smt

import (
	"bytes"
	"crypto/sha256"
)

// MerkleState
// A Merkle Dag State is the state kept while building a Merkle Tree.  Except where a Merkle Tree has a clean
// power of two number of elements as leaf nodes, there will be multiple roots.  The combination of these
// roots provides a Directed Acyclic Graph (DAG) to all the leaves.
//
// Interestingly, the state of building such a Merkle Tree looks just like counting in binary.  And the
// higher order bits set will correspond to where the binary roots must be kept in a Merkle state.
type MerkleState struct {
	HashFunction func(data []byte) Hash

	count int64 // Count of hashes added to the Merkle tree
	// Note that if the count is zero, there is no previous Hash
	previous Hash    // Hash of the previous MerkleState added to the Merkle Tree
	Pending  []*Hash // Array of hashes that represent the left edge of the Merkle tree
	HashList []Hash  // List of Hashes in the order added to the chain
}

// Copy
// Make a completely independent copy of the Merkle State that removes all references to
// structures in the given Merkle State.  This means copying any entries in the Pending slice
func (m *MerkleState) Copy() *MerkleState {
	ms := *m
	// Must make a new slice for ms.Pending
	ms.Pending = append(ms.Pending[:0], m.Pending...)
	// Extra paranoid, make the hashes new hashes in pending
	// (nobody should change a hash, but even if they do the copy will be independent
	for i, v := range ms.Pending {
		if v != nil {
			var hash = *v
			ms.Pending[i] = &hash
		}
	}
	return &ms
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

// GetCount
// Get the count of elements in the Merkle Tree
func (m MerkleState) GetCount() int64 {
	return m.count
}

// GetPrevious
// Get the hash of the Previous MerkleState
func (m MerkleState) GetPrevious() Hash {
	return m.previous
}

// GetCurrentCount
// Get the count of the hashes added in the current Merkle Block
func (m MerkleState) GetCurrentCount() int64 {
	return int64(len(m.HashList))
}

// Equal
// Compares one MerkleState to another, and returns true if they are the same
func (m MerkleState) Equal(m2 MerkleState) (errorFlag bool) {
	// Any errors indicate at m is not the same as m2, or either m or m2 or both is malformed.
	defer func() {
		if recover() != nil {
			errorFlag = false
			return
		}
	}()
	// Count is how many elements in the merkle tree.  should be the same
	if m.count != m2.count {
		return false
	}

	// Check previous hash, which should be the same.
	if m.previous != m2.previous {
		return false
	}

	// The count drives what is compared, so even if one or the other Pending has a trailing nil, it won't
	// matter; the bit won't be set.  (A trailing nil is possible as a side effect of an odd number of elements
	// in the merkle tree).
	cnt := m.count             // Only check that we have entries in Pending that have a bit set in the count.
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
		cnt = cnt >> 1 // Shift a bit out of count; When cnt is zero, we are done.
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
	// Add the count of all the hashes in the merkle tree to MSBytes
	MSBytes = append(MSBytes, Int64Bytes(m.count)...)
	if m.count != 0 { // If count is zero, there is no previous Merkle State sono previous to unpack
		// Add previous to MSBytes
		MSBytes = append(MSBytes, m.previous[:]...)
	}
	// Add the Pending hashes to MSBytes; note we only have hashes where bits in count are set
	cnt := m.count             // Get the count of elements in the Merkle Tree
	for i := 0; cnt > 0; i++ { // For each bit in cnt,
		if cnt&1 > 0 { // if the bit is set in cnt, record the hash
			MSBytes = append(MSBytes, m.Pending[i][:]...)
		} // if the bit is not set, ignore (it is nil anyway)
		cnt = cnt >> 1 // Shift cnt so we can check the next bit
	}
	// Add the HashList to MSBytes.  First record the number of hashes added since last Merkle State
	MSBytes = append(MSBytes, Int64Bytes(int64(len(m.HashList)))...)
	// Now add all the Hashes in the HashList to MSBytes
	for _, v := range m.HashList {
		MSBytes = append(MSBytes, v[:]...)
	}

	return MSBytes
}

// UnMarshal
// Take the state of an MSMarshal instance defined by MSBytes, and set all the values
// in this instance of MSMarshal to the state defined by MSBytes.  It is assumed that the
// hash function has been set by the caller.
func (m *MerkleState) UnMarshal(MSBytes []byte) {
	// Extract the count of hashes in the Merkle Tree from MSBytes
	m.count, MSBytes = BytesInt64(MSBytes)
	// Remember, if m.count is zero, there is no previous Merkle State to unpack
	if m.count != 0 {
		// Extract the previous hash from MSBytes
		copy(m.previous[:], MSBytes[:32])
		MSBytes = MSBytes[32:]
	}

	// We are getting a new Pending Array, so clear the old one.
	m.Pending = m.Pending[:0]
	// Extract the Pending roots array from MSBytes; not only where bits in count are set do we have a value in Pending
	cnt := m.count
	for i := 0; cnt > 0; i++ {
		// Add an nil element to Pending (for both if this bit in cnt is set or not
		m.Pending = append(m.Pending, nil)
		// If the cnt bit is set, we need to grab the hash out of MSBytes
		if cnt&1 > 0 {
			// Make this entry in Pending point to a hash
			m.Pending[i] = new(Hash)
			// Set the Hash to the value specified in MSBytes
			copy(m.Pending[i][:], MSBytes[:32])
			// Now skip MSBytes past the value we just copied to Pending
			MSBytes = MSBytes[32:]
		}
		cnt = cnt >> 1
	}

	// Make sure this merkle state has a blank HashList, because we are replacing it with a new one.
	m.HashList = m.HashList[:0]
	// Extract the length of the HashList
	var length int64
	length, MSBytes = BytesInt64(MSBytes)
	// For the length of the HashList, extract each Hash
	for i := int64(0); i < length; i++ {
		// First make room for one more Hash
		m.HashList = append(m.HashList, Hash{})
		// Copy over the Hash value
		copy(m.HashList[i][:], MSBytes[:32])
		// Move MSBytes over to the next hash value
		MSBytes = MSBytes[32:]
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
func (m *MerkleState) InitSha256() {
	m.HashFunction = GetSha256()
}

// AddToMerkleTree
// Add a Hash to the chain and incrementally build the MerkleState
func (m *MerkleState) AddToMerkleTree(hash_ [32]byte) {
	hash := Hash(hash_)
	// We are going through through the MerkleState list and combining hashes; we have to record the hash first thing
	m.HashList = append(m.HashList, hash) // before it is combined with other hashes already added to MerkleState[].
	m.count++                             // Add one to our count of elements added to the Merkle State

	// We make sure m.MerkleState ends with a nil entry, because that cuts out most of the corner cases in adding hashes
	m.PadPending()

	// Okay, now we go through m.Pending and look for the first nil entry in Pending and add our hash there. Along the
	// way, we take every non-vil entry and combine it with the hash we are adding. Note we ALWAYS have a nil at the
	// end of m.MerkleState so we don't have a end case to deal with.
	for i, v := range m.Pending {

		// Look and see if the current spot in MerkleState is open.
		if v == nil { // If it is open, put our hash here and continue.
			m.Pending[i] = &hash // put a pointer to a copy of hash into m.MerkleState
			return               // If we have added the hash to m.MerkleState then we are done.
		}

		// If the current spot is NOT open, we need to combine the hash we have with the hash on the "left", i.e.
		// the hash already in m.Pending
		hash = v.Combine(m.HashFunction, hash) // Combine v (left) and hash (right) to get a new combined hash to use forward
		m.Pending[i] = nil                     // Now that we have combined v and hash, this spot is now empty, so clear it.
	}
}

// GetMDRoot
// Close off the Merkle Directed Acyclic Graph (Merkle DAG or MerkleState)
// We take any trailing hashes in MerkleState, hash them up and combine to create the Merkle Dag Root.
// Getting the closing ListMDRoot is non-destructive, which is useful for some use cases.
func (m *MerkleState) GetMDRoot() (MDRoot *Hash) {
	// We go through m.MerkleState and combine any left over hashes in m.MerkleState with each other and the MR.
	// If this is a power of two, that's okay because we will pick up the MR (a balanced MerkleState) and
	// return that, the correct behavior
	for _, v := range m.Pending {
		if MDRoot == nil { // We will pick up the first hash in m.MerkleState no matter what.
			MDRoot = v // If we assign a nil over a nil, no harm no foul.  Fewer cases to test this way.
		} else if v != nil { // If MDRoot isn't nil and v isn't nil, we combine them.
			combine := v.Combine(m.HashFunction, *MDRoot) // v is on the left, MDRoot candidate is on the right, for a new MDRoot
			MDRoot = &combine
		}
	}
	// We drop out with a MDRoot unless m.MerkleState is zero length, in which case we return a nil (correct)
	// If m.MerkleState has the entries for a power of two, then only one hash (the last) is in m.MerkleState, which we return (correct)
	// If m.MerkleState has a railing nil, we return the trailing entries combined with the last entry in m.MerkleState (correct)
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

// EndBlock
// Data is added to a Merkle State over time. Entries can be grouped into Merkle Blocks.  Each Merkle Block
// begins with the Merkle State describing the state of the Merkle Tree prior to that Merkle Block.  Each
// Merkle block holds the hash of the previous Merkle State.
//
// Ending a Merkle Block means adding the Current Merkle State to the Merkle Tree.  So every Merkle Block
// begins with the hash of the Merkle State of the end of the previous Merkle Block.
//
// NOTE: The caller is responsible for being able to match the Merkle State to the block by putting it in a
// database or memory map.
func (m *MerkleState) EndBlock() (MSBytes []byte, hash Hash) {
	MSBytes = m.Marshal()          // Get the serialization of the current Merkle State
	hash = m.HashFunction(MSBytes) // Get the hash of last Merkle State of the current Merkle Block
	m.HashList = m.HashList[:0]    // The HashList is just the hashes in the Merkle Block, so Clear that.
	m.previous = hash              // Point the previous hash to the Merkle State of the new previous Merkle Block
	return MSBytes, hash
}
