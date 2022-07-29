package indexing

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
)

// BlockStateIndexer tracks transient state for a block.
type BlockStateIndexer struct {
	*record.Set[*database.BlockStateSynthTxnEntry]
}

// BlockState returns a block state indexer.
func BlockState(batch *database.Batch, partition string) *BlockStateIndexer {
	return &BlockStateIndexer{batch.SystemData(partition).BlockState()}
}

// Clear resets the block state.
func (x *BlockStateIndexer) Clear() error {
	return x.Put(nil)
}

// DidProduceSynthTxn records a produced synthetic transaction.
func (x *BlockStateIndexer) DidProduceSynthTxn(entry *database.BlockStateSynthTxnEntry) error {
	return x.Add(entry)
}
