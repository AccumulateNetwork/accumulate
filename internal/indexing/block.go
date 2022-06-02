package indexing

import (
	"errors"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// BlockChainUpdatesIndexer indexes chain updates for each block.
type BlockChainUpdatesIndexer struct {
	value *database.Value
}

// BlockChainUpdates returns a block updates indexer.
func BlockChainUpdates(batch *database.Batch, network *config.Describe, blockIndex uint64) *BlockChainUpdatesIndexer {
	return &BlockChainUpdatesIndexer{batch.Account(network.NodeUrl()).Index("Block", "Minor", blockIndex)}
}

// Get loads and unmarshals the index. Get returns an empty index if it has not
// been defined.
func (bu *BlockChainUpdatesIndexer) Get() (*BlockChainUpdatesIndex, error) {
	v := new(BlockChainUpdatesIndex)
	err := bu.value.GetAs(v)
	switch {
	case err == nil:
		return v, nil
	case errors.Is(err, storage.ErrNotFound):
		return new(BlockChainUpdatesIndex), nil
	default:
		return nil, err
	}
}

// Set adds the updates array to the index.
func (bu *BlockChainUpdatesIndexer) Set(updates []ChainUpdate) error {
	v, err := bu.Get()
	if err != nil {
		return err
	}

	v.Entries = make([]*ChainUpdate, len(updates))
	for i, update := range updates {
		update := update // See docs/developer/rangevarref.md
		v.Entries[i] = &update
	}

	return bu.value.PutAs(v)
}
