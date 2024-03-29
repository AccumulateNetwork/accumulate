// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateKey struct{}

var _ SignerValidator = UpdateKey{}
var _ AuthorityValidator = UpdateKey{}

func (UpdateKey) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateKey
}

func (UpdateKey) AuthorityIsAccepted(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, sig *protocol.AuthoritySignature) (fallback bool, err error) {
	// The principal is allowed to sign
	if sig.Origin.Equal(transaction.Header.Principal) {
		return false, nil
	}

	// Delegates are allowed to sign
	var page *protocol.KeyPage
	err = batch.Account(transaction.Header.Principal).Main().GetAs(&page)
	if err != nil {
		return false, err
	}
	_, _, ok := page.EntryByDelegate(sig.Authority)
	if ok {
		return false, nil
	}

	return false, errors.Unauthorized.WithFormat("%v is not authorized to sign %v for %v", sig.Origin, transaction.Body.Type(), transaction.Header.Principal)
}

func (x UpdateKey) AuthorityWillVote(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, authority *url.URL) (fallback bool, vote *AuthVote, err error) {
	// If the authority is a delegate, fallback to the normal logic
	if !authority.ParentOf(transaction.Header.Principal) {
		return true, nil, nil
	}

	// Load the principal's signatures
	entries, err := batch.
		Account(transaction.Header.Principal).
		Transaction(transaction.ID().Hash()).
		Signatures().
		Get()
	if err != nil {
		return false, nil, errors.UnknownError.WithFormat("load %v signers: %w", transaction.ID(), err)
	}

	for _, entry := range entries {
		sig, err := delegate.GetSignatureAs(batch, entry.Hash)
		if err != nil {
			return false, nil, errors.UnknownError.WithFormat("load %v: %w", transaction.Header.Principal.WithTxID(entry.Hash), err)
		}

		// Ignore any signatures that are not the initiator
		if ok, _ := protocol.SignatureDidInitiate(sig, transaction.Header.Initiator[:], nil); ok {
			// Initiator received, transaction is ready
			v := &AuthVote{Source: sig.GetSigner(), Vote: sig.GetVote()}
			return false, v, nil
		}
	}

	// Not ready
	return false, nil, nil
}

func (x UpdateKey) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction) (ready, fallback bool, err error) {
	// Wait for the initiator
	isInit, pay, err := delegate.TransactionIsInitiated(batch, transaction)
	if err != nil {
		return false, false, errors.UnknownError.Wrap(err)
	}
	if !isInit {
		return false, false, nil
	}

	// If the initiator is the principal, the transaction is ready once the
	// book's authority signature is received
	if pay.Payer.Equal(transaction.Header.Principal) {
		ok, vote, err := delegate.AuthorityDidVote(batch, transaction, transaction.Header.Principal.Identity())
		switch {
		case !ok || err != nil:
			return false, false, err
		case vote == protocol.VoteTypeAccept:
			return true, false, nil
		default:
			// Any vote that is not accept is counted as reject. Since a
			// transaction is only executed once _all_ authorities accept, a
			// single authority rejecting or abstaining is sufficient to block
			// the transaction.
			return false, false, errors.Rejected
		}
	}

	// If the initiator is a delegate, the transaction is ready once the
	// delegate's authority signature is received
	var page *protocol.KeyPage
	err = batch.Account(transaction.Header.Principal).Main().GetAs(&page)
	if err != nil {
		return false, false, err
	}
	_, entry, ok := page.EntryByDelegate(pay.Payer)
	if !ok {
		// If the initiator is neither the principal nor a delegate, it is not
		// authorized
		return false, false, errors.Unauthorized.WithFormat("%v is not authorized to initiate %v for %v", pay.Payer, transaction.Body.Type(), transaction.Header.Principal)
	}
	ok, vote, err := delegate.AuthorityDidVote(batch, transaction, entry.(*protocol.KeySpec).Delegate)
	switch {
	case !ok || err != nil:
		return false, false, err
	case vote == protocol.VoteTypeAccept:
		return true, false, nil
	default:
		// Any vote that is not accept is counted as reject. Since a transaction
		// is only executed once _all_ authorities accept, a single authority
		// rejecting or abstaining is sufficient to block the transaction.
		return false, false, errors.Rejected
	}
}

