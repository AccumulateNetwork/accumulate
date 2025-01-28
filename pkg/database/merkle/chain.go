// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"fmt"
	"math"
	"strings"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func NewChain(logger log.Logger, store record.Store, key *record.Key, markPower int64, typ ChainType, namefmt string) *Chain {
	c := new(Chain)
	c.logger.L = logger
	c.store = store
	c.key = key
	c.typ = typ

	// TODO markFreq = 1 << markPower?

	c.markPower = markPower                             // # levels in Merkle Tree to be indexed
	c.markFreq = int64(math.Pow(2, float64(markPower))) // The number of elements between indexes
	c.markMask = c.markFreq - 1                         // Mask to index of next mark (0 if at a mark)

	if strings.ContainsRune(namefmt, '%') {
		c.name = key.Stringf(namefmt)
	} else {
		c.name = namefmt
	}

	return c
}

func (c *Chain) Name() string     { return c.name }
func (c *Chain) Type() ChainType  { return c.typ }
func (c *Chain) MarkPower() int64 { return c.markPower }
func (c *Chain) MarkMask() int64  { return c.markMask }
func (c *Chain) MarkFreq() int64  { return c.markFreq }

func (c *Chain) getMarkPoints() ([]chainStatesKey, error) {
	head, err := c.Head().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load head: %w", err)
	}

	n := head.Count / c.markFreq
	keys := make([]chainStatesKey, 0, n)
	for i := c.markFreq; i < head.Count; i += c.markFreq {
		keys = append(keys, chainStatesKey{Index: uint64(i - 1)})
	}
	return keys, nil
}

