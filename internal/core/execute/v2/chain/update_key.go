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
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateKey struct{}

func (UpdateKey) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateKey
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

	// Find the first signature
	txObj := st.batch.Transaction(tx.Transaction.GetHash())
	status, err := txObj.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("load transaction status: %w", err)
	}

	var initiator protocol.Signature
	for _, signer := range status.Signers {
		sigs, err := database.GetSignaturesForSigner(txObj, signer)
		if err != nil {
			return nil, fmt.Errorf("load signatures for %v: %w", signer.GetUrl(), err)
		}

		for _, sig := range sigs {
			if protocol.SignatureDidInitiate(sig, tx.Transaction.Header.Initiator[:], &initiator) {
				goto found_init
			}
		}
	}
	return nil, errors.InternalError.WithFormat("unable to locate initiator signature")

found_init:
	switch initiator := initiator.(type) {
	case protocol.KeySignature:
		err = updateKey(page, book,
			&protocol.KeySpecParams{KeyHash: initiator.GetPublicKeyHash()},
			&protocol.KeySpecParams{KeyHash: body.NewKeyHash}, true)

	case *protocol.DelegatedSignature:
		err = updateKey(page, book,
			&protocol.KeySpecParams{Delegate: initiator.GetSigner()},
			&protocol.KeySpecParams{KeyHash: body.NewKeyHash}, true)

		if _, ok := initiator.Signature.(protocol.KeySignature); !ok {
			return nil, fmt.Errorf("cannot UpdateKey with a multi-level delegated signature")
		}
	default:
		return nil, errors.InternalError.WithFormat("%v does not support %v signatures", protocol.TransactionTypeUpdateKey, initiator.Type())

	}
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
