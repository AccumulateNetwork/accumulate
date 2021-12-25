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
	Execute(*StateManager, *DeliverMetadata) error
}

type createRecord struct {
	url    *url.URL
	record state.Chain
}

// Create queues a record for a synthetic chain create transaction. Will panic
// if called by a synthetic transaction. Will panic if the record is a
// transaction.
func (m *StateManager) Create(record ...state.Chain) {
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

		m.chains[u.ResourceChain32()] = r
		m.operations = append(m.operations, &createRecord{u, r})
	}
}

func (op *createRecord) Execute(st *StateManager, meta *DeliverMetadata) error {
	data, err := op.record.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal record: %v", err)
	}

	scc := new(protocol.SyntheticCreateChain)
	scc.Cause = st.txHash
	scc.Chains = []protocol.ChainParams{{Data: data}}
	st.Submit(op.url, scc)
	return nil
}

type updateRecord struct {
	url    *url.URL
	record state.Chain
}

// Update queues a record for storage in the database. The queued update will
// fail if the record does not already exist, unless it is created by a
// synthetic transaction, or the record is a transaction.
func (m *StateManager) Update(record ...state.Chain) {
	for _, r := range record {
		u, err := r.Header().ParseUrl()
		if err != nil {
			// TODO Return error
			panic(fmt.Errorf("invalid URL: %v", err))
		}

		m.chains[u.ResourceChain32()] = r
		m.operations = append(m.operations, &updateRecord{u, r})
	}
}

func (op *updateRecord) Execute(st *StateManager, meta *DeliverMetadata) error {
	// Update: update an existing record. Non-synthetic transactions are
	// not allowed to create records, so we must check if the record
	// already exists. The record may have been added to the DB
	// transaction already, so in order to actually know if the record
	// exists on disk, we have to use GetPersistentEntry.

	rec := st.batch.Record(op.url)
	_, err := rec.GetState()
	switch {
	case err == nil:
		// If the record already exists, update it

	case !errors.Is(err, storage.ErrNotFound):
		// Handle unexpected errors
		return fmt.Errorf("failed to check for an existing record: %v", err)

	case op.record.Header().Type.IsTransaction():
		// Non-synthetic transactions are allowed to create transaction
		// records

	case st.txType.IsSynthetic():
		// Synthetic transactions are allowed to create records

	default:
		// Non-synthetic transactions are NOT allowed to create records
		// (except for TX records)
		return fmt.Errorf("cannot create a data record in a non-synthetic transaction")
	}

	header := op.record.Header()
	if header.ChainUrl == "" {
		header.ChainUrl = types.String(op.url.String())
	}

	err = st.updateStateAndChain(rec, op.record, "Main")
	if err != nil {
		return err
	}

	meta.Updated = append(meta.Updated, op.url)
	return nil
}

type updateNonce struct {
	url    *url.URL
	record state.Chain
}

func (m *StateManager) UpdateNonce(record state.Chain) {
	u, err := record.Header().ParseUrl()
	if err != nil {
		// TODO Return error
		panic(fmt.Errorf("invalid URL: %v", err))
	}

	m.chains[u.ResourceChain32()] = record
	m.operations = append(m.operations, &updateNonce{u, record})
}

func (op *updateNonce) Execute(st *StateManager, meta *DeliverMetadata) error {
	// Load the previous state of the record
	rec := st.batch.Record(op.url)
	old, err := rec.GetState()
	if err != nil {
		return fmt.Errorf("failed to load state for %q", op.record.Header().ChainUrl)
	}

	// Check that the nonce is the only thing that changed
	switch op.record.Header().Type {
	case types.ChainTypeLiteTokenAccount:
		old, new := old.(*protocol.LiteTokenAccount), op.record.(*protocol.LiteTokenAccount)
		old.Nonce = new.Nonce
		if !old.Equal(new) {
			return fmt.Errorf("attempted to change more than the nonce")
		}

	case types.ChainTypeKeyPage:
		old, new := old.(*protocol.KeyPage), op.record.(*protocol.KeyPage)
		for i := 0; i < len(old.Keys) && i < len(new.Keys); i++ {
			old.Keys[i].Nonce = new.Keys[i].Nonce
		}
		if !old.Equal(new) {
			return fmt.Errorf("attempted to change more than a nonce")
		}

	default:
		return fmt.Errorf("chain type %d is not a signator", old.Header().Type)
	}

	return st.updateStateAndChain(rec, op.record, "Pending")
}

type addDataEntry struct {
	url   *url.URL
	hash  []byte
	entry *protocol.DataEntry
}

