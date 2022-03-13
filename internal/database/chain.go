package database

import (
	"encoding"
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/smt/managed"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// Chain manages a Merkle tree (chain).
type Chain struct {
	key      storage.Key
	writable bool
	merkle   *managed.MerkleManager
}

// newChain creates a new Chain.
func newChain(db storage.KeyValueTxn, key storage.Key, writable bool) (*Chain, error) {
	m := new(Chain)
	m.key = key
	m.writable = writable

	var err error
	m.merkle, err = managed.NewMerkleManager(db, markPower)
	if err != nil {
		return nil, err
	}

	err = m.merkle.SetKey(key)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// Height returns the height of the chain.
func (c *Chain) Height() int64 {
	return c.merkle.MS.Count
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
		return nil, errors.New("invalid range: start is greater than end")
	}

	// GetRange will not cross mark point boundaries, so we may need to call it
	// multiple times
	entries := make([][]byte, 0, end-start)
	for start < end {
		h, err := c.merkle.GetRange(c.key, start, end)
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

// HeightOf returns the height of the given entry in the chain.
func (c *Chain) HeightOf(hash []byte) (int64, error) {
	return c.merkle.GetElementIndex(hash)
}

// Anchor calculates the anchor of the current Merkle state.
func (c *Chain) Anchor() []byte {
	return c.merkle.MS.GetMDRoot()
}

// Pending returns the pending roots of the current Merkle state.
func (c *Chain) Pending() []managed.Hash {
	return c.merkle.MS.Pending
}

// AddEntry adds an entry to the chain
func (c *Chain) AddEntry(entry []byte, unique bool) error {
	if !c.writable {
		return fmt.Errorf("chain opened as read-only")
	}

	// TODO MerkleManager.AddHash really should return an error
	c.merkle.AddHash(entry, unique)
	return nil
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
	r.ElementIndex = from
	r.AnchorIndex = to
	r.Element, err = c.Entry(from)
	if err != nil {
		return nil, err
	}
	r.Anchor, err = c.Entry(to)
	if err != nil {
		return nil, err
	}

	// If this is the first element in the Merkle Tree, we are already done
	if from == 0 && to == 0 {
		r.MDRoot = r.Element
		return r, nil
	}

	err = r.BuildReceipt()
	if err != nil {
		return nil, err
	}

	return r, nil
}
