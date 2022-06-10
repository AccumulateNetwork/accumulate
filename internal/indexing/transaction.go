package indexing

import (
	"errors"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// TransactionChainIndexer indexes account chains against a transaction.
type TransactionChainIndexer struct {
	value *database.ValueAs[*TransactionChainIndex]
}

// TransactionChain returns a transaction chain indexer.
func TransactionChain(batch *database.Batch, txid []byte) *TransactionChainIndexer {
	v := database.TransactionIndex(batch, txid, newfn[TransactionChainIndex](), "Chains")
	return &TransactionChainIndexer{v}
}

// Get loads and unmarshals the index. Get returns an empty index if it has not
// been defined.
func (t *TransactionChainIndexer) Get() (*TransactionChainIndex, error) {
	v, err := t.value.Get()
	switch {
	case err == nil:
		return v, nil
	case errors.Is(err, storage.ErrNotFound):
		return new(TransactionChainIndex), nil
	default:
		return nil, err
	}
}

// Add adds a new entry to the index.
func (t *TransactionChainIndexer) Add(entry *TransactionChainEntry) error {
	v, err := t.Get()
	if err != nil {
		return err
	}

	v.Entries = append(v.Entries, entry)
	return t.value.Put(v)
}