// AddEntry adds a Hash to the Chain controlled by the ChainManager. If unique is
// true, the hash will not be added if it is already in the chain.
func (m *Chain) AddEntry(hash []byte, unique bool) error {
	head, err := m.Head().Get() // Get the current state
	if err != nil {
		return err
	}

	hash = copyHash(hash)                    // Just to make sure hash doesn't get changed
	_, err = m.ElementIndex(hash).Get()      // See if this element is a duplicate
	if errors.Is(err, storage.ErrNotFound) { // So only if the hash is not yet added to the Merkle Tree
		err = m.ElementIndex(hash).Put(uint64(head.Count)) // Keep its index
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if unique {
		return nil // Don't add duplicates
	}

	err = m.Element(uint64(head.Count)).Put(hash)
	if err != nil {
		return err
	}
	switch (head.Count + 1) & m.markMask {
	case 0: // Is this the end of the Mark set, i.e. 0, ..., markFreq-1
		head.AddEntry(hash)                                     // Add the hash to the Merkle Tree
		err = m.States(uint64(head.Count) - 1).Put(head.Copy()) // Save Merkle State at n*MarkFreq-1
		if err != nil {
			return err
		}
	case 1: //                              After MarkFreq elements are written
		head.HashList = head.HashList[:0] // then clear the HashList
		fallthrough                       // then fall through as normal
	default:
		head.AddEntry(hash) // 0 to markFeq-2, always add to the merkle tree
	}

	err = m.Head().Put(head)
	if err != nil {
		return fmt.Errorf("error writing chain head: %v", err)
	}

	return nil
}

// IndexOf
// Get an Element of a Merkle Tree from the database
func (m *Chain) IndexOf(hash []byte) (int64, error) {
	i, err := m.ElementIndex(hash).Get()
	if err != nil {
		return 0, err
	}
	return int64(i), nil
}

// getState
// Query the database for the merkle state for a given index, i.e. the state
// Note that not every element in the Merkle Tree has a stored state;
// states are stored at the frequency indicated by the Mark Power.  We also
// store the state of the chain at the end of a block regardless, but this
// state overwrites the previous block state.
//
// If no state exists in the database for the element, getState returns nil
func (m *Chain) getState(element int64) *State {
	head, err := m.Head().Get()
	if err == nil && head.Count == 0 {
		ms := new(State)
		if eHash, err := m.Entry(element); err != nil {
			ms.AddEntry(eHash)
		}
		return ms
	}

	ms, e := m.States(uint64(element)).Get() // Get the data at this height
	if e != nil {                            // If nil, there is no state saved
		return nil //                           return nil, as no state exists
	}
	return ms.Copy() // return it
}

// StateAt
// We only store the state at MarkPoints.  This function computes a missing
// state even if one isn't stored for a particular element.
func (m *Chain) StateAt(element int64) (ms *State, err error) {
	if element == -1 { //                                A need exists for the state before adding the first element
		ms = new(State) //                         In that case, just allocate a State
		return ms, nil  //                         And all is golden
	}
	if ms = m.getState(element); ms != nil { //          Shoot for broke. Return a state if it is in the db
		return ms, nil
	}
	head, err := m.Head().Get()
	if err != nil {
		return nil, err
	} else if element >= head.Count { //               Check to make sure element is not outside bounds
		return nil, errors.BadRequest.With("element out of range")
	}
	MIPrev := element&(^m.markMask) - 1 //               Calculate the index of the prior markpoint
	cState := m.getState(MIPrev)        //               Use state at the prior mark point to compute what we need
	if MIPrev < 0 {
		cState = new(State)
	}
	if cState == nil { // Should be in database
		return nil, errors.NotFound.With("reading a truncated chain, and this index has been truncated")
	}
	cState.HashList = cState.HashList[:0] //             element is past the previous mark, so clear the HashList

	MINext := element&(^m.markMask) - 1 + m.markFreq //            Calculate the following mark point
	var NMark *State                                 //
	if MINext >= head.Count {                        //             If past the end of the chain, then
		if NMark, err = m.Head().Get(); err != nil { //        read the chain state instead
			return nil, err //                                        Should be in the database
		}
	} else {
		if NMark = m.getState(MINext); NMark == nil { // Get next mark point
			return nil, errors.NotFound.With("reading a truncated chain, and this index has been truncated")
		}
	}
	for _, v := range NMark.HashList { //                           Now iterate and add to the cState
		if element+1 == cState.Count { //                              until the loop adds the element
			break
		}
		cState.AddEntry(v)
	}
	if cState.Count&m.markMask == 0 { //                           If we progress out of the mark set,
		cState.HashList = cState.HashList[:0] //                       start over collecting hashes.
	}
	return cState, nil
}

// Entry the nth leaf node
func (m *Chain) Entry(element int64) ([]byte, error) {
	hash, err := m.Element(uint64(element)).Get() // Check the index
	switch {
	case err == nil:
		return hash, nil
	case errors.Is(err, errors.NotFound):
		// Continue
	default:
		return nil, errors.UnknownError.WithFormat("load element %d: %w", element, err)
	}

	head, err := m.Head().Get() // Load the head
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load head: %w", err)
	}
	if element >= head.Count { // Make sure element is not greater than count
		return nil, errors.NotFound.WithFormat("element %d does not exist (count=%d)", element, head.Count)
	}

	lastMark := head.Count &^ m.markMask // Last mark point
	if element >= lastMark {             // Get element from head
		i := element & m.markMask //        Index within the hash list
		if i >= int64(len(head.HashList)) {
			return nil, errors.InvalidRecord.WithFormat("head: expected %d elements, got %d", head.Count, len(head.HashList))
		}
		return head.HashList[i], nil
	}

	elemMark := element&^m.markMask + m.markFreq // Mark point after element
	state, err := m.States(uint64(elemMark - 1)).Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.NotFound):
		return nil, errors.NotFound.WithFormat("cannot locate element %d", element)
	default:
		return nil, errors.UnknownError.WithFormat("load mark point %d: %w", elemMark-1, err)
	}

	i := element & m.markMask // Index within the mark point
	if i >= int64(len(state.HashList)) {
		return nil, errors.InvalidRecord.WithFormat("mark point %d: expected %d elements, got %d", elemMark-1, m.markFreq, len(state.HashList))
	}
	return state.HashList[i], nil
}

// getIntermediate
// Return the last two hashes that were combined to create the local
// Merkle Root at the given index.  The element provided must be odd,
// and the Pending List must be fully populated up to height specified.
func (m *Chain) getIntermediate(element, height int64) (Left, Right []byte, err error) {
	hash, e := m.Entry(element) // Get the element at this height
	if e != nil {               // Error out if we can't
		return nil, nil, e //
	} //
	s, e2 := m.StateAt(element - 1) // Get the state before the state we want
	if e2 != nil {                  // If the element doesn't exist, that's a problem
		return nil, nil, e2 //
	} //
	return getMerkleStateIntermediate(s, hash, height)
}

func getMerkleStateIntermediate(m *State, hash []byte, height int64) (left, right []byte, err error) {
	m.pad()                       // Pad Pending with a nil to remove corner cases
	for i, v := range m.Pending { // Adding the hash is like incrementing a variable
		if v == nil { //               Look for an empty slot; should not encounter one
			return nil, nil, fmt.Errorf("should not encounter a nil at height %d", height)
		}
		if i+1 == int(height) { // Found the height
			left = copyHash(v)      // Get the left and right
			right = copyHash(hash)  //
			return left, right, nil // return them
		}
		hash = combineHashes(v, hash) // If this slot isn't empty, combine the hash with the slot
	}
	return nil, nil, fmt.Errorf("no values found at height %d", height)
}
