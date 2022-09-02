package database

import (
	"encoding"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
)

// Chain manages a Merkle tree (chain).
type Chain struct {
	merkle *managed.MerkleManager
	head   *managed.MerkleState
}

func wrapChain(merkle *managed.Chain) (*Chain, error) {
	m := new(Chain)
	m.merkle = merkle

	var err error
	m.head, err = m.merkle.Head().Get()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}

	return m, nil
}

// Height returns the height of the chain.
func (c *Chain) Height() int64 {
	return c.head.Count
}

// Entry loads the entry in the chain at the given height.
func (c *Chain) Entry(height int64) ([]byte, error) {
	return c.merkle.Get(height)
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
		return nil, errors.New(errors.StatusBadRequest, "invalid range: start is greater than end")
	}

	// GetRange will not cross mark point boundaries, so we may need to call it
	// multiple times
	entries := make([][]byte, 0, end-start)
	for start < end {
		h, err := c.merkle.GetRange(start, end)
		if err != nil {
			return nil, err
		}

		for i := range h {
			entries = append(entries, h[i])
		}
		start += int64(len(h))
	}

	return entries, nil
}

// State returns the state of the chain at the given height.
func (c *Chain) State(height int64) (*managed.MerkleState, error) {
	return c.merkle.GetAnyState(height)
}

// CurrentState returns the current state of the chain.
func (c *Chain) CurrentState() *managed.MerkleState {
	return c.head
}

// HeightOf returns the height of the given entry in the chain.
func (c *Chain) HeightOf(hash []byte) (int64, error) {
	return c.merkle.GetElementIndex(hash)
}

// Anchor calculates the anchor of the current Merkle state.
func (c *Chain) Anchor() []byte {
	return c.head.GetMDRoot()
}

// AnchorAt calculates the anchor of the chain at the given height.
func (c *Chain) AnchorAt(height uint64) ([]byte, error) {
	ms, err := c.State(int64(height))
	if err != nil {
		return nil, err
	}
	return ms.GetMDRoot(), nil
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

	err := c.merkle.AddHash(entry, unique)
	return errors.Wrap(errors.StatusUnknownError, err)
}

// Receipt builds a receipt from one index to another
func (c *Chain) Receipt(from, to int64) (*managed.Receipt, error) {
	if from < 0 {
		return nil, fmt.Errorf("invalid range: from (%d) < 0", from)
	}
	if to < 0 {
		return nil, fmt.Errorf("invalid range: to (%d) < 0", to)
	}
	if from > c.Height() {
		return nil, fmt.Errorf("invalid range: from (%d) > height (%d)", from, c.Height())
	}
	if to > c.Height() {
		return nil, fmt.Errorf("invalid range: to (%d) > height (%d)", to, c.Height())
	}
	if from > to {
		return nil, fmt.Errorf("invalid range: from (%d) > to (%d)", from, to)
	}

	var err error
	r := managed.NewReceipt(c.merkle)
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

	err = r.BuildReceipt()
	if err != nil {
		return nil, err
	}

	return r, nil
}

// RestoreHead is specifically only to be used to restore a
// chain's head from a snapshot.
func (c *Chain) RestoreHead(head *managed.MerkleState) error {
	return c.merkle.Head().Put(head)
}
