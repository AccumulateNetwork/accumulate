package database

import (
	"github.com/AccumulateNetwork/SMT/smt/storage"
)

type TXList struct {
	List []storage.TX
}

// Init
// If the TXList is copied, it needs to be initialized so it doesn't point to
// the same storage
func (b *TXList) Init() {
	b.List = []storage.TX{} // The copy needs new storage behind its list, and take nothing from the original list
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
