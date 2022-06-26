package database

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
)

// Transaction manages a transaction.
type Transaction struct {
	batch *Batch
	key   transactionBucket
	id    [32]byte
}

// Transaction returns a Transaction for the given transaction ID.
func (b *Batch) Transaction(id []byte) *Transaction {
	return &Transaction{b, transaction(id), *(*[32]byte)(id)}
}

func (t *Transaction) main() Value[*SigOrTxn] {
	return getOrCreateValue(t.batch, t.key.State(), false, record.Struct[SigOrTxn]())
}

func (t *Transaction) status() Value[*protocol.TransactionStatus] {
	return getOrCreateValue(t.batch, t.key.Status(), true, record.Struct[protocol.TransactionStatus]())
}

func (t *Transaction) signatures(signer *url.URL) Value[*sigSetData] {
	return getOrCreateValue(t.batch, t.key.Signatures(signer), true, record.Struct[sigSetData]())
}

func (t *Transaction) synthTxns() Value[*protocol.TxIdSet] {
	return getOrCreateValue(t.batch, t.key.Synthetic(), true, record.Struct[protocol.TxIdSet]())
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

// GetState loads the transaction state.
func (t *Transaction) GetState() (*SigOrTxn, error) {
	v, err := t.main().Get()
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, errors.StatusNotFound) {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	return nil, errors.FormatWithCause(errors.StatusNotFound, err, "transaction %X not found", t.id)
}

// PutState stores the transaction state.
func (t *Transaction) PutState(v *SigOrTxn) error {
	return t.main().Put(v)
}

// GetStatus loads the transaction status.
func (t *Transaction) GetStatus() (*protocol.TransactionStatus, error) {
	return t.status().Get()
}

// PutStatus stores the transaction status.
func (t *Transaction) PutStatus(v *protocol.TransactionStatus) error {
	if v.Result == nil {
		v.Result = new(protocol.EmptyResult)
	}

	err := t.status().Put(v)
	if err != nil {
		return err
	}

	// TODO Why?
	if !v.Pending() {
		return nil
	}

	// Ensure the principal's BPT entry is up to date
	txn, err := t.GetState()
	if err != nil {
		return err
	}
	return t.batch.Account(txn.Transaction.Header.Principal).putBpt()
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
func (t *Transaction) AddSignature(keyEntryIndex uint64, newSignature protocol.Signature) (int, error) {
	set, err := t.newSigSet(newSignature.GetSigner(), true)
	if err != nil {
		return 0, err
	}

	return set.Add(keyEntryIndex, newSignature)
}

// AddSystemSignature adds a system signature to the operator signature set.
// AddSystemSignature panics if the signature is not a system signature.
func (t *Transaction) AddSystemSignature(net *config.Describe, newSignature protocol.Signature) (int, error) {
	if !newSignature.Type().IsSystem() {
		panic("not a system signature")
	}

	set, err := t.newSigSet(net.OperatorsPage(), true)
	if err != nil {
		return 0, err
	}

	return set.Add(0, newSignature)
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
func (t *Transaction) GetSyntheticTxns() (*protocol.TxIdSet, error) {
	return t.synthTxns().Get()
}

// PutSyntheticTxns stores the IDs of synthetic transactions produced by the
// transaction.
func (t *Transaction) PutSyntheticTxns(v *protocol.TxIdSet) error {
	return t.synthTxns().Put(v)
}

// AddSyntheticTxns is a convenience method that calls GetSyntheticTxns, adds
// the IDs, and calls PutSyntheticTxns.
func (t *Transaction) AddSyntheticTxns(txids ...*url.TxID) error {
	set, err := t.GetSyntheticTxns()
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}

	for _, id := range txids {
		set.Add(id)
	}

	return t.PutSyntheticTxns(set)
}
