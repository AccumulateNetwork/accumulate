package indexing

import (
	"errors"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// PendingTransactionsIndexer indexes pending transactions of an account.
type PendingTransactionsIndexer struct {
	value *database.Value
}

// PendingTransactions returns a pending transactions indexer.
func PendingTransactions(batch *database.Batch, account *url.URL) *PendingTransactionsIndexer {
	return &PendingTransactionsIndexer{batch.Account(account).Index("PendingTransactions")}
}

// Get loads the list of pending transactions.
func (x *PendingTransactionsIndexer) Get() ([][32]byte, error) {
	idx := new(PendingTransactionsIndex)
	err := x.value.GetAs(idx)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}

	return idx.Transactions, nil
}

// Add adds a transaction to the list of pending transactions, if it is not
// already on the list.
func (x *PendingTransactionsIndexer) Add(txid [32]byte) error {
	pending, err := x.Get()
	if err != nil {
		return err
	}

	for _, pending := range pending {
		if txid == pending {
			return nil
		}
	}

	pending = append(pending, txid)
	return x.value.PutAs(&PendingTransactionsIndex{Transactions: pending})
}

// Remove removes a transaction from the list of pending transactions, if it is
// on the list.
func (x *PendingTransactionsIndexer) Remove(txid [32]byte) error {
	pending, err := x.Get()
	if err != nil {
		return err
	}

	index := -1
	for i, pending := range pending {
		if txid == pending {
			index = i
			break
		}
	}

	switch {
	case index < 0:
		return nil
	case index == 0:
		pending = pending[1:]
	case index == len(pending)-1:
		pending = pending[:len(pending)-1]
	default:
		pending = append(pending[:index], pending[index+1:]...)
	}
	return x.value.PutAs(&PendingTransactionsIndex{Transactions: pending})
}

type DirectoryIndex struct {
	account *database.Account
}

func Directory(batch *database.Batch, account *url.URL) *DirectoryIndex {
	return &DirectoryIndex{batch.Account(account)}
}

func (d *DirectoryIndex) Count() (uint64, error) {
	md := new(protocol.DirectoryIndexMetadata)
	err := d.account.Index("Directory", "Metadata").GetAs(md)
	if err != nil {
		return 0, err
	}
	return md.Count, nil
}

func (d *DirectoryIndex) Get(i uint64) (*url.URL, error) {
	b, err := d.account.Index("Directory", i).Get()
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(string(b))
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (d *DirectoryIndex) Put(u *url.URL) error {
	md := new(protocol.DirectoryIndexMetadata)
	err := d.account.Index("Directory", "Metadata").GetAs(md)
	if err != nil {
		return err
	}

	err = d.account.Index("Directory", md.Count).Put([]byte(u.String()))
	if err != nil {
		return err
	}

	md.Count++
	return d.account.Index("Directory", "Metadata").PutAs(md)
}
