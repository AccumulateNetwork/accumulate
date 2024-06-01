// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"encoding"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

// Chain manages a Merkle tree (chain).
type Chain struct {
	merkle *MerkleManager
	head   *merkle.State
}

func wrapChain(merkle *MerkleManager) (*Chain, error) {
	m := new(Chain)
	m.merkle = merkle

	var err error
	m.head, err = m.merkle.Head().Get()
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	return m, nil
}

// Height returns the height of the chain.
func (c *Chain) Height() int64 {
	return c.head.Count
}

// Entry loads the entry in the chain at the given height.
func (c *Chain) Entry(height int64) ([]byte, error) {
	return c.merkle.Entry(height)
}

// EntryAs loads and unmarshals the entry in the chain at the given height.
func (c *Chain) EntryAs(height int64, value encoding.BinaryUnmarshaler) error {
	data, err := c.Entry(height)
	if err != nil {
		return err
	}

	return value.UnmarshalBinary(data)
}

// Entries returns entries in the given range.
func (c *Chain) Entries(start int64, end int64) ([][]byte, error) {
	if end > c.Height() {
		end = c.Height()
	}

	if end < start {
		return nil, errors.BadRequest.With("invalid range: start is greater than end")
	}

	// GetRange will not cross mark point boundaries, so we may need to call it
	// multiple times
	entries := make([][]byte, 0, end-start)
	for start < end {
		h, err := c.merkle.Entries(start, end)
		if err != nil {
			return nil, err
		}

		entries = append(entries, h...)
		start += int64(len(h))
	}

	return entries, nil
}

// State returns the state of the chain at the given height.
func (c *Chain) State(height int64) (*merkle.State, error) {
	return c.merkle.StateAt(height)
}

// State returns the state of the chain at the given height.
func (c *Chain2) State(height int64) (*merkle.State, error) {
	return c.Inner().StateAt(height)
}

// CurrentState returns the current state of the chain.
func (c *Chain) CurrentState() *merkle.State {
	return c.head
}

// HeightOf returns the height of the given entry in the chain.
func (c *Chain) HeightOf(hash []byte) (int64, error) {
	return c.merkle.IndexOf(hash)
}

// Anchor calculates the anchor of the current Merkle state.
func (c *Chain) Anchor() []byte {
	return c.head.Anchor()
}

// Anchor calculates the anchor of the current Merkle state.
func (c *Chain2) Anchor() ([]byte, error) {
	head, err := c.Head().Get()
	if err != nil {
		return nil, err
	}
	return head.Anchor(), nil
}

// AnchorAt calculates the anchor of the chain at the given height.
func (c *Chain) AnchorAt(height uint64) ([]byte, error) {
	ms, err := c.State(int64(height))
	if err != nil {
		return nil, err
	}
	return ms.Anchor(), nil
}

// Pending returns the pending roots of the current Merkle state.
func (c *Chain) Pending() [][]byte {
	return c.head.Pending
}

// AddEntry adds an entry to the chain
func (c *Chain) AddEntry(entry []byte, unique bool) error {
	if entry == nil {
		panic("attempted to add a nil entry to a chain")
	}

	// TODO Update SMT to handle non-32-byte entries?
	if len(entry) > 32 {
		panic("Entry is too big")
	}
	if len(entry) < 32 {
		padding := make([]byte, 32-len(entry))
		// TODO Remove once AC-1096 is done
		// Fake field number to make unmarshalling work
		padding[0] = 32
		entry = append(entry, padding...)
	}

	err := c.merkle.AddEntry(entry, unique)
	return errors.UnknownError.Wrap(err)
}

// Receipt builds a receipt from one index to another
func (c *Chain) Receipt(from, to int64) (*merkle.Receipt, error) {
	return c.merkle.Receipt(from, to)
}