//UpdateData will cache a data associated with a DataAccount chain.
//the cache data will not be stored directly in the state but can be used
//upstream for storing a chain in the state database.
func (m *StateManager) UpdateData(record state.Chain, entryHash []byte, dataEntry *protocol.DataEntry) {
	u, err := record.Header().ParseUrl()
	if err != nil {
		// TODO Return error
		panic(fmt.Errorf("invalid URL: %v", err))
	}

	// m.chains[u.ResourceChain32()] = record
	m.operations = append(m.operations, &addDataEntry{u, entryHash, dataEntry})
}

func (op *addDataEntry) Execute(st *StateManager, meta *DeliverMetadata) error {
	// Add entry to data chain
	record := st.batch.Record(op.url)
	data, err := record.Data()
	if err != nil {
		return fmt.Errorf("failed to load data chain of %q: %v", op.url, err)
	}

	err = data.Put(op.hash, op.entry)
	if err != nil {
		return fmt.Errorf("failed to add entry to data chain of %q: %v", op.url, err)
	}

	// Add TX to main chain
	main, err := record.Chain(protocol.Main)
	if err != nil {
		return fmt.Errorf("failed to load main chain of %q: %v", op.url, err)
	}

	err = main.AddEntry(st.txHash[:])
	if err != nil {
		return fmt.Errorf("failed to add entry to main chain of %q: %v", op.url, err)
	}

	meta.Updated = append(meta.Updated, op.url)
	return nil
}

type addSignature struct {
	txid [32]byte
	sig  *transactions.ED25519Sig
}

// AddSignature adds a signature to a transaction
func (m *StateManager) AddSignature(publicKey []byte, sig *protocol.SyntheticSignature) {
	m.operations = append(m.operations, &addSignature{
		txid: sig.Txid,
		sig: &transactions.ED25519Sig{
			Nonce:     sig.Nonce,
			Signature: sig.Signature,
			PublicKey: publicKey,
		},
	})
}

func (op *addSignature) Execute(st *StateManager, meta *DeliverMetadata) error {
	tx := st.batch.Transaction(op.txid[:])
	txState, err := tx.GetState()
	if err != nil {
		return err
	}

	err = tx.AddSignatures(op.sig)
	if err != nil {
		return err
	}

	if !txState.TxType().IsSynthetic() {
		return nil
	}

	synth := st.batch.Record(st.nodeUrl.JoinPath(protocol.Synthetic))
	synthState := state.NewSyntheticTransactionChain()
	err = synth.GetStateAs(synthState)
	if err != nil {
		return err
	}

	synthState.Signatures = append(synthState.Signatures, state.SyntheticSignature{
		Txid:      op.txid,
		PublicKey: op.sig.PublicKey,
		Signature: op.sig.Signature,
		Nonce:     op.sig.Nonce,
	})
	return synth.PutState(synthState)
}

type writeIndex struct {
	index *database.Value
	put   bool
	value []byte
}

func (m *StateManager) RecordIndex(u *url.URL, key ...interface{}) *writeIndex {
	return m.getIndex(m.batch.Record(u).Index(key...))
}

func (m *StateManager) TxnIndex(id []byte, key ...interface{}) *writeIndex {
	return m.getIndex(m.batch.Transaction(id).Index(key...))
}

func (m *StateManager) getIndex(i *database.Value) *writeIndex {
	op, ok := m.indices[i.Key()]
	if ok {
		return op
	}

	op = &writeIndex{i, false, nil}
	m.indices[i.Key()] = op
	m.operations = append(m.operations, op)
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

func (op *writeIndex) Execute(st *StateManager, meta *DeliverMetadata) error {
	if !op.put {
		return nil
	}
	op.index.Put(op.value)
	return nil
}

//UpdateCreditBalance update the credits used for a transaction
func (m *StateManager) UpdateCreditBalance(record state.Chain) {
	panic("todo: UpdateCredtedBalance needs to be implemented")
}

func (m *StateManager) updateStateAndChain(record *database.Record, state state.Chain, name string) error {
	chain, err := record.Chain(name)
	if err != nil {
		return fmt.Errorf("failed to load %s chain of %q: %v", name, state.Header().ChainUrl, err)
	}

	err = chain.AddEntry(m.txHash[:])
	if err != nil {
		return fmt.Errorf("failed to add a hash to %s chain of %q: %v", name, state.Header().ChainUrl, err)
	}

	err = record.PutState(state)
	if err != nil {
		return fmt.Errorf("failed to update state of %q: %v", state.Header().ChainUrl, err)
	}

	return nil
}
