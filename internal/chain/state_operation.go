package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type stateOperation interface {
	// Execute executes the operation and returns any chains that should be
	// created via a synthetic transaction.
	Execute(*stateCache) ([]protocol.Account, error)
}

type addDataEntry struct {
	url          *url.URL
	liteStateRec protocol.Account
	hash         []byte
	entry        protocol.DataEntry
}

//UpdateData will cache a data associated with a DataAccount chain.
//the cache data will not be stored directly in the state but can be used
//upstream for storing a chain in the state database.
func (m *stateCache) UpdateData(record protocol.Account, entryHash []byte, dataEntry protocol.DataEntry) {
	var stateRec protocol.Account

	if record.Type() == protocol.AccountTypeLiteDataAccount {
		stateRec = record
	}

	m.operations = append(m.operations, &addDataEntry{record.GetUrl(), stateRec, entryHash, dataEntry})
}

func (op *addDataEntry) Execute(st *stateCache) ([]protocol.Account, error) {
	// Add entry to data chain
	record := st.Batch.Account(op.url)

	// Add lite record to data chain if applicable
	if op.liteStateRec != nil {
		_, err := record.GetState()
		if err != nil {
			//if we have no state, store it
			err = record.PutState(op.liteStateRec)
			if err != nil {
				return nil, err
			}
		}
	}

	err := indexing.Data(st.Batch, op.url).Put(op.hash, st.TxHash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to add entry to data index of %q: %v", op.url, err)
	}

	// Add TX to main chain
	return nil, st.State.ChainUpdates.AddChainEntry(st.Batch, record.MainChain(), st.TxHash[:], 0, 0)
}
