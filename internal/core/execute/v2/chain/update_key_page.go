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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateKeyPage struct{}

var _ SignerValidator = (*UpdateKeyPage)(nil)

func (UpdateKeyPage) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateKeyPage
}

func (UpdateKeyPage) AuthorityIsAccepted(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, sig *protocol.AuthoritySignature) (fallback bool, err error) {
	principalBook, principalPageIdx, ok := protocol.ParseKeyPageUrl(transaction.Header.Principal)
	if !ok {
		return false, errors.BadRequest.WithFormat("principal is not a key page")
	}

	signerBook, signerPageIdx, ok := protocol.ParseKeyPageUrl(sig.Origin)
	if !ok {
		return true, nil // Signer is not a page
	}
	if !sig.Authority.Equal(signerBook) {
		return false, errors.InternalError.WithFormat("%v is not a page of %v", sig.Origin, sig.Authority)
	}

	// If the signer is a page of the principal
	if principalBook.Equal(sig.Authority) {
		// Lower indices are higher priority
		if signerPageIdx > principalPageIdx {
			return false, errors.Unauthorized.WithFormat("signer %v is lower priority than the principal %v", sig.Origin, transaction.Header.Principal)
		}

		// Operation-specific checks
		body, ok := transaction.Body.(*protocol.UpdateKeyPage)
		if !ok {
			return false, errors.BadRequest.WithFormat("invalid payload: want %T, got %T", new(protocol.UpdateKeyPage), transaction.Body)
		}
		for _, op := range body.Operation {
			switch op.Type() {
			case protocol.KeyPageOperationTypeUpdateAllowed:
				if signerPageIdx == principalPageIdx {
					return false, errors.Unauthorized.WithFormat("%v cannot modify its own allowed operations", transaction.Header.Principal)
				}
			}
		}
	}

	// Signers belonging to new delegates are authorized to sign the transaction
	newOwners, err := updateKeyPage_getNewOwners(batch, transaction)
	if err != nil {
		return false, err
	}

	return newOwners.AuthorityIsAccepted(delegate, batch, transaction, sig)
}

func (UpdateKeyPage) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction) (ready, fallback bool, err error) {
	// All new delegates must sign the transaction
	newOwners, err := updateKeyPage_getNewOwners(batch, transaction)
	if err != nil {
		return false, false, errors.UnknownError.Wrap(err)
	}

	return newOwners.TransactionIsReady(delegate, batch, transaction)
}

func (x UpdateKeyPage) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (x UpdateKeyPage) check(st *StateManager, tx *Delivery) (*protocol.UpdateKeyPage, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateKeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateKeyPage), tx.Transaction.Body)
	}

	_, _, ok = protocol.ParseKeyPageUrl(tx.Transaction.Header.Principal)
	if !ok {
		return nil, fmt.Errorf("invalid principal: page url is invalid: %s", tx.Transaction.Header.Principal)
	}

	for _, op := range body.Operation {
		err := x.checkOperation(tx, op)
		if err != nil {
			return nil, err
		}
	}

	return body, nil
}

func (x UpdateKeyPage) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
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
		return nil, fmt.Errorf("UpdateKeyPage cannot be used to modify the validator key book")
	}

	for _, op := range body.Operation {
		err = x.executeOperation(page, book, op)
		if err != nil {
			return nil, err
		}
	}

	if len(page.Keys) > int(st.Globals.Globals.Limits.PageEntries) {
		return nil, errors.BadRequest.WithFormat("page will have too many entries")
	}

	didUpdateKeyPage(page)
	err = st.Update(page)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", page.GetUrl(), err)
	}

	return nil, nil
}

func (UpdateKeyPage) checkOperation(tx *Delivery, op protocol.KeyPageOperation) error {
	switch op := op.(type) {
	case *protocol.AddKeyOperation:
		if op.Entry.IsEmpty() {
			return errors.BadRequest.With("cannot add an empty entry")
		}

		if op.Entry.Delegate != nil {
			if op.Entry.Delegate.ParentOf(tx.Transaction.Header.Principal) {
				return errors.BadRequest.With("self-delegation is not allowed")
			}
		}

		return nil

	case *protocol.RemoveKeyOperation:
		if op.Entry.IsEmpty() {
			return errors.BadRequest.With("cannot add an empty entry")
		}
		return nil

	case *protocol.UpdateKeyOperation:
		if op.OldEntry.IsEmpty() {
			return errors.BadRequest.With("cannot update: old entry is empty")
		}
		if op.NewEntry.IsEmpty() {
			return errors.BadRequest.With("cannot update: new entry is empty")
		}
		if op.NewEntry.Delegate != nil && op.NewEntry.Delegate.ParentOf(tx.Transaction.Header.Principal) {
			return fmt.Errorf("self-delegation is not allowed")
		}
		return nil

	case *protocol.SetThresholdKeyPageOperation:
		if op.Threshold == 0 {
			return errors.BadRequest.With("cannot require 0 signatures on a key page")
		}
		return nil

	case *protocol.UpdateAllowedKeyPageOperation:
		for _, txn := range op.Allow {
			_, ok := txn.AllowedTransactionBit()
			if !ok {
				return errors.BadRequest.WithFormat("transaction type %v cannot be (dis)allowed", txn)
			}
		}

		for _, txn := range op.Deny {
			_, ok := txn.AllowedTransactionBit()
			if !ok {
				return errors.BadRequest.WithFormat("transaction type %v cannot be (dis)allowed", txn)
			}
		}

		return nil

	default:
		return errors.BadRequest.WithFormat("invalid operation: %v", op.Type())
	}
}

