package indexing

import (
	"errors"

	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
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
