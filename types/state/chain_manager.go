package state

import (
	"crypto/sha256"
	"encoding"
	"fmt"
	"math/bits"

	"github.com/AccumulateNetwork/accumulate/internal/logging"
	"github.com/AccumulateNetwork/accumulate/smt/managed"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

// ChainManager manages a Merkle chain.
type ChainManager struct {
	key    []interface{}
	state  *StateDB
	merkle *managed.MerkleManager
}

// ManageChain creates a Merkle chain manager for the given key.
func (s *StateDB) ManageChain(key ...interface{}) (*ChainManager, error) {
	mm, err := s.merkleMgr.ManageChain(key...)
	if err != nil {
		return nil, err
	}

	return &ChainManager{key, s, mm}, nil
}

// Height returns the height of the chain.
func (c *ChainManager) Height() int64 {
	return c.merkle.MS.Count
}

// Record returns the record.
func (c *ChainManager) Record() (*Object, error) {
	data, err := c.state.dbMgr.Key(append(c.key, "Record")...).Get()
	if err != nil {
		return nil, err
	}

	obj := new(Object)
	err = obj.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

// RecordAs loads the record and unmarshals its data into the given value.
func (c *ChainManager) RecordAs(value encoding.BinaryUnmarshaler) error {
	obj, err := c.Record()
	if err != nil {
		return err
	}

	return obj.As(value)
}

// Entry returns the entry in the chain at the given height.
func (c *ChainManager) Entry(height int64) ([]byte, error) {
	return c.merkle.Get(height)
}

func (c *ChainManager) Entries(start int64, end int64) ([][]byte, error) {
	if end > c.Height() {
		end = c.Height()
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
func (c *ChainManager) State(height int64) (*managed.MerkleState, error) {
	// TODO MerkleManager.GetState really should return an error
	return c.merkle.GetState(height), nil
}

// HeightOf returns the height of the given entry in the chain.
func (c *ChainManager) HeightOf(hash []byte) (int64, error) {
	return c.merkle.GetElementIndex(hash)
}

// Anchor calculates the anchor of the current Merkle state.
func (c *ChainManager) Anchor() []byte {
	return c.merkle.MS.GetMDRoot()
}

// AddEntry adds an entry to the chain
func (c *ChainManager) AddEntry(entry []byte) error {
	// TODO MerkleManager.AddHash really should return an error
	c.merkle.AddHash(entry)
	return nil
}

// Update writes the record to the database and updates the BPT.
func (c *ChainManager) Update(record []byte) error {
	obj := new(Object)
	obj.Entry = record
	obj.Height = uint64(c.Height())
	obj.Roots = make([][]byte, 64-bits.LeadingZeros64(obj.Height))
	for i := range obj.Roots {
		if obj.Height&(1<<i) == 0 {
			// Only store the hashes we need
			continue
		}
		obj.Roots[i] = c.merkle.MS.Pending[i].Copy()
	}

	data, err := obj.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal: %v", err)
	}

	h := sha256.Sum256(data)
	c.state.logDebug("Updating chain state", "key", c.key, "hash", logging.AsHex(h))

	c.state.dbMgr.Key(append(c.key, "Record")...).PutBatch(data)
	c.state.bptMgr.Bpt.Insert(storage.ComputeKey(c.key...), h)

	return nil
}

// UpdateAs writes the record value to the database and updates the BPT.
func (c *ChainManager) UpdateAs(value encoding.BinaryMarshaler) error {
	data, err := value.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal: %v", err)
	}
	return c.Update(data)
}
