package chain

import (
	"errors"
	"fmt"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type stateOperation interface {
	// Execute executes the operation and returns any chains that should be
	// created via a synthetic transaction.
	Execute(*stateCache) ([]state.Chain, error)
}

type createRecords struct {
	records []state.Chain
}

// Create queues a record for a synthetic chain create transaction. Will panic
// if called by a synthetic transaction. Will panic if the record is a
// transaction.
func (m *stateCache) Create(record ...state.Chain) {
	if m.txType.IsSynthetic() {
		panic("Called StateManager.Create from a synthetic transaction!")
	}
	for _, r := range record {
		if r.Header().Type.IsTransaction() {
			panic("Called StateManager.Create with a transaction record!")
		}

		u, err := r.Header().ParseUrl()
		if err != nil {
			// TODO Return error
			panic(fmt.Errorf("invalid URL: %v", err))
		}

		m.chains[u.AccountID32()] = r
	}

	m.operations = append(m.operations, &createRecords{record})
}

func (op *createRecords) Execute(st *stateCache) ([]state.Chain, error) {
	return op.records, nil
}

type updateRecord struct {
	url    *url.URL
	record state.Chain
}

// Update queues a record for storage in the database. The queued update will
// fail if the record does not already exist, unless it is created by a
// synthetic transaction, or the record is a transaction.
func (m *stateCache) Update(record ...state.Chain) {
	for _, r := range record {
		u, err := r.Header().ParseUrl()
		if err != nil {
			// TODO Return error
			panic(fmt.Errorf("invalid URL: %v", err))
		}

		m.chains[u.AccountID32()] = r
		m.operations = append(m.operations, &updateRecord{u, r})
	}
}

func (op *updateRecord) Execute(st *stateCache) ([]state.Chain, error) {
	// Update: update an existing record. Non-synthetic transactions are
	// not allowed to create records, so we must check if the record
	// already exists. The record may have been added to the DB
	// transaction already, so in order to actually know if the record
	// exists on disk, we have to use GetPersistentEntry.

	rec := st.batch.Account(op.url)
	_, err := rec.GetState()
	switch {
	case err == nil:
		// If the record already exists, update it

	case !errors.Is(err, storage.ErrNotFound):
		// Handle unexpected errors
		return nil, fmt.Errorf("failed to check for an existing record: %v", err)

	case st.txType.IsSynthetic() || st.txType.IsInternal():
		// Synthetic and internal transactions are allowed to create records

	default:
		// Non-synthetic transactions are NOT allowed to create records
		// (except for TX records)
		return nil, fmt.Errorf("cannot create a data record in a non-synthetic transaction")
	}

	header := op.record.Header()
	if header.ChainUrl == "" {
		header.ChainUrl = types.String(op.url.String())
	}

	record := st.batch.Account(op.url)
	err = record.PutState(op.record)
	if err != nil {
		return nil, fmt.Errorf("failed to update state of %q: %v", op.url, err)
	}

	return nil, addChainEntry(st.nodeUrl, st.batch, op.url, protocol.MainChain, protocol.ChainTypeTransaction, st.txHash[:], 0)
}

type updateNonce struct {
	url    *url.URL
	record state.Chain
}

func (m *stateCache) UpdateNonce(record state.Chain) error {
	u, err := record.Header().ParseUrl()
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	// Load the previous state of the record
	rec := m.batch.Account(u)
	old, err := rec.GetState()
	if err != nil {
		return fmt.Errorf("failed to load state for %q", record.Header().ChainUrl)
	}

	// Check that the nonce is the only thing that changed
	switch record.Header().Type {
	case types.AccountTypeLiteTokenAccount:
		old, new := old.(*protocol.LiteTokenAccount), record.(*protocol.LiteTokenAccount)
		old.Nonce = new.Nonce
		if !old.Equal(new) {
			return fmt.Errorf("attempted to change more than the nonce")
		}

	case types.AccountTypeKeyPage:
		old, new := old.(*protocol.KeyPage), record.(*protocol.KeyPage)
		for i := 0; i < len(old.Keys) && i < len(new.Keys); i++ {
			old.Keys[i].Nonce = new.Keys[i].Nonce
		}
		if !old.Equal(new) {
			return fmt.Errorf("attempted to change more than a nonce")
		}

	default:
		return fmt.Errorf("account type %d is not a signator", old.Header().Type)
	}

	m.chains[u.AccountID32()] = record
	m.operations = append(m.operations, &updateNonce{u, record})
	return nil
}

func (op *updateNonce) Execute(st *stateCache) ([]state.Chain, error) {
	record := st.batch.Account(op.url)
	err := record.PutState(op.record)
	if err != nil {
		return nil, fmt.Errorf("failed to update state of %q: %v", op.url, err)
	}

	return nil, addChainEntry(st.nodeUrl, st.batch, op.url, protocol.PendingChain, protocol.ChainTypeTransaction, st.txHash[:], 0)
}

//UpdateCreditBalance update the credits used for a transaction
func (m *stateCache) UpdateCreditBalance(record state.Chain) {
	panic("todo: UpdateCredtedBalance needs to be implemented")
}

type addDataEntry struct {
	url          *url.URL
	liteStateRec state.Chain
	hash         []byte
	entry        *protocol.DataEntry
}

//UpdateData will cache a data associated with a DataAccount chain.
//the cache data will not be stored directly in the state but can be used
//upstream for storing a chain in the state database.
func (m *stateCache) UpdateData(record state.Chain, entryHash []byte, dataEntry *protocol.DataEntry) {
	u, err := record.Header().ParseUrl()
	if err != nil {
		// TODO Return error
		panic(fmt.Errorf("invalid URL: %v", err))
	}

	var stateRec state.Chain

	if record.Header().Type == types.AccountTypeLiteDataAccount {
		stateRec = record
	}

	m.operations = append(m.operations, &addDataEntry{u, stateRec, entryHash, dataEntry})
}

func (op *addDataEntry) Execute(st *stateCache) ([]state.Chain, error) {
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

	err = didAddChainEntry(st.nodeUrl, st.batch, op.url, protocol.DataChain, protocol.ChainTypeData, uint64(index), 0)
	if err != nil {
		return nil, err
	}

	// Add TX to main chain
	return nil, addChainEntry(st.nodeUrl, st.batch, op.url, protocol.MainChain, protocol.ChainTypeTransaction, st.txHash[:], 0)
}

type addChainEntryOp struct {
	account     *url.URL
	name        string
	typ         protocol.ChainType
	entry       []byte
	sourceIndex uint64
}

func (m *stateCache) AddChainEntry(u *url.URL, name string, typ protocol.ChainType, entry []byte, sourceIndex uint64) error {
	// The main and pending chain cannot be updated this way
	switch name {
	case protocol.MainChain, protocol.PendingChain:
		return fmt.Errorf("invalid operation: cannot update %s chain with AddChainEntry", name)
	}

	// Check if the chain is valid
	_, err := m.batch.Account(u).Chain(name, typ)
	if err != nil {
		return fmt.Errorf("failed to load %s#chain/%s: %v", u, name, err)
	}

	m.operations = append(m.operations, &addChainEntryOp{u, name, typ, entry, sourceIndex})
	return nil
}

func (op *addChainEntryOp) Execute(st *stateCache) ([]state.Chain, error) {
	return nil, addChainEntry(st.nodeUrl, st.batch, op.account, op.name, op.typ, op.entry, op.sourceIndex)
}

type writeIndex struct {
	index *database.Value
	put   bool
	value []byte
}

func (c *stateCache) RecordIndex(u *url.URL, key ...interface{}) *writeIndex {
	return c.getIndex(c.batch.Account(u).Index(key...))
}

func (c *stateCache) TxnIndex(id []byte, key ...interface{}) *writeIndex {
	return c.getIndex(c.batch.Transaction(id).Index(key...))
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

func (op *writeIndex) Put(data []byte) {
	op.value, op.put = data, true
}

func (op *writeIndex) Execute(st *stateCache) ([]state.Chain, error) {
	if !op.put {
		return nil, nil
	}
	op.index.Put(op.value)
	return nil, nil
}

type signTransaction struct {
	txid      []byte
	signature *transactions.ED25519Sig
}

func (m *stateCache) SignTransaction(txid []byte, signature *transactions.ED25519Sig) {
	m.operations = append(m.operations, &signTransaction{
		txid:      txid,
		signature: signature,
	})
}

func (op *signTransaction) Execute(st *stateCache) ([]state.Chain, error) {
	return nil, st.batch.Transaction(op.txid).AddSignatures(op.signature)
}

type addSyntheticTxns struct {
	txid  []byte
	synth [][32]byte
}

func (m *stateCache) AddSyntheticTxns(txid []byte, synth [][32]byte) {
	m.operations = append(m.operations, &addSyntheticTxns{
		txid:  txid,
		synth: synth,
	})
}

func (op *addSyntheticTxns) Execute(st *stateCache) ([]state.Chain, error) {
	return nil, st.batch.Transaction(op.txid).AddSyntheticTxns(op.synth...)
}
