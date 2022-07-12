package indexing

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

// BlockStateIndexer tracks transient state for a block.
type BlockStateIndexer struct {
	*record.Set[*database.BlockStateSynthTxnEntry]
}

// BlockState returns a block state indexer.
func BlockState(batch *database.Batch, ledger *url.URL) *BlockStateIndexer {
	return &BlockStateIndexer{batch.BlockState(ledger)}
}

// Clear resets the block state.
func (x *BlockStateIndexer) Clear() error {
	return x.Put(nil)
}

// DidProduceSynthTxn records a produced synthetic transaction.
func (x *BlockStateIndexer) DidProduceSynthTxn(entry *database.BlockStateSynthTxnEntry) error {
	return x.Add(entry)
}
