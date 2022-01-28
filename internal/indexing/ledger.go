package indexing

import (
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
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

// DirectoryAnchorIndexer indexes directory anchors.
type DirectoryAnchorIndexer struct {
	account *database.Account
}

// DirectoryAnchor returns a directory anchor indexer.
func DirectoryAnchor(batch *database.Batch, ledger *url.URL) *DirectoryAnchorIndexer {
	return &DirectoryAnchorIndexer{batch.Account(ledger)}
}

// Add indexes a directory anchor.
func (x *DirectoryAnchorIndexer) Add(anchor *protocol.SyntheticAnchor) error {
	var v *database.Value
	if anchor.Major {
		v = x.account.Index("MajorDirectoryAnchor", anchor.SourceBlock)
	} else {
		v = x.account.Index("MinorDirectoryAnchor", anchor.SourceBlock)
	}

	return v.PutAs(anchor)
}

// AnchorForLocalBlock retrieves the directory anchor that anchors the given local block.
func (x *DirectoryAnchorIndexer) AnchorForLocalBlock(block uint64, major bool) (*protocol.SyntheticAnchor, error) {
	var v *database.Value
	if major {
		v = x.account.Index("MajorDirectoryAnchor", block)
	} else {
		v = x.account.Index("MinorDirectoryAnchor", block)
	}

	anchor := new(protocol.SyntheticAnchor)
	err := v.GetAs(anchor)
	if err != nil {
		return nil, err
	}
	return anchor, nil
}
