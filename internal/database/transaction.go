package database

import (
	"errors"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
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
func (t *Transaction) Get() (*protocol.Envelope, *protocol.TransactionStatus, []protocol.Signature, error) {
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

	return state, status, signatures.Signatures, nil
}

// Put stores the transaction object metadata, state, status, and signatures.
// Put appends signatures and does not overwrite existing signatures.
//
// See PutState, PutStatus, and AddSignatures.
func (t *Transaction) Put(state *protocol.Envelope, status *protocol.TransactionStatus, sigs []protocol.Signature) error {
	// Ensure the object metadata is stored. Transactions don't have chains, so
	// we don't need to add chain metadata.
	if _, err := t.batch.store.Get(t.key.Object()); errors.Is(err, storage.ErrNotFound) {
		meta := new(protocol.ObjectMetadata)
		meta.Type = protocol.ObjectTypeTransaction
		err = t.batch.putAs(t.key.Object(), meta)
		if err != nil {
			return err
		}
	}

	err := t.PutState(state)
	if err != nil {
		return err
	}

	err = t.PutStatus(status)
	if err != nil {
		return err
	}

	ss, err := t.GetSignatures()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}

	ss.Add(sigs...)
	return t.PutSignatures(ss)
}

// GetState loads the transaction state.
func (t *Transaction) GetState() (*protocol.Envelope, error) {
	data, err := t.batch.store.Get(t.key.State())
	if err != nil {
		return nil, err
	}

	state := new(protocol.Envelope)
	err = state.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return state, nil
}

// PutState stores the transaction state and adds the transaction to the BPT (as a hash).
func (t *Transaction) PutState(state *protocol.Envelope) error {
	data, err := state.MarshalBinary()
	if err != nil {
		return err
	}

	return t.batch.store.Put(t.key.State(), data)
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
	if status.Result == nil {
		status.Result = new(protocol.EmptyResult)
	}

	data, err := status.MarshalBinary()
	if err != nil {
		return err
	}

	return t.batch.store.Put(t.key.Status(), data)
}

// GetSignatures loads the transaction's signatures.
func (t *Transaction) GetSignatures() (*SignatureSet, error) {
	data, err := t.batch.store.Get(t.key.Signatures())
	if errors.Is(err, storage.ErrNotFound) {
		return new(SignatureSet), err
	} else if err != nil {
		return nil, err
	}

	v := new(SignatureSet)
	err = v.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// PutSignatures stores the transaction's signatures.
func (t *Transaction) PutSignatures(v *SignatureSet) error {
	data, err := v.MarshalBinary()
	if err != nil {
		return err
	}

	return t.batch.store.Put(t.key.Signatures(), data)
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

	return t.batch.store.Put(t.key.Synthetic(), data)
}
