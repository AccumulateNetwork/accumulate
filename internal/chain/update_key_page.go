package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateKeyPage struct{}

func (UpdateKeyPage) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateKeyPage
}

func (UpdateKeyPage) SignerIsAuthorized(batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer) (fallback bool, err error) {
	principalBook, principalPageIdx, ok := protocol.ParseKeyPageUrl(transaction.Header.Principal)
	if !ok {
		return false, errors.Format(errors.StatusBadRequest, "principal is not a key page")
	}

	signerBook, signerPageIdx, ok := protocol.ParseKeyPageUrl(signer.GetUrl())
	if !ok {
		return true, nil // Signer is not a page
	}

	if !principalBook.Equal(signerBook) {
		return true, nil // Signer belongs to a different book
	}

	// Lower indices are higher priority
	if signerPageIdx > principalPageIdx {
		return false, errors.Format(errors.StatusUnauthorized, "signer %v is lower priority than the principal %v", signer.GetUrl(), transaction.Header.Principal)
	}

	// Operation-specific checks
	body, ok := transaction.Body.(*protocol.UpdateKeyPage)
	if !ok {
		return false, errors.Format(errors.StatusBadRequest, "invalid payload: want %T, got %T", new(protocol.UpdateKeyPage), transaction.Body)
	}
	for _, op := range body.Operation {
		switch op.Type() {
		case protocol.KeyPageOperationTypeUpdateAllowed:
			if signerPageIdx == principalPageIdx {
				return false, errors.Format(errors.StatusUnauthorized, "%v cannot modify its own allowed operations", transaction.Header.Principal)
			}
		}
	}

	// Run the normal checks
	return true, nil
}

func (UpdateKeyPage) TransactionIsReady(*database.Batch, *protocol.Transaction, *protocol.TransactionStatus) (ready, fallback bool, err error) {
	// Do not override the ready check
	return false, true, nil
}

func (UpdateKeyPage) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (UpdateKeyPage{}).Validate(st, tx)
}

func (UpdateKeyPage) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateKeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateKeyPage), tx.Transaction.Body)
	}

	page, ok := st.Origin.(*protocol.KeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", protocol.AccountTypeKeyPage, st.Origin.Type())
	}

	bookUrl, _, ok := protocol.ParseKeyPageUrl(st.OriginUrl)
	if !ok {
		return nil, fmt.Errorf("invalid origin record: page url is invalid: %s", page.Url)
	}

	var book *protocol.KeyBook
	err := st.LoadUrlAs(bookUrl, &book)
	if err != nil {
		return nil, fmt.Errorf("invalid key book: %v", err)
	}

	if st.nodeUrl.JoinPath(protocol.ValidatorBook).Equal(book.Url) {
		return nil, fmt.Errorf("UpdateKeyPage cannot be used to modify the validator key book")
	}

	for _, op := range body.Operation {
		err = UpdateKeyPage{}.executeOperation(page, op)
		if err != nil {
			return nil, err
		}
	}

	didUpdateKeyPage(page)
	err = st.Update(page)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", page.GetUrl(), err)
	}
	return nil, nil
}

func (UpdateKeyPage) executeOperation(page *protocol.KeyPage, op protocol.KeyPageOperation) error {
	switch op := op.(type) {
	case *protocol.AddKeyOperation:
		if op.Entry.IsEmpty() {
			return fmt.Errorf("cannot add an empty entry")
		}

		_, _, found := findKeyPageEntry(page, &op.Entry)
		if found {
			return fmt.Errorf("cannot have duplicate entries on key page")
		}

		entry := new(protocol.KeySpec)
		entry.PublicKeyHash = op.Entry.KeyHash
		entry.Owner = op.Entry.Owner
		page.Keys = append(page.Keys, entry)
		return nil

	case *protocol.RemoveKeyOperation:
		index, _, found := findKeyPageEntry(page, &op.Entry)
		if !found {
			return fmt.Errorf("entry to be removed not found on the key page")
		}

		page.Keys = append(page.Keys[:index], page.Keys[index+1:]...)

		_, pageIndex, ok := protocol.ParseKeyPageUrl(page.Url)
		if !ok {
			return errors.Format(errors.StatusInternalError, "principal is not a key page")
		}
		if len(page.Keys) == 0 && pageIndex == 0 {
			return fmt.Errorf("cannot delete last key of the highest priority page of a key book")
		}

		if page.AcceptThreshold > uint64(len(page.Keys)) {
			page.AcceptThreshold = uint64(len(page.Keys))
		}
		return nil

	case *protocol.UpdateKeyOperation:
		if op.NewEntry.IsEmpty() {
			return fmt.Errorf("cannot add an empty entry")
		}

		_, entry, found := findKeyPageEntry(page, &op.OldEntry)
		if !found {
			return fmt.Errorf("entry to be updated not found on the key page")
		}

		_, _, found = findKeyPageEntry(page, &op.NewEntry)
		if found {
			return fmt.Errorf("cannot have duplicate entries on key page")
		}

		entry.PublicKeyHash = op.NewEntry.KeyHash
		entry.Owner = op.NewEntry.Owner
		return nil

	case *protocol.SetThresholdKeyPageOperation:
		return page.SetThreshold(op.Threshold)

	case *protocol.UpdateAllowedKeyPageOperation:
		if page.TransactionBlacklist == nil {
			page.TransactionBlacklist = new(protocol.AllowedTransactions)
		}

		for _, txn := range op.Allow {
			bit, ok := txn.AllowedTransactionBit()
			if !ok {
				return fmt.Errorf("transaction type %v cannot be (dis)allowed", txn)
			}
			page.TransactionBlacklist.Clear(bit)
		}

		for _, txn := range op.Deny {
			bit, ok := txn.AllowedTransactionBit()
			if !ok {
				return fmt.Errorf("transaction type %v cannot be (dis)allowed", txn)
			}
			page.TransactionBlacklist.Set(bit)
		}

		if *page.TransactionBlacklist == 0 {
			page.TransactionBlacklist = nil
		}
		return nil

	default:
		return fmt.Errorf("invalid operation: %v", op.Type())
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
	} else if search.Owner != nil {
		i, entry, ok = page.EntryByDelegate(search.Owner)
	} else {
		return -1, nil, false
	}

	var keySpec *protocol.KeySpec
	if ok {
		// If this is not true, something is seriously wrong
		keySpec = entry.(*protocol.KeySpec)
	}
	return i, keySpec, ok
}
