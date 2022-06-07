package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateKeyPage struct{}

var _ SignerValidator = (*UpdateKeyPage)(nil)

func (UpdateKeyPage) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateKeyPage
}

func (UpdateKeyPage) SignerIsAuthorized(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, signer protocol.Signer, checkAuthz bool) (fallback bool, err error) {
	principalBook, principalPageIdx, ok := protocol.ParseKeyPageUrl(transaction.Header.Principal)
	if !ok {
		return false, errors.Format(errors.StatusBadRequest, "principal is not a key page")
	}

	signerBook, signerPageIdx, ok := protocol.ParseKeyPageUrl(signer.GetUrl())
	if !ok {
		return true, nil // Signer is not a page
	}

	// If the signer is a page of the principal
	if principalBook.Equal(signerBook) {
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
	}

	// Signers belonging to new delegates are authorized to sign the transaction
	newOwners, err := getNewOwners(batch, transaction)
	if err != nil {
		return false, errors.Wrap(errors.StatusUnknown, err)
	}

	for _, owner := range newOwners {
		if owner.Equal(signerBook) {
			return false, delegate.SignerIsAuthorized(batch, transaction, signer, false)
		}
	}

	// Run the normal checks
	return true, nil
}

func (UpdateKeyPage) TransactionIsReady(delegate AuthDelegate, batch *database.Batch, transaction *protocol.Transaction, status *protocol.TransactionStatus) (ready, fallback bool, err error) {
	// All new delegates must sign the transaction
	newOwners, err := getNewOwners(batch, transaction)
	if err != nil {
		return false, false, errors.Wrap(errors.StatusUnknown, err)
	}

	for _, owner := range newOwners {
		ok, err := delegate.AuthorityIsSatisfied(batch, transaction, status, owner)
		if !ok || err != nil {
			return false, false, err
		}
	}

	// Fallback to general authorization
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
		return nil, fmt.Errorf("invalid principal: want account type %v, got %v", protocol.AccountTypeKeyPage, st.Origin.Type())
	}

	bookUrl, _, ok := protocol.ParseKeyPageUrl(st.OriginUrl)
	if !ok {
		return nil, fmt.Errorf("invalid principal: page url is invalid: %s", page.Url)
	}

	var book *protocol.KeyBook
	err := st.LoadUrlAs(bookUrl, &book)
	if err != nil {
		return nil, fmt.Errorf("invalid key book: %v", err)
	}

	if book.BookType == protocol.BookTypeValidator {
		return nil, fmt.Errorf("UpdateKeyPage cannot be used to modify the validator key book")
	}

	for _, op := range body.Operation {
		err = UpdateKeyPage{}.executeOperation(page, op)
		if err != nil {
			return nil, err
		}
	}

	isDnOprBook := protocol.IsDnUrl(st.NodeUrl()) && page.KeyBook().Equal(st.Network.OperatorBook())
	if isDnOprBook {
		// Update the threshold
		ratio := loadOperatorsThresholdRatio(st, st.NodeUrl(protocol.Globals))
		page.AcceptThreshold = protocol.GetMOfN(len(page.Keys), ratio)
	}

	didUpdateKeyPage(page)
	err = st.Update(page)
	if err != nil {
		return nil, fmt.Errorf("failed to update %v: %v", page.GetUrl(), err)
	}

	// If we are the DN and the page is an operator book, broadcast the update to the BVNs
	if isDnOprBook {
		err = operatorUpdatesToLedger(st, body.Operation)
		if err != nil {
			return nil, err
		}
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
		entry.Delegate = op.Entry.Delegate
		page.AddKeySpec(entry)
		return nil

	case *protocol.RemoveKeyOperation:
		index, _, found := findKeyPageEntry(page, &op.Entry)
		if !found {
			return fmt.Errorf("entry to be removed not found on the key page")
		}

		page.RemoveKeySpecAt(index)

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

		// Find the old entry
		oldPos, entry, found := findKeyPageEntry(page, &op.OldEntry)
		if !found {
			return fmt.Errorf("entry to be updated not found on the key page")
		}

		// Check for an existing key
		_, _, found = findKeyPageEntry(page, &op.NewEntry)
		if found {
			return fmt.Errorf("cannot have duplicate entries on key page")
		}

		// Update the entry
		entry.PublicKeyHash = op.NewEntry.KeyHash
		entry.Delegate = op.NewEntry.Delegate

		// Relocate the entry
		page.RemoveKeySpecAt(oldPos)
		page.AddKeySpec(entry)
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
	} else if search.Delegate != nil {
		i, entry, ok = page.EntryByDelegate(search.Delegate)
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

func getNewOwners(batch *database.Batch, transaction *protocol.Transaction) ([]*url.URL, error) {
	body, ok := transaction.Body.(*protocol.UpdateKeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateKeyPage), transaction.Body)
	}

	var page *protocol.KeyPage
	err := batch.Account(transaction.Header.Principal).GetStateAs(&page)
	if err != nil {
		return nil, errors.Format(errors.StatusUnknown, "load principal: %w", err)
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

			// Don't check if the owner is not changing
			_, oldEntry, ok := findKeyPageEntry(page, &op.OldEntry)
			if ok && oldEntry.Delegate != nil && oldEntry.Delegate.Equal(op.NewEntry.Delegate) {
				continue
			}

			owners = append(owners, op.NewEntry.Delegate)

		default:
			continue
		}
	}

	return owners, nil
}

func loadOperatorsThresholdRatio(st *StateManager, url *url.URL) float64 {
	entry, err := indexing.Data(st.batch, url).GetLatestEntry()
	if err != nil {
		st.logger.Error("Failed to get latest globals data entry", "error", err)
		return protocol.FallbackValidatorThreshold
	}

	globals := new(protocol.NetworkGlobals)
	err = globals.UnmarshalBinary(entry.GetData()[0])
	if err != nil {
		st.logger.Error("Failed to decode latest globals entry", "error", err)
		return protocol.FallbackValidatorThreshold
	}

	return globals.OperatorAcceptThreshold.GetFloat()
}
