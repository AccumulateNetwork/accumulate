package indexing

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
)

// BlockStateIndexer tracks transient state for a block.
type BlockStateIndexer struct {
	value *database.ValueAs[*BlockStateIndex]
}

// BlockState returns a block state indexer.
func BlockState(batch *database.Batch, ledger *url.URL) *BlockStateIndexer {
	v := database.AccountIndex(batch, ledger, newfn[BlockStateIndex](), "BlockState")
	return &BlockStateIndexer{v}
}

// Clear resets the block state.
func (x *BlockStateIndexer) Clear() error {
	return x.value.Put(new(BlockStateIndex))
}

// Get loads the block state.
func (x *BlockStateIndexer) Get() (*BlockStateIndex, error) {
	state, err := x.value.Get()
	switch {
	case err == nil:
		return state, nil
	case errors.Is(err, errors.StatusNotFound):
		return new(BlockStateIndex), nil
	default:
		return nil, err
	}
}

// DidProduceSynthTxn records a produced synthetic transaction.
func (x *BlockStateIndexer) DidProduceSynthTxn(entry *BlockStateSynthTxnEntry) error {
	state, err := x.Get()
	switch {
	case err == nil:
		// Ok
	case errors.Is(err, errors.StatusNotFound):
		state = new(BlockStateIndex)
	default:
		return err
	}

	state.ProducedSynthTxns = append(state.ProducedSynthTxns, entry)
	return x.value.Put(state)
}
