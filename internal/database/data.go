package database

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Data manages a data chain.
type Data struct {
	batch  *Batch
	record accountBucket
	chain  *Chain
}

// Height returns the number of entries.
func (d *Data) Height() int64 {
	return d.chain.Height()
}

// Put adds an entry to the chain.
func (d *Data) Put(hash []byte, entry *protocol.DataEntry) error {
	// Write data entry
	data, err := entry.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %v", err)
	}
	d.batch.store.Put(d.record.Data(hash), data)

	// Add entry to the chain
	err = d.chain.AddEntry(hash, false)
	if err != nil {
		return err
	}

	return nil
}

// Get looks up an entry by it's hash.
func (d *Data) Get(hash []byte) (*protocol.DataEntry, error) {
	data, err := d.batch.store.Get(d.record.Data(hash))
	if err != nil {
		return nil, err
	}

	entry := new(protocol.DataEntry)
	err = entry.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return entry, nil
}

// GetLatest looks up the latest entry.
func (d *Data) GetLatest() ([]byte, *protocol.DataEntry, error) {
	height := d.chain.Height()
	hash, err := d.chain.Entry(height - 1)
	if err != nil {
		return nil, nil, err
	}

	entry, err := d.Get(hash)
	if err != nil {
		return nil, nil, err
	}

	return hash, entry, nil
}

// GetHashes returns entry hashes in the given range
func (d *Data) GetHashes(start, end int64) ([][]byte, error) {
	return d.chain.Entries(start, end)
}

// Entry looks up an entry by its height.
func (d *Data) Entry(height int64) (*protocol.DataEntry, error) {
	hash, err := d.chain.Entry(height)
	if err != nil {
		return nil, err
	}

	return d.Get(hash)
}
