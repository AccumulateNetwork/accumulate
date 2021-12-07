package state

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/AccumulateNetwork/accumulate/smt/storage"
)

type AnchorChainManager struct {
	Chain ChainManager
}

func (ac *AnchorChainManager) Height() int64 {
	return ac.Chain.Height()
}

func (ac *AnchorChainManager) Record() (*Anchor, error) {
	record := new(Anchor)
	err := ac.Chain.RecordAs(record)
	return record, err
}

func (ac *AnchorChainManager) Update(index int64, timestamp time.Time, chains [][32]byte) error {
	// Sort the chain IDs
	sort.Slice(chains, func(i, j int) bool {
		return bytes.Compare(chains[i][:], chains[j][:]) < 0
	})

	// Load the record
	prev, err := ac.Record()
	if errors.Is(err, storage.ErrNotFound) {
		prev = new(Anchor)
	} else if err != nil {
		return err
	}

	// Make sure the block index is increasing
	if prev.Index >= index {
		panic(fmt.Errorf("Current height is %d but the next block height is %d!", prev.Index, index))
	}

	// Add an anchor for each updated chain to the anchor chain
	for _, chainId := range chains {
		chain, err := ac.Chain.state.ManageChain(chainId)
		if err != nil {
			return err
		}

		err = ac.Chain.AddEntry(chain.Anchor())
		if err != nil {
			return err
		}
	}

	// Update the record
	err = ac.Chain.UpdateAs(&Anchor{
		Index:     index,
		Timestamp: timestamp,
		Chains:    chains,
	})
	if err != nil {
		return err
	}

	return nil
}
