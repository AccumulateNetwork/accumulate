package database

import (
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// BlockChainUpdatesIndex manages the chain updates for a block.
type BlockChainUpdatesIndex struct {
	batch *Batch
	key   blockIndexBucket
}

// BlockChainUpdatesIndex returns a BlockChainUpdatesIndex for the given block index
func (b *Batch) BlockChainUpdatesIndex(blockIndex int64) *BlockChainUpdatesIndex {
	return &BlockChainUpdatesIndex{b, blockUpdatesIndex(blockIndex)}
}

// GetBlockChainUpdatesIndex loads the chain updates index.
func (bu *BlockChainUpdatesIndex) GetBlockChainUpdatesIndex() (*protocol.Envelope, error) {
	v := new(protocol.Envelope)
	err := bu.batch.getValuePtr(bu.key.State(), v, &v, false)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// PutBlockChainUpdatesIndex stores the chain updates index.
func (bu *BlockChainUpdatesIndex) PutBlockChainUpdatesIndex(v *protocol.Envelope) error {
	bu.batch.putValue(bu.key.State(), v)
	return nil
}

// Index returns a value that can read or write an index value.
func (bu *BlockChainUpdatesIndex) Index() *Value {
	return &Value{bu.batch, bu.key.Index()}
}
