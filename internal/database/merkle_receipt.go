package database

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types"
)

// GetReceipt
// Given a merkle tree and two elements, produce a proof that the element was used to derive the DAG at the anchor
// Note that the element must be added to the Merkle Tree before the anchor, but the anchor can be any element
// after the element, or even the element itself.
func (m *MerkleManager) GetReceipt(element Hash, anchor Hash) (r *types.Receipt, err error) {
	// Allocate r, the receipt we are building and record our element
	r = new(types.Receipt) // Allocate a r
	r.Start = element      // Add the element to the r
	r.Anchor = anchor      // Add the anchor hash to the r
	if r.StartIndex, err = m.GetElementIndex(element); err != nil {
		return nil, err
	}
	if r.AnchorIndex, err = m.GetElementIndex(anchor); err != nil {
		return nil, err
	}

	if r.StartIndex > r.AnchorIndex ||
		r.StartIndex < 0 ||
		r.StartIndex > m.GetElementCount() { // The element must be at the anchorIndex or before
		return nil, fmt.Errorf("invalid indexes for the element %d and anchor %d", r.StartIndex, r.AnchorIndex)
	}

	if r.StartIndex == 0 && r.AnchorIndex == 0 { // If this is the first element in the Merkle Tree, we are already done.
		r.Result = element // A Merkle Tree of one element has a root of the element itself.
		return r, nil      // And we are done!
	}

	if err := m.BuildReceipt(r); err != nil {
		return nil, err
	}
	return r, nil
}

// BuildReceipt
// takes the values collected by GetReceipt and flushes out the data structures
// in the Receipt to represent a fully populated version.
func (m *MerkleManager) BuildReceipt(r *types.Receipt) error {
	state, err := m.GetAnyState(r.AnchorIndex) // Get the state at the Anchor Index
	if err != nil {
		return err
	}
	state.Trim() // If Pending has any trailing nils, remove them.
	return r.Build(func(element, height uint64) (l []byte, r []byte, err error) {
		return m.GetIntermediate(element, height)
	}, state.Pending)
}
