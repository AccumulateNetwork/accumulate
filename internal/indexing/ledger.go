package indexing

import (
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/url"
)

// BlockStateIndexer tracks transient state for a block.
type BlockStateIndexer struct {
	value *database.Value
}

// BlockState returns a block state indexer.
func BlockState(batch *database.Batch, ledger *url.URL) *BlockStateIndexer {
	return &BlockStateIndexer{batch.Account(ledger).Index("BlockState")}
}

// Clear resets the block state.
func (x *BlockStateIndexer) Clear() error {
	return x.value.PutAs(new(BlockStateIndex))
}

// Get loads the block state.
func (x *BlockStateIndexer) Get() (*BlockStateIndex, error) {
	state := new(BlockStateIndex)
	err := x.value.GetAs(state)
	if err != nil {
		return nil, err
	}

	return state, nil
}

// DidProduceSynthTxn records a produced synthetic transaction.
func (x *BlockStateIndexer) DidProduceSynthTxn(entry *BlockStateSynthTxnEntry) error {
	state, err := x.Get()
	if err != nil {
		return err
	}

	state.ProducedSynthTxns = append(state.ProducedSynthTxns, entry)
	return x.value.PutAs(state)
}
