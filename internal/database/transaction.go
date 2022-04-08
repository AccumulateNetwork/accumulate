package database

import (
	"errors"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// Transaction manages a transaction.
type Transaction struct {
	batch *Batch
	key   transactionBucket
}

// Transaction returns a Transaction for the given transaction ID.
func (b *Batch) Transaction(id []byte) *Transaction {
	return &Transaction{b, transaction(id)}
}

// ensureSigner ensures that the transaction's status includes the given signer.
func (t *Transaction) ensureSigner(signer protocol.Signer) error {
	status, err := t.GetStatus()
	if err != nil {
		return err
	}

	status.AddSigner(signer)
	return t.PutStatus(status)
}

// Index returns a value that can read or write an index value.
func (t *Transaction) Index(key ...interface{}) *Value {
	return &Value{t.batch, t.key.Index(key...)}
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
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
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

// Signatures returns a signature set for the given signer.
func (t *Transaction) Signatures(signer *url.URL) (*SignatureSet, error) {
	return t.newSigSet(signer, true)
}

// ReadSignatures returns a read-only signature set for the given signer.
func (t *Transaction) ReadSignatures(signer *url.URL) (*SignatureSet, error) {
	return t.newSigSet(signer, false)
}

// SignaturesForSigner returns a signature set for the given signer account.
func (t *Transaction) SignaturesForSigner(signer protocol.Signer) (*SignatureSet, error) {
	set, err := newSigSet(t, signer, true)
	if err != nil {
		return nil, fmt.Errorf("load signature set: %w", err)
	}

	return set, nil
}

// SignaturesForSigner returns a read-only signature set for the given signer account.
func (t *Transaction) ReadSignaturesForSigner(signer protocol.Signer) (*SignatureSet, error) {
	set, err := newSigSet(t, signer, false)
	if err != nil {
		return nil, fmt.Errorf("load signature set: %w", err)
	}

	return set, nil
}

// AddSignature loads the appropriate siganture set and adds the signature to
// it.
func (t *Transaction) AddSignature(newSignature protocol.Signature) (int, error) {
	set, err := t.newSigSet(newSignature.GetSigner(), true)
	if err != nil {
		return 0, err
	}

	return set.Add(newSignature)
}

func (t *Transaction) newSigSet(signer *url.URL, writable bool) (*SignatureSet, error) {
	var acct protocol.Signer
	err := t.batch.Account(signer).GetStateAs(&acct)
	switch {
	case err == nil:
		// If the signer exists, use its version

	case errors.Is(err, storage.ErrNotFound):
		// If the signer does not exist, use version 0. This is for signatures
		// on synthetic transactions.
		acct = &protocol.UnknownSigner{Url: signer}

	default:
		return nil, fmt.Errorf("load signer: %w", err)
	}

	set, err := newSigSet(t, acct, writable)
	if err != nil {
		return nil, fmt.Errorf("load signature set: %w", err)
	}

	return set, nil
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
