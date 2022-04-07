package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateKeyPage struct{}

func (UpdateKeyPage) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateKeyPage
}

func (UpdateKeyPage) Execute(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	return (UpdateKeyPage{}).Validate(st, tx)
}

func (UpdateKeyPage) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
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

	pagePri, ok := getKeyPageIndex(page.Url)
	if !ok {
		return nil, fmt.Errorf("cannot parse key page URL: %v", page.Url)
	}

	signerPri, ok := getKeyPageIndex(st.SignatorUrl)
	if !ok {
		return nil, fmt.Errorf("cannot parse key page URL: %v", st.SignatorUrl)
	}

	// 0 is the highest priority, followed by 1, etc
	if signerPri > pagePri {
		return nil, fmt.Errorf("cannot modify %q with a lower priority key page", page.Url)
	}

	for _, op := range body.Operation {
		err = UpdateKeyPage{}.executeOperation(page, pagePri, signerPri, op)
		if err != nil {
			return nil, err
		}
	}

	didUpdateKeyPage(page)
	st.Update(page)
	return nil, nil
}

func (UpdateKeyPage) executeOperation(page *protocol.KeyPage, pagePri, signerPri uint64, op protocol.KeyPageOperation) error {
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

		if len(page.Keys) == 0 && pagePri == 0 {
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
		if signerPri == pagePri {
			return fmt.Errorf("%v cannot modify its own allowed operations", page.Url)
		}

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

func getKeyPageIndex(page *url.URL) (uint64, bool) {
	_, index, ok := protocol.ParseKeyPageUrl(page)
	return index - 1, ok
}

func didUpdateKeyPage(page *protocol.KeyPage) {
	page.Version += 1

	// We're changing the height of the key page, so reset all the nonces
	for _, key := range page.Keys {
		key.LastUsedOn = 0
	}
}

func findKeyPageEntry(page *protocol.KeyPage, search *protocol.KeySpecParams) (int, *protocol.KeySpec, bool) {
	if len(search.KeyHash) > 0 {
		i, entry, ok := page.EntryByKeyHash(search.KeyHash)
		var keySpec *protocol.KeySpec
		if ok {
			// If this is not true, something is seriously wrong
			keySpec = entry.(*protocol.KeySpec)
		}
		return i, keySpec, ok
	}

	if search.Owner != nil {
		return page.EntryByOwner(search.Owner)
	}

	return -1, nil, false
}
