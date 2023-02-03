// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/internal"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func (b *bundle) ProcessRemoteSignatures() error {
	var transactions []*protocol.SyntheticForwardTransaction
	txnIndex := map[[32]byte]int{}
	signerSeen := map[[32]byte]bool{}

	batch := b.Block.Batch.Begin(false)
	defer batch.Discard()

	for _, msg := range b.messages {
		if fwd, ok := msg.(*internal.ForwardedMessage); ok {
			msg = fwd.Message
		}
		sig, ok := msg.(*messaging.UserSignature)
		if !ok {
			continue
		}

		// Load the transaction. Earlier checks should guarantee this never fails.
		txn, err := batch.Transaction(sig.TransactionHash[:]).Main().Get()
		switch {
		case err != nil:
			return errors.InternalError.WithFormat("load transaction: %w", err)
		case txn.Transaction == nil:
			return errors.InternalError.WithFormat("%x is not a transaction", sig.TransactionHash)
		}

		// Synthetic transactions are never remote
		if !txn.Transaction.Body.Type().IsUser() {
			continue
		}

		_, fwd, err := b.Executor.shouldForwardSignature(batch, txn.Transaction, sig.Signature, txn.Transaction.Header.Principal, signerSeen)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
		if fwd == nil {
			continue
		}

		fwd.Cause = append(fwd.Cause, sig.ID().Hash())
		if fwd.Destination == nil {
			fwd.Destination = txn.Transaction.Header.Principal
		}

		if i, ok := txnIndex[fwd.Destination.AccountID32()]; ok {
			transaction := transactions[i]
			transaction.Signatures = append(transaction.Signatures, *fwd)
		}

		transaction := new(protocol.SyntheticForwardTransaction)
		transaction.Transaction = txn.Transaction
		transaction.Signatures = append(transaction.Signatures, *fwd)
		txnIndex[fwd.Destination.AccountID32()] = len(transactions)
		transactions = append(transactions, transaction)

		state, ok := b.state.Get(sig.TransactionHash)
		if !ok {
			state = new(chain.ProcessTransactionState)
			b.state.Set(sig.TransactionHash, state)
		}
		state.DidProduceTxn(fwd.Destination, transaction)
	}

	return nil
}

func (x *Executor) shouldForwardSignature(batch *database.Batch, transaction *protocol.Transaction, signature protocol.Signature, destination *url.URL, seen map[[32]byte]bool) (*url.URL, *protocol.RemoteSignature, error) {
	var signerUrl *url.URL
	switch signature := signature.(type) {
	case *protocol.DelegatedSignature:
		// Check inner signature
		s, fwd, err := x.shouldForwardSignature(batch, transaction, signature.Signature, signature.Delegator, seen)
		if err != nil {
			return nil, nil, errors.UnknownError.Wrap(err)
		}
		if fwd != nil {
			delegated := signature.Copy()
			delegated.Signature = fwd.Signature
			fwd.Signature = delegated
			return s, fwd, nil
		}

		signerUrl = signature.Delegator

	case *protocol.RemoteSignature:
		return x.shouldForwardSignature(batch, transaction, signature.Signature, destination, seen)

	case *protocol.SignatureSet:
		// Already forwarded
		return nil, nil, nil

	case protocol.KeySignature:
		signerUrl = signature.GetSigner()
	}

	if key, _, _ := protocol.ParseLiteTokenAddress(signerUrl); key != nil {
		signerUrl = signerUrl.RootIdentity()
	}

	// Signer is remote?
	if signerUrl.LocalTo(destination) {
		return signerUrl, nil, nil
	}

	if seen[signerUrl.AccountID32()] {
		return nil, nil, nil
	} else {
		seen[signerUrl.AccountID32()] = true
	}

	signer, err := loadSigner(batch, signerUrl)
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load signer: %w", err)
	}

	// Signer is satisfied?
	record := batch.Transaction(transaction.GetHash())
	status, err := record.GetStatus()
	if err != nil {
		return nil, nil, errors.UnknownError.WithFormat("load transaction status: %w", err)
	}

	ready, err := x.SignerIsSatisfied(batch, transaction, status, signer)
	if !ready || err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}

	// Load all of the signatures
	sigset, err := database.GetSignaturesForSigner(batch.Transaction(transaction.GetHash()), signer)
	if err != nil {
		return nil, nil, errors.UnknownError.Wrap(err)
	}

	set := new(protocol.SignatureSet)
	set.Vote = protocol.VoteTypeAccept
	set.Signer = signerUrl
	set.TransactionHash = *(*[32]byte)(transaction.GetHash())
	set.Signatures = sigset

	fwd := new(protocol.RemoteSignature)
	fwd.Destination = destination
	fwd.Signature = set
	return signerUrl, fwd, nil
}
