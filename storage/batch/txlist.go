package batch

import (
	"github.com/AccumulateNetwork/SMT/storage"
	"github.com/AccumulateNetwork/SMT/storage/database"
)

type TXList struct {
	Manager *database.Manager
	List    []storage.TX
}

// Begin
// Begin a Transaction List
func (b *TXList) Begin(manager *database.Manager) {
	b.Manager = manager
	b.List = b.List[:0]
}

// End
// Write all the transactions in a given batch of pending transactions.  The
// batch is emptied, so it can be reused.
func (b *TXList) End() (err error) {
	return b.Manager.DB.PutBatch(b.List)
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
