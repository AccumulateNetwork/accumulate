package database

import (
	"crypto/sha256"
	"errors"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

// Transaction manages a transaction.
type Transaction struct {
	batch *Batch
	key   transactionBucket
}

// Index returns a value that can read or write an index value.
func (t *Transaction) Index(key ...interface{}) *Value {
	return &Value{t.batch, t.key.Index(key...)}
}

// Get loads the transaction state, status, and signatures.
//
// See GetState, GetStatus, and GetSignatures.
func (t *Transaction) Get() (*state.Transaction, *protocol.TransactionStatus, []*transactions.ED25519Sig, error) {
	state, err := t.GetState()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, nil, nil, err
	}

	status, err := t.GetStatus()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, nil, nil, err
	}

	signatures, err := t.GetSignatures()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, nil, nil, err
	}

	if state == nil && status == nil && signatures == nil {
		return nil, nil, nil, storage.ErrNotFound
	}

	return state, status, signatures, nil
}

// Put stores the transaction state, status, and signatures. Put appends
// signatures and does not overwrite existing signatures.
//
// See PutState, PutStatus, and AddSignatures.
func (t *Transaction) Put(state *state.Transaction, status *protocol.TransactionStatus, sigs []*transactions.ED25519Sig) error {
	err := t.PutState(state)
	if err != nil {
		return err
	}

	err = t.PutStatus(status)
	if err != nil {
		return err
	}

	if len(sigs) > 0 {
		return t.AddSignatures(sigs...)
	}
	return nil
}

// GetState loads the transaction state.
func (t *Transaction) GetState() (*state.Transaction, error) {
	data, err := t.batch.store.Get(t.key.State())
	if err != nil {
		return nil, err
	}

	state := new(state.Transaction)
	err = state.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return state, nil
}

// PutState stores the transaction state and adds the transaction to the BPT (as a hash).
func (t *Transaction) PutState(state *state.Transaction) error {
	data, err := state.MarshalBinary()
	if err != nil {
		return err
	}

	t.batch.store.Put(t.key.State(), data)
	t.batch.putBpt(t.key.Object(), sha256.Sum256(data))
	return nil
}

// GetStatus loads the transaction state.
func (t *Transaction) GetStatus() (*protocol.TransactionStatus, error) {
	data, err := t.batch.store.Get(t.key.Status())
	if err != nil {
		return nil, err
	}

	status := new(protocol.TransactionStatus)
	err = status.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return status, nil
}

// PutStatus stores the transaction state.
func (t *Transaction) PutStatus(status *protocol.TransactionStatus) error {
	data, err := status.MarshalBinary()
	if err != nil {
		return err
	}

	t.batch.store.Put(t.key.Status(), data)
	return nil
}

// GetSignatures loads the transaction's signatures.
func (t *Transaction) GetSignatures() ([]*transactions.ED25519Sig, error) {
	data, err := t.batch.store.Get(t.key.Signatures())
	// if errors.Is
	if err != nil {
		return nil, err
	}

	v := new(txSignatures)
	err = v.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return v.Signatures, nil
}

// AddSignatures adds signatures the transaction's list of signatures.
func (t *Transaction) AddSignatures(signatures ...*transactions.ED25519Sig) error {
	current, err := t.GetSignatures()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}

	v := new(txSignatures)
	v.Signatures = append(current, signatures...)
	data, err := v.MarshalBinary()
	if err != nil {
		return err
	}

	t.batch.store.Put(t.key.Signatures(), data)
	return nil
}

// GetSyntheticTxns returns IDs of synthetic transactions produced by the
// transaction.
func (t *Transaction) GetSyntheticTxns() ([][32]byte, error) {
	data, err := t.batch.store.Get(t.key.Synthetic())
	if err != nil {
		return nil, err
	}

	v := new(txSyntheticTxns)
	err = v.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return v.Txids, nil
}

// AddSyntheticTxns adds the given IDs to the list of synthetic transactions
// produced by the transaction.
func (t *Transaction) AddSyntheticTxns(txids ...[32]byte) error {
	current, err := t.GetSyntheticTxns()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}

	v := new(txSyntheticTxns)
	v.Txids = append(current, txids...)
	data, err := v.MarshalBinary()
	if err != nil {
		return err
	}

	t.batch.store.Put(t.key.Synthetic(), data)
	return nil
}
