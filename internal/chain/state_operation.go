package chain

import (
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
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
	entry        *protocol.DataEntry
}

//UpdateData will cache a data associated with a DataAccount chain.
//the cache data will not be stored directly in the state but can be used
//upstream for storing a chain in the state database.
func (m *stateCache) UpdateData(record protocol.Account, entryHash []byte, dataEntry *protocol.DataEntry) {
	var stateRec protocol.Account

	if record.Type() == protocol.AccountTypeLiteDataAccount {
		stateRec = record
	}

	m.operations = append(m.operations, &addDataEntry{record.GetUrl(), stateRec, entryHash, dataEntry})
}

func (op *addDataEntry) Execute(st *stateCache) ([]protocol.Account, error) {
	// Add entry to data chain
	record := st.batch.Account(op.url)

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

	// Add entry to data chain
	data, err := record.Data()
	if err != nil {
		return nil, fmt.Errorf("failed to load data chain of %q: %v", op.url, err)
	}

	index := data.Height()
	err = data.Put(op.hash, op.entry)
	if err != nil {
		return nil, fmt.Errorf("failed to add entry to data chain of %q: %v", op.url, err)
	}

	err = st.state.ChainUpdates.DidAddChainEntry(st.batch, op.url, protocol.DataChain, protocol.ChainTypeData, op.hash, uint64(index), 0, 0)
	if err != nil {
		return nil, err
	}

	// Add TX to main chain
	return nil, st.state.ChainUpdates.AddChainEntry(st.batch, op.url, protocol.MainChain, protocol.ChainTypeTransaction, st.txHash[:], 0, 0)
}

type addChainEntryOp struct {
	account     *url.URL
	name        string
	typ         protocol.ChainType
	entry       []byte
	sourceIndex uint64
	sourceBlock uint64
}

func (m *stateCache) AddChainEntry(u *url.URL, name string, typ protocol.ChainType, entry []byte, sourceIndex, sourceBlock uint64) error {
	// The main and pending chain cannot be updated this way
	switch strings.ToLower(name) {
	case protocol.MainChain, protocol.SignatureChain:
		return fmt.Errorf("invalid operation: cannot update %s chain with AddChainEntry", name)
	}

	// Check if the chain is valid
	_, err := m.batch.Account(u).Chain(name, typ)
	if err != nil {
		return fmt.Errorf("failed to load %s#chain/%s: %v", u, name, err)
	}

	m.operations = append(m.operations, &addChainEntryOp{u, name, typ, entry, sourceIndex, sourceBlock})
	return nil
}

func (op *addChainEntryOp) Execute(st *stateCache) ([]protocol.Account, error) {
	return nil, st.state.ChainUpdates.AddChainEntry(st.batch, op.account, op.name, op.typ, op.entry, op.sourceIndex, op.sourceBlock)
}

type writeIndex struct {
	index *database.Value
	put   bool
	value []byte
}

func (c *stateCache) RecordIndex(u *url.URL, key ...interface{}) *writeIndex {
	return c.getIndex(c.batch.Account(u).Index(key...))
}

func (c *stateCache) getIndex(i *database.Value) *writeIndex {
	op, ok := c.indices[i.Key()]
	if ok {
		return op
	}

	op = &writeIndex{i, false, nil}
	c.indices[i.Key()] = op
	c.operations = append(c.operations, op)
	return op
}

func (op *writeIndex) Get() ([]byte, error) {
	if !op.put {
		return op.index.Get()
	}
	return op.value, nil
}

func (op *writeIndex) Put(data []byte) error {
	op.value, op.put = data, true
	return nil
}

func (op *writeIndex) Execute(st *stateCache) ([]protocol.Account, error) {
	if !op.put {
		return nil, nil
	}
	return nil, op.index.Put(op.value)
}
