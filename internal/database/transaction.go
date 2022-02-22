package database

import (
	"crypto/sha256"
	"errors"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/types/state"
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
func (t *Transaction) Get() (*state.Transaction, *protocol.TransactionStatus, []protocol.Signature, error) {
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

// Put stores the transaction object metadata, state, status, and signatures.
// Put appends signatures and does not overwrite existing signatures.
//
// See PutState, PutStatus, and AddSignatures.
func (t *Transaction) Put(state *state.Transaction, status *protocol.TransactionStatus, sigs []protocol.Signature) error {
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

	_, err = t.AddSignatures(sigs...)
	return err
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
	if status.Result == nil {
		status.Result = new(protocol.EmptyResult)
	}

	data, err := status.MarshalBinary()
	if err != nil {
		return err
	}

	t.batch.store.Put(t.key.Status(), data)
	return nil
}

// GetSignatures loads the transaction's signatures.
func (t *Transaction) GetSignatures() ([]protocol.Signature, error) {
	data, err := t.batch.store.Get(t.key.Signatures())
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
func (t *Transaction) AddSignatures(newSignatures ...protocol.Signature) (count int, err error) {
	signatures, err := t.GetSignatures()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return 0, err
	}

	if len(newSignatures) == 0 {
		return len(signatures), nil
	}

	// Only keep one signature per public key
	seen := map[[32]byte]bool{}
	for _, sig := range signatures {
		hash := sha256.Sum256(sig.GetPublicKey())
		seen[hash] = true
	}
	for _, sig := range newSignatures {
		hash := sha256.Sum256(sig.GetPublicKey())
		if seen[hash] {
			continue
		}
		seen[hash] = true
		signatures = append(signatures, sig)
	}

	v := new(txSignatures)
	v.Signatures = signatures
	data, err := v.MarshalBinary()
	if err != nil {
		return 0, err
	}

	t.batch.store.Put(t.key.Signatures(), data)
	return len(signatures), nil
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
