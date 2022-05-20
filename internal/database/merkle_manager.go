package database

import (
	"math"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

type Hash = types.Hash
type MerkleState = types.MerkleState

type MerkleManager struct {
	batch     *Batch       // Database batch
	key       storage.Key  // Base database key
	state     *MerkleState // Merkle state
	MarkPower uint64       // log2 of the MarkFreq
	MarkFreq  uint64       // The count between Marks
	MarkMask  uint64       // The binary mask to detect Mark boundaries
}

func newfn[T any]() func() *T {
	return func() *T { return new(T) }
}

func (m *MerkleManager) intValue(key ...interface{}) *ValueAs[*intValue] {
	return NewValue(m.batch, newfn[intValue](), m.key.Append(key...))
}

func (m *MerkleManager) hashValue(key ...interface{}) *ValueAs[*hashValue] {
	return NewValue(m.batch, newfn[hashValue](), m.key.Append(key...))
}

func (m *MerkleManager) stateValue(key ...interface{}) *ValueAs[*MerkleState] {
	return NewValue(m.batch, newfn[MerkleState](), m.key.Append(key...))
}

// AddHash adds a Hash to the Chain controlled by the ChainManager. If unique is
// true, the hash will not be added if it is already in the chain.
func (m *MerkleManager) AddHash(hash Hash, unique bool) error {
	hash = hash.Copy()                            // Just to make sure hash doesn't get changed
	elemIndex := m.intValue("ElementIndex", hash) //
	_, err := elemIndex.Get()                     // See if this element is a duplicate
	switch {                                      // So only if the hash is not yet added to the Merkle Tree
	case errors.Is(err, errors.StatusNotFound):
		err = elemIndex.Put(&intValue{Value: m.state.Count}) // Keep its index
		if err != nil {
			return errors.Format(errors.StatusUnknown, "store element index: %w", err)
		}
	case err != nil:
		return errors.Format(errors.StatusUnknown, "load element index: %w", err)
	case unique:
		return nil // Don't add duplicates
	}

	err = m.hashValue("Element", m.state.Count).Put(&hashValue{Value: hash.As32()})
	if err != nil {
		return errors.Format(errors.StatusUnknown, "load element: %w", err)
	}

	switch (m.state.Count + 1) & m.MarkMask {
	case 0: // Is this the end of the Mark set, i.e. 0, ..., m.MarkFreq-1
		m.state.AddToMerkleTree(hash)                              // Add the hash to the Merkle Tree
		err = m.stateValue("States", m.state.Count-1).Put(m.state) // Save Merkle State at n*MarkFreq-1
		if err != nil {
			return errors.Format(errors.StatusUnknown, "store mark point: %w", err)
		}

		err = m.WriteChainHead()
		if err != nil {
			return errors.Format(errors.StatusUnknown, "store chain head: %w", err)
		}

	case 1: //                              After MarkFreq elements are written
		m.state.HashList = m.state.HashList[:0] // then clear the HashList
		fallthrough                             // then fall through as normal

	default:
		m.state.AddToMerkleTree(hash) // 0 to m.MarkFeq-2, always add to the merkle tree
		err = m.WriteChainHead()
		if err != nil {
			return errors.Format(errors.StatusUnknown, "store chain head: %w", err)
		}
	}

	return nil
}

// GetElementIndex
// Get an Element of a Merkle Tree from the database
func (m *MerkleManager) GetElementIndex(hash Hash) (uint64, error) {
	v, err := m.intValue("ElementIndex", hash).Get()
	if err != nil {
		return 0, errors.Wrap(errors.StatusUnknown, err)
	}
	return v.Value, nil
}

// WriteChainHead
// Save the current State as the head of the chain.  This does not flush
// the database, because all chains in an application need have their head of
// chain updated as one atomic transaction to prevent messing up the database
func (m *MerkleManager) WriteChainHead() error {
	return m.stateValue("Head").Put(m.state)
}

// GetChainState
// Reads the highest state of the chain stored to the   Returns nilTestCli
// if no state has been recorded for a chain
func (m *MerkleManager) GetChainState() (*MerkleState, error) {
	return m.stateValue("Head").Get()
}

// ReadChainHead
// Retrieve the current State from the given database, and set
// that state as the State of this Manager.  Note that the cache
// will be cleared.
func (m *MerkleManager) ReadChainHead() (*MerkleState, error) {
	return m.stateValue("Head").Get()
}

// Equal
// Compares the Manager to the given Manager and returns false if
// the fields in the Manager are different from m2
func (m *MerkleManager) Equal(m2 *MerkleManager) bool {
	if !m.state.Equal(m2.state) {
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

// NewManager
// Create a new Manager given a MainChain.Manager and markPower.
// The MainChain.Manager handles the persistence for the Merkle Tree under management
// The markPower is the log 2 frequency used to collect states in the Merkle Tree
func NewManager(
	batch *Batch, // database that can be shared with other Manager instances
	key storage.Key, //       Base key for records
	markPower uint64, //      log 2 of the frequency of creating marks in the Merkle Tree
) (*MerkleManager, error) {
	mm := new(MerkleManager)
	if err := mm.init(batch, markPower, key); err != nil {
		return nil, err
	}

	return mm, nil
}

// ManageChain returns a copy of the manager with the key set to the given
// value.
func (m *MerkleManager) ManageChain(key storage.Key) (*MerkleManager, error) {
	n := new(MerkleManager)
	err := n.init(m.batch, m.MarkPower, key)
	if err != nil {
		return nil, err
	}

	return n, nil
}

// GetElementCount
// Return number of elements in the Merkle Tree managed by this Manager
func (m *MerkleManager) GetElementCount() (elementCount uint64) {
	return m.state.Count
}

// init
// Create a Merkle Tree manager to collect hashes and build a Merkle Tree and a
// database behind it to validate hashes and create receipts.
//
// This is an internal routine; calling init outside of constructing the first
// reference to the Manager doesn't make much sense.
func (m *MerkleManager) init(batch *Batch, markPower uint64, key storage.Key) (err error) {
	if markPower >= 20 { // 2^20 is 1,048,576 and is too big for the collection of elements in memory
		return errors.Format(errors.StatusInternalError, "A power %d is greater than 2^29, and is unreasonable", markPower)
	}
	m.key = key
	m.batch = batch // Manager for writing the Merkle states
	if m.state == nil {
	}
	m.state, err = m.ReadChainHead() // Set the State
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.StatusNotFound):
		m.state = new(MerkleState) // Allocate a state if we don't have one
	default:
		return errors.Format(errors.StatusUnknown, "load chain head: %w", err)
	}
	m.MarkPower = markPower                              // # levels in Merkle Tree to be indexed
	m.MarkFreq = uint64(math.Pow(2, float64(markPower))) // The number of elements between indexes
	m.MarkMask = m.MarkFreq - 1                          // Mask to index of next mark (0 if at a mark)

	return nil
}

// GetState
// Query the database for the State for a given index, i.e. the state
// Note that not every element in the Merkle Tree has a stored state;
// states are stored at the frequency indicated by the Mark Power.  We also
// store the state of the chain at the end of a block regardless, but this
// state overwrites the previous block state.
//
// If no state exists in the database for the element, GetState returns nil
func (m *MerkleManager) GetState(element uint64) (*MerkleState, error) {
	if m.GetElementCount() == 0 {
		ms := new(MerkleState)
		if eHash, err := m.Get(element); err != nil {
			ms.AddToMerkleTree(eHash)
		}
		return ms, nil
	}

	ms, err := m.stateValue("States", element).Get()
	switch {
	case err == nil:
		return ms, nil
	case errors.Is(err, errors.StatusNotFound):
		return nil, nil
	default:
		return nil, errors.Format(errors.StatusUnknown, "load state for element %d: %w", element, err)
	}
}

// GetAnyState
// We only store the state at MarkPoints.  This function computes a missing
// state even if one isn't stored for a particular element.
func (m *MerkleManager) GetAnyState(element uint64) (ms *MerkleState, err error) {
	// Shoot for broke. Return a state if it is in the db
	ms, err = m.GetState(element)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}
	if ms != nil {
		return ms, nil
	}

	if element >= m.GetElementCount() { //               Check to make sure element is not outside bounds
		return nil, errors.Format(errors.StatusBadRequest, "element %d out of range", element)
	}
	MIPrev := element&(^m.MarkMask) - 1 //               Calculate the index of the prior markpoint
	cState, err := m.GetState(MIPrev)   //               Use state at the prior mark point to compute what we need
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}
	if MIPrev < 0 {
		cState = new(MerkleState)
	}
	if cState == nil { //                                Should be in the
		return nil, errors.NotFound( //                        Report error if it isn't in the database'
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
		NMark, err = m.GetState(MINext)
		if err != nil {
			return nil, errors.Wrap(errors.StatusUnknown, err)
		}
		if NMark == nil { //             Read the mark point
			return nil, errors.NotFound("mark not found in the database")
		}
	}
	for _, v := range NMark.HashList { //                           Now iterate and add to the cState
		if element+1 == cState.Count { //                              until the loop adds the element
			break
		}
		v := v // See docs/developer/rangevarref.md
		cState.AddToMerkleTree(v[:])
	}
	if cState.Count&m.MarkMask == 0 { //                           If we progress out of the mark set,
		cState.HashList = cState.HashList[:0] //                       start over collecting hashes.
	}
	return cState, nil
}

// Get the nth leaf node
func (m *MerkleManager) Get(element uint64) (Hash, error) {
	v, err := m.hashValue("Element", element).Get()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknown, err)
	}
	return v.Value[:], nil
}

// GetIntermediate
// Return the last two hashes that were combined to create the local
// Merkle Root at the given index.  The element provided must be odd,
// and the Pending List must be fully populated up to height specified.
func (m *MerkleManager) GetIntermediate(element, height uint64) (Left, Right Hash, err error) {
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
