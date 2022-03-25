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
	meta := new(protocol.ObjectMetadata)
	err := t.batch.getValuePtr(t.key.Object(), meta, &meta, true)
	if errors.Is(err, storage.ErrNotFound) {
		meta.Type = protocol.ObjectTypeTransaction
		t.batch.putValue(t.key.Object(), meta)
	} else if err != nil {
		return err
	}

	err = t.PutState(state)
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
	v := new(protocol.Envelope)
	err := t.batch.getValuePtr(t.key.State(), v, &v, false)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// PutState stores the transaction state.
func (t *Transaction) PutState(v *protocol.Envelope) error {
	t.batch.putValue(t.key.State(), v)
	return nil
}

// GetStatus loads the transaction status.
func (t *Transaction) GetStatus() (*protocol.TransactionStatus, error) {
	v := new(protocol.TransactionStatus)
	err := t.batch.getValuePtr(t.key.Status(), v, &v, true)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// PutStatus stores the transaction status.
func (t *Transaction) PutStatus(v *protocol.TransactionStatus) error {
	if v.Result == nil {
		v.Result = new(protocol.EmptyResult)
	}

	t.batch.putValue(t.key.Status(), v)
	return nil
}

// GetSignatures loads the transaction's signatures.
func (t *Transaction) GetSignatures() (*SignatureSet, error) {
	v := new(SignatureSet)
	err := t.batch.getValuePtr(t.key.Signatures(), v, &v, true)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	return v, nil
}

// PutSignatures stores the transaction's signatures.
func (t *Transaction) PutSignatures(v *SignatureSet) error {
	t.batch.putValue(t.key.Signatures(), v)
	return nil
}

// GetSyntheticTxns loads the IDs of synthetic transactions produced by the
// transaction.
func (t *Transaction) GetSyntheticTxns() (*protocol.HashSet, error) {
	v := new(protocol.HashSet)
	err := t.batch.getValuePtr(t.key.Synthetic(), v, &v, true)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}
	return v, nil
}

// PutSyntheticTxns stores the IDs of synthetic transactions produced by the
// transaction.
func (t *Transaction) PutSyntheticTxns(v *protocol.HashSet) error {
	t.batch.putValue(t.key.Synthetic(), v)
	return nil
}

// AddSyntheticTxns is a convenience method that calls GetSyntheticTxns, adds
// the IDs, and calls PutSyntheticTxns.
func (t *Transaction) AddSyntheticTxns(txids ...[32]byte) error {
	set, err := t.GetSyntheticTxns()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}

	for _, id := range txids {
		set.Add(id)
	}

	return t.PutSyntheticTxns(set)
}
