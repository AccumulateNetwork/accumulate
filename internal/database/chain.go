package database

import (
	"encoding"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// Chain manages a Merkle tree (chain).
type Chain struct {
	account  *Account
	writable bool
	merkle   *MerkleManager
}

// newChain creates a new Chain.
func newChain(account *Account, key storage.Key, writable bool) (*Chain, error) {
	m := new(Chain)
	m.account = account
	m.writable = writable

	var err error
	m.merkle, err = NewManager(account.batch, key, markPower)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// Height returns the height of the chain.
func (c *Chain) Height() uint64 {
	return c.merkle.state.Count
}

// Entry loads the entry in the chain at the given height.
func (c *Chain) Entry(height uint64) ([]byte, error) {
	return c.merkle.Get(height)
}

// EntryAs loads and unmarshals the entry in the chain at the given height.
func (c *Chain) EntryAs(height uint64, value encoding.BinaryUnmarshaler) error {
	data, err := c.Entry(height)
	if err != nil {
		return err
	}

	return value.UnmarshalBinary(data)
}

// Entries returns entries in the given range.
func (c *Chain) Entries(start uint64, end uint64) ([][]byte, error) {
	if end > c.Height() {
		end = c.Height()
	}

	if end < start {
		return nil, errors.New("invalid range: start is greater than end")
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
		start += uint64(len(h))
	}

	return entries, nil
}

// State returns the state of the chain at the given height.
func (c *Chain) State(height uint64) (*MerkleState, error) {
	return c.merkle.GetAnyState(height)
}

// CurrentState returns the current state of the chain.
func (c *Chain) CurrentState() *MerkleState {
	return c.merkle.state
}

// HeightOf returns the height of the given entry in the chain.
func (c *Chain) HeightOf(hash []byte) (uint64, error) {
	return c.merkle.GetElementIndex(hash)
}

// Anchor calculates the anchor of the current Merkle state.
func (c *Chain) Anchor() []byte {
	return c.merkle.state.GetMDRoot()
}

// AnchorAt calculates the anchor of the chain at the given height.
func (c *Chain) AnchorAt(height uint64) ([]byte, error) {
	ms, err := c.State(uint64(height))
	if err != nil {
		return nil, err
	}
	return ms.GetMDRoot(), nil
}

// Pending returns the pending roots of the current Merkle state.
func (c *Chain) Pending() [][32]byte {
	return c.merkle.state.Pending
}

// AddEntry adds an entry to the chain
func (c *Chain) AddEntry(entry []byte, unique bool) error {
	if !c.writable {
		return fmt.Errorf("chain opened as read-only")
	}

	if entry == nil {
		panic("attempted to add a nil entry to a chain")
	}

	err := c.merkle.AddHash(entry, unique)
	if err != nil {
		return err
	}

	return c.account.putBpt()
}

// Receipt builds a receipt from one index to another
func (c *Chain) Receipt(from, to uint64) (*types.Receipt, error) {
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
	r := new(types.Receipt)
	r.StartIndex = from
	r.AnchorIndex = to
	r.Start, err = c.Entry(from)
	if err != nil {
		return nil, err
	}
	r.Anchor, err = c.Entry(to)
	if err != nil {
		return nil, err
	}

	// If this is the first element in the Merkle Tree, we are already done
	if from == 0 && to == 0 {
		r.Result = r.Start
		return r, nil
	}

	err = c.merkle.BuildReceipt(r)
	if err != nil {
		return nil, err
	}

	return r, nil
}
