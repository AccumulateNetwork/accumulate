// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (r *Transaction) hash32() [32]byte {
	h := r.key.Get(1).([32]byte)
	return h
}

func (r *Transaction) hash() []byte {
	h := r.key.Get(1).([32]byte)
	return h[:]
}

// ensureSigner ensures that the transaction's status includes the given signer.
func (t *Transaction) ensureSigner(signer protocol.Signer2) error {
	status, err := t.Status().Get()
	if err != nil {
		return err
	}

	status.AddSigner(signer)
	return t.Status().Put(status)
}

func (t *Transaction) Status() values.Value[*protocol.TransactionStatus] {
	return &transactionStatus{t.getStatus(), t}
}

type transactionStatus struct {
	values.Value[*protocol.TransactionStatus]
	parent *Transaction
}

func (t *transactionStatus) Put(v *protocol.TransactionStatus) error {
	if v.Result == nil {
		v.Result = new(protocol.EmptyResult)
	}

	err := t.Value.Put(v)
	if err != nil {
		return err
	}

	// TODO Why?
	if !v.Pending() {
		return nil
	}

	// Ensure the principal's BPT entry is up to date (if the message is a
	// transaction)
	msg, err := t.parent.parent.Message(t.parent.hash32()).Main().Get()
	if err != nil {
		return err
	}
	txn, ok := msg.(*messaging.TransactionMessage)
	if !ok {
		return nil
	}
	return t.parent.parent.Account(txn.Transaction.Header.Principal).putBpt()
}

// RestoreSignatureSets is specifically only to be used to restore a
// transaction's signature sets from a snapshot.
func (t *Transaction) RestoreSignatureSets(signer *url.URL, version uint64, entries []SigSetEntry) error {
	return t.getSignatures(signer).Put(&sigSetData{Version: version, Entries: entries})
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
func (t *Transaction) SignaturesForSigner(signer protocol.Signer2) (*SignatureSet, error) {
	set, err := newSigSet(t, signer, true)
	if err != nil {
		return nil, fmt.Errorf("load signature set: %w", err)
	}

	return set, nil
}

// SignaturesForSigner returns a read-only signature set for the given signer account.
func (t *Transaction) ReadSignaturesForSigner(signer protocol.Signer2) (*SignatureSet, error) {
	set, err := newSigSet(t, signer, false)
	if err != nil {
		return nil, fmt.Errorf("load signature set: %w", err)
	}

	return set, nil
}

// AddSignature loads the appropriate signature set and adds the signature to
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
func (t *Transaction) AddSystemSignature(net config.NetworkUrl, newSignature protocol.Signature) (int, error) {
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
	err := t.parent.Account(signer).Main().GetAs(&acct)
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
