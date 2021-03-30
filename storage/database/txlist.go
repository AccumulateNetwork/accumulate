package database

import (
	"github.com/AccumulateNetwork/SMT/storage"
)

type TXList struct {
	List []storage.TX
}

// PutBatch
// Put a key value pair into a batch.  These commits are effectively pending and will
// only be written to the database if the batch is passed to PutBatch
func (b *TXList) Put(key [storage.KeyLength]byte, value []byte) error {
	entry := new(storage.TX)
	entry.Key = key[:]
	entry.Value = value
	b.List = append(b.List, *entry)
	return nil
}
