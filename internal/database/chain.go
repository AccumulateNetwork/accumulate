package database

import (
	"errors"

	"github.com/AccumulateNetwork/accumulate/smt/managed"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

// Chain manages a Merkle tree (chain).
type Chain struct {
	db     storage.KeyValueTxn
	key    storage.Key
	merkle *managed.MerkleManager
}

// newChain creates a new Chain.
func newChain(db storage.KeyValueTxn, key storage.Key) (*Chain, error) {
	m := new(Chain)
	m.key = key

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

// Entry returns the entry in the chain at the given height.
func (c *Chain) Entry(height int64) ([]byte, error) {
	return c.merkle.Get(height)
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
	// TODO MerkleManager.GetState really should return an error
	return c.merkle.GetState(height), nil
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
func (c *Chain) AddEntry(entry []byte) error {
	// TODO MerkleManager.AddHash really should return an error
	c.merkle.AddHash(entry)
	return nil
}
