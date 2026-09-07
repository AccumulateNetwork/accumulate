// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"fmt"
)

// A Segment is the tail of a chain held in memory: the merkle state before
// index First, and the elements from First onwards. It builds the same
// receipts and receipt lists the stored chain would, for any span inside
// it, without reading the chain — the producer's synthetic/anchor cache
// keeps one per block so a package's or a bundle's proof is built from the
// cache alone (healing spec, "The cache").
type Segment struct {
	// First is the chain index of Elements[0].
	First int64
	// Before is the chain's state before First: Count == First.
	Before *State
	// Elements are the entries at First, First+1, ...
	Elements [][]byte
	// MarkMask is the chain's mark mask (MarkFreq-1). A state's hash list
	// restarts at every mark point, and the segment replays that so its
	// states are byte-identical to the chain's.
	MarkMask int64
}

// NewSegment captures the tail of a chain from index first: the state before
// it and its entries through the head.
func NewSegment(c *Chain, first int64) (*Segment, error) {
	head, err := c.Head().Get()
	if err != nil {
		return nil, err
	}
	if first < 0 || first > head.Count {
		return nil, fmt.Errorf("first %d is outside the chain (height %d)", first, head.Count)
	}
	before, err := c.StateAt(first - 1)
	if err != nil {
		return nil, err
	}
	s := &Segment{First: first, Before: before.Copy(), MarkMask: c.markMask}
	for i := first; i < head.Count; i++ {
		h, err := c.Entry(i)
		if err != nil {
			return nil, err
		}
		s.Elements = append(s.Elements, copyHash(h))
	}
	return s, nil
}

// Last is the chain index of the segment's last element, or First-1 when it
// holds none.
func (s *Segment) Last() int64 { return s.First + int64(len(s.Elements)) - 1 }

// Append adds the chain's next element.
func (s *Segment) Append(hash []byte) { s.Elements = append(s.Elements, copyHash(hash)) }

func (s *Segment) entry(i int64) ([]byte, error) {
	if i < s.First || i > s.Last() {
		return nil, fmt.Errorf("index %d is outside the segment [%d, %d]", i, s.First, s.Last())
	}
	return s.Elements[i-s.First], nil
}

// stateAt is the chain's state after element i, for i >= First-1.
func (s *Segment) stateAt(i int64) (*State, error) {
	if i < s.First-1 || i > s.Last() {
		return nil, fmt.Errorf("state at %d is outside the segment [%d, %d]", i, s.First-1, s.Last())
	}
	st := s.Before.Copy()
	for j := s.First; j <= i; j++ {
		// Chain.AddEntry: the first element of a mark set starts a new
		// hash list before it is added
		if st.Count&s.MarkMask == 0 {
			st.HashList = st.HashList[:0]
		}
		st.AddEntry(s.Elements[j-s.First])
	}
	return st, nil
}

// Receipt is Chain.Receipt(from, to) for a span inside the segment.
func (s *Segment) Receipt(from, to int64) (*Receipt, error) {
	if from > to {
		return nil, fmt.Errorf("invalid range: from (%d) > to (%d)", from, to)
	}
	r := new(Receipt)
	r.StartIndex, r.EndIndex = from, to
	var err error
	if r.Start, err = s.entry(from); err != nil {
		return nil, err
	}
	if r.End, err = s.entry(to); err != nil {
		return nil, err
	}
	if from == 0 && to == 0 {
		r.Anchor = r.Start
		return r, nil
	}
	anchorState, err := s.stateAt(to)
	if err != nil {
		return nil, err
	}
	anchorState.trim()
	err = r.build(func(element, height int64) ([]byte, []byte, error) {
		hash, err := s.entry(element)
		if err != nil {
			return nil, nil, err
		}
		st, err := s.stateAt(element - 1)
		if err != nil {
			return nil, nil, err
		}
		return getMerkleStateIntermediate(st, hash, height)
	}, anchorState)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// ReceiptList is GetReceiptList(chain, from, to) for a span inside the
// segment: the state before from, the elements from..to, and the receipt
// anchoring the last of them.
func (s *Segment) ReceiptList(from, to int64) (*ReceiptList, error) {
	if from > to {
		return nil, fmt.Errorf("start %d and end %d is invalid for ReceiptList", from, to)
	}
	r := NewReceiptList()
	for i := from; i <= to; i++ {
		h, err := s.entry(i)
		if err != nil {
			return nil, err
		}
		r.Elements = append(r.Elements, copyHash(h))
	}
	var err error
	r.MerkleState, err = s.stateAt(from - 1)
	if err != nil {
		return nil, err
	}
	r.Receipt, err = s.Receipt(to, to)
	if err != nil {
		return nil, err
	}
	return r, nil
}