func (UpdateKey) check(_ *StateManager, tx *Delivery) (*protocol.UpdateKey, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateKey)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateKey), tx.Transaction.Body)
	}

	err := requireKeyHash(body.NewKeyHash)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

	_, _, ok = protocol.ParseKeyPageUrl(tx.Transaction.Header.Principal)
	if !ok {
		return nil, fmt.Errorf("invalid principal: page url is invalid: %s", tx.Transaction.Header.Principal)
	}

	return body, nil
}

func (UpdateKey) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := UpdateKey{}.check(st, tx)
	return nil, err
}

func (UpdateKey) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := UpdateKey{}.check(st, tx)
	if err != nil {
		return nil, err
	}

	page, ok := st.Origin.(*protocol.KeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid principal: want account type %v, got %v", protocol.AccountTypeKeyPage, st.Origin.Type())
	}

	var book *protocol.KeyBook
	err = st.LoadUrlAs(page.GetAuthority(), &book)
	if err != nil {
		return nil, fmt.Errorf("invalid key book: %v", err)
	}

	if book.BookType == protocol.BookTypeValidator {
		return nil, fmt.Errorf("UpdateKey cannot be used to modify the validator key book")
	}

	// Do not update the key page version, do not reset LastUsedOn

	isInit, initiator, err := st.AuthDelegate.TransactionIsInitiated(st.batch, tx.Transaction)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	if !isInit {
		return nil, errors.InternalError.With("transaction has not been initiated")
	}

	oldEntry := new(protocol.KeySpecParams)
	i, _, ok := page.EntryByDelegate(initiator.Payer)
	switch {
	case ok:
		// Update entry for delegate
		oldEntry.Delegate = page.Keys[i].Delegate

	case initiator.Payer.Equal(page.Url):
		// Update entry by key hash
		var msg messaging.MessageWithSignature
		err = st.batch.Message(initiator.Cause.Hash()).Main().GetAs(&msg)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load initiator signature: %w", err)
		}

		sig, ok := msg.GetSignature().(protocol.KeySignature)
		if !ok {
			return nil, errors.InternalError.WithFormat("invalid initiator: expected key signature, got %v", msg.GetSignature().Type())
		}
		oldEntry.KeyHash = sig.GetPublicKeyHash()

	default:
		return nil, errors.InternalError.WithFormat("initiator %v is neither the principal nor a delegate", initiator)
	}

	err = updateKey(page, book, oldEntry, &protocol.KeySpecParams{KeyHash: body.NewKeyHash}, true)
	if err != nil {
		return nil, err
	}

	// Store the update, but do not change the page version
	err = st.Update(page)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", page.GetUrl(), err)
	}
	return nil, nil
}

func requireKeyHash(h []byte) error {
	if len(h) == 0 {
		return errors.BadRequest.WithFormat("public key hash is missing")
	}
	if len(h) > 32 {
		return errors.BadRequest.WithFormat("public key hash is too long to be a hash")
	}
	return nil
}

func updateKey(page *protocol.KeyPage, book *protocol.KeyBook, old, new *protocol.KeySpecParams, preserveDelegate bool) error {
	if new.Delegate != nil {
		if err := verifyIsNotPage(&book.AccountAuth, new.Delegate); err != nil {
			return errors.UnknownError.WithFormat("invalid delegate %v: %w", new.Delegate, err)
		}
	}

	// Find the old entry
	oldPos, entry, found := findKeyPageEntry(page, old)
	if !found {
		return fmt.Errorf("entry to be updated not found on the key page")
	}

	// Check for an existing key with same delegate
	newPos, _, found := findKeyPageEntry(page, new)
	if found && oldPos != newPos {
		return fmt.Errorf("cannot have duplicate entries on key page")
	}

	// Update the entry
	entry.PublicKeyHash = new.KeyHash

	if new.Delegate != nil || !preserveDelegate {
		entry.Delegate = new.Delegate
	}

	// Relocate the entry
	page.RemoveKeySpecAt(oldPos)
	page.AddKeySpec(entry)
	return nil
}
