// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"fmt"
)

// getReceipt
// Given a merkle tree and two elements, produce a proof that the element was used to derive the DAG at the anchor
// Note that the element must be added to the Merkle Tree before the anchor, but the anchor can be any element
// after the element, or even the element itself.
func getReceipt(manager *Chain, element []byte, anchor []byte) (r *Receipt, err error) {
	// Allocate r, the receipt we are building and record our element
	r = new(Receipt)  // Allocate a r
	r.Start = element // Add the element to the r
	r.End = anchor    // Add the anchor hash to the r
	if r.StartIndex, err = manager.IndexOf(element); err != nil {
		return nil, err
	}
	if r.EndIndex, err = manager.IndexOf(anchor); err != nil {
		return nil, err
	}

	head, err := manager.Head().Get()
	if err != nil {
		return nil, err
	}

	if r.StartIndex > r.EndIndex ||
		r.StartIndex < 0 ||
		r.StartIndex > head.Count { // The element must be at the anchorIndex or before
		return nil, fmt.Errorf("invalid indexes for the element %d and anchor %d", r.StartIndex, r.EndIndex)
	}

	if r.StartIndex == 0 && r.EndIndex == 0 { // If this is the first element in the Merkle Tree, we are already done.
		r.Anchor = element // A Merkle Tree of one element has a root of the element itself.
		return r, nil      // And we are done!
	}

	if err := manager.buildReceipt(r); err != nil {
		return nil, err
	}
	return r, nil
}

// buildReceipt
// takes the values collected by GetReceipt and flushes out the data structures
// in the receipt to represent a fully populated version.
func (m *Chain) buildReceipt(r *Receipt) error {
	state, _ := m.StateAt(r.EndIndex) // Get the state at the Anchor Index
	state.trim()                      // If Pending has any trailing nils, remove them.
	return r.build(func(element, height int64) ([]byte, []byte, error) {
		return m.getIntermediate(element, height)
	}, state)
}

// Receipt builds a receipt from one index to another
func (c *Chain) Receipt(from, to int64) (*Receipt, error) {
	if from < 0 {
		return nil, fmt.Errorf("invalid range: from (%d) < 0", from)
	}
	if to < 0 {
		return nil, fmt.Errorf("invalid range: to (%d) < 0", to)
	}

	head, err := c.Head().Get()
	if err != nil {
		return nil, err
	}
	if from > head.Count {
		return nil, fmt.Errorf("invalid range: from (%d) > height (%d)", from, head.Count)
	}
	if to > head.Count {
		return nil, fmt.Errorf("invalid range: to (%d) > height (%d)", to, head.Count)
	}
	if from > to {
		return nil, fmt.Errorf("invalid range: from (%d) > to (%d)", from, to)
	}

	r := new(Receipt)
	r.StartIndex = from
	r.EndIndex = to
	r.Start, err = c.Entry(from)
	if err != nil {
		return nil, err
	}
	r.End, err = c.Entry(to)
	if err != nil {
		return nil, err
	}

	// If this is the first element in the Merkle Tree, we are already done
	if from == 0 && to == 0 {
		r.Anchor = r.Start
		return r, nil
	}

	err = c.buildReceipt(r)
	if err != nil {
		return nil, err
	}

	return r, nil
}
