package indexing

import (
	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
)

// BlockChainUpdatesIndexer indexes chain updates for each block.
type BlockChainUpdatesIndexer struct {
	*record.List[*database.ChainUpdate]
}

// BlockChainUpdates returns a block updates indexer.
func BlockChainUpdates(batch *database.Batch, network *config.Describe, blockIndex uint64) *BlockChainUpdatesIndexer {
	return &BlockChainUpdatesIndexer{batch.SystemData(network.PartitionId).BlockChainUpdates(blockIndex)}
}

func (x *BlockChainUpdatesIndexer) Set(entries []database.ChainUpdate) error {
	var ptrs []*database.ChainUpdate
	for _, v := range entries {
		v := v // See docs/developer/rangevarref.md
		ptrs = append(ptrs, &v)
	}
	return x.Put(ptrs)
}
