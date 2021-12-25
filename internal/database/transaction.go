package database

import (
	"crypto/sha256"
	"encoding/json"
	"errors"

	"github.com/AccumulateNetwork/accumulate/smt/storage"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type Transaction struct {
	batch *Batch
	key   transactionBucket
}

func (t *Transaction) Index(key ...interface{}) *Value {
	return &Value{t.batch, t.key.Index(key...)}
}

func (t *Transaction) Get() (*state.Transaction, json.RawMessage, []*transactions.ED25519Sig, error) {
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

func (t *Transaction) Put(state *state.Transaction, status json.RawMessage, sigs []*transactions.ED25519Sig) error {
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

func (t *Transaction) PutState(state *state.Transaction) error {
	data, err := state.MarshalBinary()
	if err != nil {
		return err
	}

	t.batch.store.Put(t.key.State(), data)
	t.batch.putBpt(t.key.Object(), sha256.Sum256(data))
	return nil
}

func (t *Transaction) GetStatus() (json.RawMessage, error) {
	data, err := t.batch.store.Get(t.key.Status())
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (t *Transaction) PutStatus(status json.RawMessage) error {
	t.batch.store.Put(t.key.Status(), status)
	return nil
}

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
