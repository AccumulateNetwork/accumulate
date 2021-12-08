package state

import (
	"bytes"
	"errors"
	"sort"

	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

type SynthChainManager struct {
	Chain ChainManager
}

func (sc *SynthChainManager) Height() int64 {
	return sc.Chain.Height()
}

func (sc *SynthChainManager) Record() (*SyntheticTransactionChain, error) {
	record := new(SyntheticTransactionChain)
	err := sc.Chain.RecordAs(record)
	return record, err
}

func (sc *SynthChainManager) Update(index int64, txids [][32]byte) error {
	// Do we have something to do?
	if len(txids) == 0 {
		return nil
	}

	// Sort the txn IDs
	sort.Slice(txids, func(i, j int) bool {
		return bytes.Compare(txids[i][:], txids[j][:]) < 0
	})

	// Add all of the synth txids
	for _, txid := range txids {
		err := sc.Chain.AddEntry(txid[:])
		if err != nil {
			return err
		}
	}

	// Load the record
	record, err := sc.Record()
	if err != nil {
		return err
	}

	// Update the record
	record.Index = index
	record.Count = int64(len(txids))
	return sc.Chain.UpdateAs(record)
}

// LastBlock returns entries from the last block
func (sc *SynthChainManager) LastBlock(blockIndex int64) ([][]byte, error) {
	head, err := sc.Record()
	if errors.Is(err, storage.ErrNotFound) {
		// Nothing to do
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	if head.Index != blockIndex {
		// Nothing happened last block
		return nil, nil
	}

	height := sc.Height()
	return sc.Chain.Entries(height-head.Count, height)
}
