// Copyright 2023 The Accumulate Authors
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

func (UpdateKey) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateKey
}

func (UpdateKey) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, md SignatureValidationMetadata) (fallback bool, err error) {
	// Do not allow delegation
	if md.Delegated {
		return false, errors.Unauthorized.WithFormat("cannot %v with a delegated signature", transaction.Body.Type())
	}

	// Can't do much if we're not at the principal
	if !transaction.Header.Principal.LocalTo(md.Location) {
		return true, nil
	}

	// The principal is allowed to sign
	if signer.GetUrl().Equal(transaction.Header.Principal) {
		return false, nil
	}

	// Delegates are allowed to sign
	var page *protocol.KeyPage
	err = batch.Account(transaction.Header.Principal).Main().GetAs(&page)
	if err != nil {
		return false, err
	}
	_, _, ok := page.EntryByDelegate(signer.GetUrl())
	if ok {
		return false, nil
	}

	return false, errors.Unauthorized.WithFormat("%v is not authorized to sign %v for %v", signer.GetUrl(), transaction.Body.Type(), transaction.Header.Principal)
}

func (x UpdateKey) AuthorityIsSatisfied(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus, authority *url.URL) (satisfied, fallback bool, err error) {
	book, _, ok := protocol.ParseKeyPageUrl(transaction.Header.Principal)
	if !ok {
		return false, false, errors.BadRequest.With("principal is not a key page")
	}

	// If the authority is a delegate, fallback to the normal logic
	if !authority.Equal(book) {
		return false, true, nil
	}

	// Otherwise, the principal must submit at least one signature
	_, ok = status.GetSigner(transaction.Header.Principal)
	return ok, false, nil
}

func (x UpdateKey) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	// Wait for the initiator
	if status.Initiator == nil {
		return false, false, nil
	}

	// Did the principal sign?
	signer, ok := status.GetSigner(transaction.Header.Principal)
	if ok {
		if ok, err := x.didVote(batch, transaction, signer.GetAuthority()); err != nil {
			return false, false, errors.UnknownError.Wrap(err)
		} else if ok {
			return true, false, nil
		}
	}

	// Did a delegate sign?
	var page *protocol.KeyPage
	err = batch.Account(transaction.Header.Principal).Main().GetAs(&page)
	if err != nil {
		return false, false, err
	}
	for _, entry := range page.Keys {
		if entry.Delegate == nil {
			continue
		}
		if ok, err := x.didVote(batch, transaction, entry.Delegate); err != nil {
			return false, false, errors.UnknownError.Wrap(err)
		} else if ok {
			return true, false, nil
		}
	}

	return false, false, nil
}

func (UpdateKey) didVote(batch *database.Batch, transaction *protocol.Transaction, book *url.URL) (bool, error) {
	// The book must vote
	_, err := batch.Account(transaction.Header.Principal).
		Transaction(transaction.ID().Hash()).
		Vote(book).
		Get()
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, errors.NotFound):
		return false, nil
	default:
		return false, errors.UnknownError.WithFormat("load vote: %w", err)
	}
}

func (UpdateKey) validate(st *StateManager, tx *Delivery) (*protocol.UpdateKey, *protocol.KeyPage, *protocol.KeyBook, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateKey)
	if !ok {
		return nil, nil, nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateKey), tx.Transaction.Body)
	}
	switch len(body.NewKeyHash) {
	case 0:
		return nil, nil, nil, errors.BadRequest.WithFormat("public key hash is missing")
	case 32:
		// Ok
	default:
		return nil, nil, nil, errors.BadRequest.WithFormat("public key hash length is invalid")
	}

	page, ok := st.Origin.(*protocol.KeyPage)
	if !ok {
		return nil, nil, nil, fmt.Errorf("invalid principal: want account type %v, got %v", protocol.AccountTypeKeyPage, st.Origin.Type())
	}

	bookUrl, _, ok := protocol.ParseKeyPageUrl(st.OriginUrl)
	if !ok {
		return nil, nil, nil, fmt.Errorf("invalid principal: page url is invalid: %s", page.Url)
	}

	var book *protocol.KeyBook
	err := st.LoadUrlAs(bookUrl, &book)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid key book: %v", err)
	}

	if book.BookType == protocol.BookTypeValidator {
		return nil, nil, nil, fmt.Errorf("UpdateKey cannot be used to modify the validator key book")
	}

	return body, page, book, nil
}

func (UpdateKey) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, _, _, err := UpdateKey{}.validate(st, tx)
	return nil, err
}

func (UpdateKey) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, page, book, err := UpdateKey{}.validate(st, tx)
	if err != nil {
		return nil, err
	}

	// Do not update the key page version. Do not reset LastUsedOn.

	txn := st.batch.Transaction(st.txHash[:])
	status, err := txn.Status().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load status: %w", err)
	}

	oldEntry := new(protocol.KeySpecParams)
	i, _, ok := page.EntryByDelegate(status.Initiator)
	switch {
	case ok:
		// Update entry for delegate
		oldEntry.Delegate = page.Keys[i].Delegate

	case status.Initiator.Equal(page.Url):
		// Update entry by key hash
		var msg messaging.MessageWithSignature
		err = st.batch.Message(tx.Transaction.Header.Initiator).Main().GetAs(&msg)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("load initiator signature: %w", err)
		}

		sig, ok := msg.GetSignature().(protocol.KeySignature)
		if !ok {
			return nil, errors.InternalError.WithFormat("invalid initiator: expected key signature, got %v", msg.GetSignature().Type())
		}
		oldEntry.KeyHash = sig.GetPublicKeyHash()

	default:
		return nil, errors.InternalError.WithFormat("initiator %v is neither the principal nor a delegate", status.Initiator)
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

func updateKey(page *protocol.KeyPage, book *protocol.KeyBook, old, new *protocol.KeySpecParams, preserveDelegate bool) error {
	if new.IsEmpty() {
		return fmt.Errorf("cannot add an empty entry")
	}

	if new.Delegate != nil {
		if new.Delegate.ParentOf(page.Url) {
			return fmt.Errorf("self-delegation is not allowed")
		}

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
