package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
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
	if len(body.NewKeyHash) != 32 {
		return nil, nil, nil, errors.New(errors.StatusBadRequest, "key hash is not a valid length for a SHA-256 hash")
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

	if status.Initiator == nil {
		// This should be impossible
		return nil, fmt.Errorf("transaction does not have an initiator")
	}

	sigs, err := txObj.ReadSignatures(status.Initiator)
	if err != nil {
		return nil, fmt.Errorf("load signatures for %v: %w", status.Initiator, err)
	}

	if sigs.Count() == 0 {
		// This should never happen, because this transaction is not multisig.
		// But it's possible (maybe) for the initiator to be invalidated by the
		// key page version changing.
		return nil, fmt.Errorf("no valid signatures found for %v", status.Initiator)
	}

	e := sigs.Entries()[0]
	sigOrTxn, err := st.batch.Transaction(e.SignatureHash[:]).GetState()
	if err != nil {
		return nil, fmt.Errorf("load first signature from %v: %w", status.Initiator, err)
	}
	if sigOrTxn.Signature == nil {
		// This should be impossible
		return nil, fmt.Errorf("invalid signature state")
	}
	signature := sigOrTxn.Signature
	control := true
	for control {
		if signature.Type() == protocol.SignatureTypeDelegated {
			signature = signature.(*protocol.DelegatedSignature).Signature
		}
		control = false
	}
	keysig, ok := signature.(protocol.KeySignature)
	if !ok {
		return nil, fmt.Errorf("signature is not a key signature")
	}

	err = updateKey(page, book,
		&protocol.KeySpecParams{KeyHash: keysig.GetPublicKeyHash()},
		&protocol.KeySpecParams{KeyHash: body.NewKeyHash})
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

func updateKey(page *protocol.KeyPage, book *protocol.KeyBook, old, new *protocol.KeySpecParams) error {
	if new.IsEmpty() {
		return fmt.Errorf("cannot add an empty entry")
	}

	if new.Delegate != nil {
		if new.Delegate.ParentOf(page.Url) {
			return fmt.Errorf("self-delegation is not allowed")
		}

		if err := verifyIsNotPage(&book.AccountAuth, new.Delegate); err != nil {
			return errors.Format(errors.StatusUnknownError, "invalid delegate %v: %w", new.Delegate, err)
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
	entry.Delegate = new.Delegate

	// Relocate the entry
	page.RemoveKeySpecAt(oldPos)
	page.AddKeySpec(entry)
	return nil
}