func (UpdateKeyPage) executeOperation(page *protocol.KeyPage, book *protocol.KeyBook, op protocol.KeyPageOperation) error {
	switch op := op.(type) {
	case *protocol.AddKeyOperation:
		if op.Entry.Delegate != nil {
			if err := verifyIsNotPage(&book.AccountAuth, op.Entry.Delegate); err != nil {
				return errors.UnknownError.WithFormat("invalid delegate %v: %w", op.Entry.Delegate, err)
			}
		}

		_, _, found := findKeyPageEntry(page, &op.Entry)
		if found {
			return errors.UnknownError.With("cannot have duplicate entries on key page")
		}

		entry := new(protocol.KeySpec)
		entry.PublicKeyHash = op.Entry.KeyHash
		entry.Delegate = op.Entry.Delegate
		page.AddKeySpec(entry)
		return nil

	case *protocol.RemoveKeyOperation:
		index, _, found := findKeyPageEntry(page, &op.Entry)
		if !found {
			return errors.UnknownError.With("entry to be removed not found on the key page")
		}

		_, pageIndex, ok := protocol.ParseKeyPageUrl(page.Url)
		if !ok {
			return errors.InternalError.WithFormat("principal is not a key page")
		}
		if len(page.Keys) == 1 && pageIndex == 1 {
			return errors.UnknownError.With("cannot delete last key of the highest priority page of a key book")
		}

		page.RemoveKeySpecAt(index)

		if page.AcceptThreshold > uint64(len(page.Keys)) {
			page.AcceptThreshold = uint64(len(page.Keys))
		}
		return nil

	case *protocol.UpdateKeyOperation:
		return updateKey(page, book, &op.OldEntry, &op.NewEntry, false)

	case *protocol.SetThresholdKeyPageOperation:
		return page.SetThreshold(op.Threshold)

	case *protocol.UpdateAllowedKeyPageOperation:
		if page.TransactionBlacklist == nil {
			page.TransactionBlacklist = new(protocol.AllowedTransactions)
		}

		for _, txn := range op.Allow {
			bit, ok := txn.AllowedTransactionBit()
			if !ok {
				return errors.InternalError.WithFormat("transaction type %v cannot be (dis)allowed", txn)
			}
			page.TransactionBlacklist.Clear(bit)
		}

		for _, txn := range op.Deny {
			bit, ok := txn.AllowedTransactionBit()
			if !ok {
				return errors.InternalError.WithFormat("transaction type %v cannot be (dis)allowed", txn)
			}
			page.TransactionBlacklist.Set(bit)
		}

		if *page.TransactionBlacklist == 0 {
			page.TransactionBlacklist = nil
		}
		return nil

	default:
		return errors.InternalError.WithFormat("invalid operation: %v", op.Type())
	}
}

func didUpdateKeyPage(page *protocol.KeyPage) {
	page.Version += 1

	// We're changing the height of the key page, so reset all the nonces
	for _, key := range page.Keys {
		key.LastUsedOn = 0
	}
}

func findKeyPageEntry(page *protocol.KeyPage, search *protocol.KeySpecParams) (int, *protocol.KeySpec, bool) {
	var i int
	var entry protocol.KeyEntry
	var ok bool
	if len(search.KeyHash) > 0 {
		i, entry, ok = page.EntryByKeyHash(search.KeyHash)
	}
	if !ok && search.Delegate != nil {
		i, entry, ok = page.EntryByDelegate(search.Delegate)
	}
	if !ok {
		return -1, nil, false
	}

	var keySpec *protocol.KeySpec
	if ok {
		// If this is not true, something is seriously wrong
		keySpec = entry.(*protocol.KeySpec)
	}
	return i, keySpec, ok
}

func updateKeyPage_getNewOwners(batch *database.Batch, transaction *protocol.Transaction) (additionalAuthorities, error) {
	body, ok := transaction.Body.(*protocol.UpdateKeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateKeyPage), transaction.Body)
	}

	var page *protocol.KeyPage
	err := batch.Account(transaction.Header.Principal).Main().GetAs(&page)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load principal: %w", err)
	}

	var owners []*url.URL
	for _, op := range body.Operation {
		switch op := op.(type) {
		case *protocol.AddKeyOperation:
			if op.Entry.Delegate == nil {
				continue
			}

			owners = append(owners, op.Entry.Delegate)

		case *protocol.UpdateKeyOperation:
			// Don't check if the new entry does not have an owner
			if op.NewEntry.Delegate == nil {
				continue
			}

			owners = append(owners, op.NewEntry.Delegate)

		default:
			continue
		}
	}

	return owners, nil
}
