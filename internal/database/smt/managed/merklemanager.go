// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package managed

import (
	"fmt"
	"math"
	"strings"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

type MerkleManager = Chain

func NewChain(logger log.Logger, store record.Store, key record.Key, markPower int64, typ ChainType, namefmt, labelfmt string) *Chain {
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
		c.name = fmt.Sprintf(namefmt, key...)
	} else {
		c.name = namefmt
	}

	if strings.ContainsRune(labelfmt, '%') {
		c.label = fmt.Sprintf(labelfmt, key...)
	} else {
		c.label = labelfmt
	}

	return c
}

func (c *Chain) Name() string    { return c.name }
func (c *Chain) Type() ChainType { return c.typ }

// AddHash adds a Hash to the Chain controlled by the ChainManager. If unique is
// true, the hash will not be added if it is already in the chain.
func (m *MerkleManager) AddHash(hash Hash, unique bool) error {
	head, err := m.Head().Get() // Get the current state
	if err != nil {
		return err
	}

	hash = hash.Copy()                       // Just to make sure hash doesn't get changed
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
		head.AddToMerkleTree(hash)                              // Add the hash to the Merkle Tree
		err = m.States(uint64(head.Count) - 1).Put(head.Copy()) // Save Merkle State at n*MarkFreq-1
		if err != nil {
			return err
		}
	case 1: //                              After MarkFreq elements are written
		head.HashList = head.HashList[:0] // then clear the HashList
		fallthrough                       // then fall through as normal
	default:
		head.AddToMerkleTree(hash) // 0 to markFeq-2, always add to the merkle tree
	}

	err = m.Head().Put(head)
	if err != nil {
		return fmt.Errorf("error writing chain head: %v", err)
	}

	return nil
}

// GetElementIndex
// Get an Element of a Merkle Tree from the database
func (m *MerkleManager) GetElementIndex(hash []byte) (int64, error) {
	i, err := m.ElementIndex(hash).Get()
	if err != nil {
		return 0, err
	}
	return int64(i), nil
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
	head, err := m.Head().Get()
	if err == nil && head.Count == 0 {
		ms := new(MerkleState)
		if eHash, err := m.Get(element); err != nil {
			ms.AddToMerkleTree(eHash)
		}
		ms.InitSha256()
		return ms
	}

	ms, e := m.States(uint64(element)).Get() // Get the data at this height
	if e != nil {                            // If nil, there is no state saved
		return nil //                           return nil, as no state exists
	}
	return ms.Copy() // return it
}

// GetAnyState
// We only store the state at MarkPoints.  This function computes a missing
// state even if one isn't stored for a particular element.
func (m *MerkleManager) GetAnyState(element int64) (ms *MerkleState, err error) {
	if element == -1 { //                                A need exists for the state before adding the first element
		ms = new(MerkleState) //                         In that case, just allocate a MerkleState
		ms.InitSha256()       //                         Initialize its hash function
		return ms, nil        //                         And all is golden
	}
	if ms = m.GetState(element); ms != nil { //          Shoot for broke. Return a state if it is in the db
		return ms, nil
	}
	head, err := m.Head().Get()
	if err != nil {
		return nil, err
	} else if element >= head.Count { //               Check to make sure element is not outside bounds
		return nil, errors.New(errors.StatusBadRequest, "element out of range")
	}
	MIPrev := element&(^m.markMask) - 1 //               Calculate the index of the prior markpoint
	cState := m.GetState(MIPrev)        //               Use state at the prior mark point to compute what we need
	if MIPrev < 0 {
		cState = new(MerkleState)
		cState.InitSha256()
	}
	if cState == nil { //                                Should be in the database.
		return nil, errors.New( //                        Report error if it isn't in the database'
			errors.StatusInternalError, "should have a state for all elements(1)")
	}
	cState.HashList = cState.HashList[:0] //             element is past the previous mark, so clear the HashList

	MINext := element&(^m.markMask) - 1 + m.markFreq //            Calculate the following mark point
	var NMark *MerkleState                           //
	if MINext >= head.Count {                        //             If past the end of the chain, then
		if NMark, err = m.Head().Get(); err != nil { //        read the chain state instead
			return nil, err //                                        Should be in the database
		}
	} else {
		if NMark = m.GetState(MINext); NMark == nil { //             Read the mark point
			return nil, errors.New(errors.StatusInternalError, "mark not found in the database")
		}
	}
	for _, v := range NMark.HashList { //                           Now iterate and add to the cState
		if element+1 == cState.Count { //                              until the loop adds the element
			break
		}
		cState.AddToMerkleTree(v)
	}
	if cState.Count&m.markMask == 0 { //                           If we progress out of the mark set,
		cState.HashList = cState.HashList[:0] //                       start over collecting hashes.
	}
	return cState, nil
}

// Get the nth leaf node
func (m *MerkleManager) Get(element int64) (Hash, error) {
	hash, err := m.Element(uint64(element)).Get() // Check the index
	switch {
	case err == nil:
		return hash, nil
	case errors.Is(err, errors.StatusNotFound):
		// Continue
	default:
		return nil, errors.Format(errors.StatusUnknownError, "load element %d: %w", element, err)
	}

	head, err := m.Head().Get() // Load the head
	if err != nil {
		return nil, errors.Format(errors.StatusUnknownError, "load head: %w", err)
	}
	if element >= head.Count { // Make sure element is not greater than count
		return nil, errors.Format(errors.StatusNotFound, "element %d does not exist (count=%d)", element, head.Count)
	}

	lastMark := head.Count &^ m.markMask // Last mark point
	if element >= lastMark {             // Get element from head
		i := element & m.markMask //        Index within the hash list
		if i >= int64(len(head.HashList)) {
			return nil, errors.Format(errors.StatusInternalError, "head: expected %d elements, got %d", head.Count, len(head.HashList))
		}
		return head.HashList[i], nil
	}

	elemMark := element&^m.markMask + m.markFreq // Mark point after element
	state, err := m.States(uint64(elemMark - 1)).Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.StatusNotFound):
		return nil, errors.Format(errors.StatusNotFound, "cannot locate element %d", element)
	default:
		return nil, errors.Format(errors.StatusUnknownError, "load mark point %d: %w", elemMark-1, err)
	}

	i := element & m.markMask // Index within the mark point
	if i >= int64(len(state.HashList)) {
		return nil, errors.Format(errors.StatusInternalError, "mark point %d: expected %d elements, got %d", elemMark-1, m.markFreq, len(state.HashList))
	}
	return state.HashList[i], nil
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
