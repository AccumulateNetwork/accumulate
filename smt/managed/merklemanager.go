package managed

import (
	"errors"
	"fmt"
	"math"

	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type MerkleManager struct {
	Key       storage.Key  // ChainID (some operations require this)
	Manager   DbManager    // AppID based Manager
	MS        *MerkleState // MerkleState managed by MerkleManager
	MarkPower int64        // log2 of the MarkFreq
	MarkFreq  int64        // The count between Marks
	MarkMask  int64        // The binary mask to detect Mark boundaries
}

// AddHash adds a Hash to the Chain controlled by the ChainManager. If unique is
// true, the hash will not be added if it is already in the chain.
func (m *MerkleManager) AddHash(hash Hash, unique bool) error {
	hash = hash.Copy()                       // Just to make sure hash doesn't get changed
	_, err := m.GetElementIndex(hash)        // See if this element is a duplicate
	if errors.Is(err, storage.ErrNotFound) { // So only if the hash is not yet added to the Merkle Tree
		err = m.Manager.Int(m.Key.Append("ElementIndex", hash)).Put(m.MS.Count) // Keep its index
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if unique {
		return nil // Don't add duplicates
	}

	err = m.Manager.Hash(m.Key.Append("Element", m.MS.Count)).Put(hash)
	if err != nil {
		return err
	}
	switch (m.MS.Count + 1) & m.MarkMask {
	case 0: // Is this the end of the Mark set, i.e. 0, ..., m.MarkFreq-1
		m.MS.AddToMerkleTree(hash)                                                     // Add the hash to the Merkle Tree
		err = m.Manager.State(m.Key.Append("States", m.GetElementCount()-1)).Put(m.MS) // Save Merkle State at n*MarkFreq-1
		if err != nil {
			return err
		}
		if err := m.WriteChainHead(); err != nil {
			return fmt.Errorf("error writing chain head: %v", err)
		}
	case 1: //                              After MarkFreq elements are written
		m.MS.HashList = m.MS.HashList[:0] // then clear the HashList
		fallthrough                       // then fall through as normal
	default:
		m.MS.AddToMerkleTree(hash) // 0 to m.MarkFeq-2, always add to the merkle tree
		if err := m.WriteChainHead(); err != nil {
			return fmt.Errorf("error writing chain head: %v", err)
		}
	}

	return nil
}

// GetElementIndex
// Get an Element of a Merkle Tree from the database
func (m *MerkleManager) GetElementIndex(hash []byte) (i int64, err error) {
	i, err = m.Manager.Int(m.Key.Append("ElementIndex", hash)).Get()
	if err != nil {
		return 0, err
	}
	return i, nil
}

// SetKey
// adds the chainID to the MerkleManager for storing some chain specific key
// value pairs where the key is an index.  Before any hash is added to a
// MerkleState, the ChainID should be set
func (m *MerkleManager) SetKey(key storage.Key) (err error) {
	m.Key = key
	m.MS, err = m.ReadChainHead()
	return err
}

// WriteChainHead
// Save the current MerkleState as the head of the chain.  This does not flush
// the database, because all chains in an application need have their head of
// chain updated as one atomic transaction to prevent messing up the database
func (m *MerkleManager) WriteChainHead() error {
	return m.Manager.State(m.Key.Append("Head")).Put(m.MS)
}

// GetChainState
// Reads the highest state of the chain stored to the database.  Returns nilTestCli
// if no state has been recorded for a chain
func (m *MerkleManager) GetChainState() (merkleState *MerkleState, err error) {
	return m.Manager.State(m.Key.Append("Head")).Get()
}

// ReadChainHead
// Retrieve the current MerkleState from the given database, and set
// that state as the MerkleState of this MerkleManager.  Note that the cache
// will be cleared.
func (m *MerkleManager) ReadChainHead() (ms *MerkleState, err error) {
	ms, err = m.Manager.State(m.Key.Append("Head")).Get() //   Get the state for the Merkle Tree
	switch {
	case err == nil:
		return ms, nil
	case errors.Is(err, storage.ErrNotFound):
		ms = new(MerkleState)
		return ms, nil
	default:
		return nil, err
	}
}

// Equal
// Compares the MerkleManager to the given MerkleManager and returns false if
// the fields in the MerkleManager are different from m2
func (m *MerkleManager) Equal(m2 *MerkleManager) bool {
	// if !m.Manager.Equal(m2.Manager) {
	// 	return false
	// }
	if !m.MS.Equal(m2.MS) {
		return false
	}
	if m.MarkPower != m2.MarkPower {
		return false
	}
	if m.MarkFreq != m2.MarkFreq {
		return false
	}
	if m.MarkMask != m2.MarkMask {
		return false
	}
	return true
}

// NewMerkleManager
// Create a new MerkleManager given a MainChain.Manager and markPower.
// The MainChain.Manager handles the persistence for the Merkle Tree under management
// The markPower is the log 2 frequency used to collect states in the Merkle Tree
func NewMerkleManager(
	DBManager DbManager, //      database that can be shared with other MerkleManager instances
	markPower int64) (*MerkleManager, error) { // log 2 of the frequency of creating marks in the Merkle Tree

	mm := new(MerkleManager)
	if err := mm.init(DBManager, markPower); err != nil {
		return nil, err
	}

	return mm, nil
}

// ManageChain returns a copy of the manager with the key set to the given
// value.
func (m *MerkleManager) ManageChain(key storage.Key) (*MerkleManager, error) {
	n := new(MerkleManager)
	err := n.init(m.Manager, m.MarkPower)
	if err != nil {
		return nil, err
	}

	err = n.SetKey(key)
	if err != nil {
		return nil, err
	}

	return n, nil
}

// GetElementCount
// Return number of elements in the Merkle Tree managed by this MerkleManager
func (m *MerkleManager) GetElementCount() (elementCount int64) {
	return m.MS.Count
}

// init
// Create a Merkle Tree manager to collect hashes and build a Merkle Tree and a
// database behind it to validate hashes and create receipts.
//
// This is an internal routine; calling init outside of constructing the first
// reference to the MerkleManager doesn't make much sense.
func (m *MerkleManager) init(DBManager DbManager, markPower int64) (err error) {

	if markPower >= 20 { // 2^20 is 1,048,576 and is too big for the collection of elements in memory
		return fmt.Errorf("A power %d is greater than 2^29, and is unreasonable", markPower)
	}
	m.Manager = DBManager // Manager for writing the Merkle states
	if m.MS == nil {      // Allocate an MS if we don't have one
		m.MS = new(MerkleState) //
		m.MS.InitSha256()       //
	}
	m.MS, err = m.ReadChainHead() //                  Set the MerkleState
	if err != nil {
		return err
	}
	m.MarkPower = markPower                             // # levels in Merkle Tree to be indexed
	m.MarkFreq = int64(math.Pow(2, float64(markPower))) // The number of elements between indexes
	m.MarkMask = m.MarkFreq - 1                         // Mask to index of next mark (0 if at a mark)

	return nil
}

// GetState
// Query the database for the MerkleState for a given index, i.e. the state
// Note that not every element in the Merkle Tree has a stored state;
// states are stored at the frequency indicated by the Mark Power.  We also
// store the state of the chain at the end of a block regardless, but this
// state overwrites the previous block state.
//
// If no state exists in the database for the element, GetState returns nil
func (m *MerkleManager) GetState(element int64) *MerkleState {
	if m.GetElementCount() == 0 {
		ms := new(MerkleState)
		if eHash, err := m.Get(element); err != nil {
			ms.AddToMerkleTree(eHash)
		}
		ms.InitSha256()
		return ms
	}

	ms, e := m.Manager.State(m.Key.Append("States", element)).Get() // Get the data at this height
	if e != nil {                                                   // If nil, there is no state saved
		return nil //                                                   return nil, as no state exists
	}
	return ms // return it
}

// GetAnyState
// We only store the state at MarkPoints.  This function computes a missing
// state even if one isn't stored for a particular element.
func (m *MerkleManager) GetAnyState(element int64) (ms *MerkleState, err error) {
	if ms = m.GetState(element); ms != nil { //          Shoot for broke. Return a state if it is in the db
		return ms, nil
	}
	if element >= m.GetElementCount() { //               Check to make sure element is not outside bounds
		return nil, errors.New("element out of range")
	}
	MIPrev := element&(^m.MarkMask) - 1 //               Calculate the index of the prior markpoint
	cState := m.GetState(MIPrev)        //               Use state at the prior mark point to compute what we need
	if MIPrev < 0 {
		cState = new(MerkleState)
		cState.InitSha256()
	}
	if cState == nil { //                                Should be in the database.
		return nil, errors.New( //                        Report error if it isn't in the database'
			"should have a state for all elements(1)")
	}
	cState.HashList = cState.HashList[:0] //             element is past the previous mark, so clear the HashList

	MINext := element&(^m.MarkMask) - 1 + m.MarkFreq //            Calculate the following mark point
	var NMark *MerkleState                           //
	if MINext >= m.GetElementCount() {               //             If past the end of the chain, then
		if NMark, err = m.GetChainState(); err != nil { //        read the chain state instead
			return nil, err //                                        Should be in the database
		}
	} else {
		if NMark = m.GetState(MINext); NMark == nil { //             Read the mark point
			return nil, errors.New("mark not found in the database")
		}
	}
	for _, v := range NMark.HashList { //                           Now iterate and add to the cState
		if element+1 == cState.Count { //                              until the loop adds the element
			break
		}
		cState.AddToMerkleTree(v)
	}
	if cState.Count&m.MarkMask == 0 { //                           If we progress out of the mark set,
		cState.HashList = cState.HashList[:0] //                       start over collecting hashes.
	}
	return cState, nil
}

// Get the nth leaf node
func (m *MerkleManager) Get(element int64) (Hash, error) {
	data, err := m.Manager.Hash(m.Key.Append("Element", element)).Get()
	if err != nil {
		return nil, err
	}

	return data.Copy(), nil
}

// GetIntermediate
// Return the last two hashes that were combined to create the local
// Merkle Root at the given index.  The element provided must be odd,
// and the Pending List must be fully populated up to height specified.
func (m *MerkleManager) GetIntermediate(element, height int64) (Left, Right Hash, err error) {
	hash, e := m.Get(element) // Get the element at this height
	if e != nil {             // Error out if we can't
		return nil, nil, e //
	} //
	s, e2 := m.GetAnyState(element - 1) // Get the state before the state we want
	if e2 != nil {                      // If the element doesn't exist, that's a problem
		return nil, nil, e2 //
	} //
	return s.GetIntermediate(hash, height)
}
