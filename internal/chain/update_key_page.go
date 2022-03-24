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

func (UpdateKeyPage) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateKeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateKeyPage), tx.Transaction.Body)
	}

	page, ok := st.Origin.(*protocol.KeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", protocol.AccountTypeKeyPage, st.Origin.GetType())
	}

	if page.KeyBook == nil {
		return nil, fmt.Errorf("invalid origin record: page %s does not have a KeyBook", page.Url)
	}

	var book *protocol.KeyBook
	err := st.LoadUrlAs(page.KeyBook, &book)
	if err != nil {
		return nil, fmt.Errorf("invalid key book: %v", err)
	}

	if st.nodeUrl.JoinPath(protocol.ValidatorBook).Equal(book.Url) {
		return nil, fmt.Errorf("UpdateKeyPage cannot be used to modify the validator key book")
	}

	originPriority, ok := getKeyPageIndex(st.OriginUrl)
	if !ok {
		return nil, fmt.Errorf("cannot parse key page URL: %v", st.OriginUrl)
	}

	signerPriority, ok := getKeyPageIndex(st.SignatorUrl)
	if !ok {
		return nil, fmt.Errorf("cannot parse key page URL: %v", st.SignatorUrl)
	}

	// 0 is the highest priority, followed by 1, etc
	if signerPriority > originPriority {
		return nil, fmt.Errorf("cannot modify %q with a lower priority key page", st.OriginUrl)
	}

	switch op := body.Operation.(type) {
	case *protocol.AddKeyOperation:
		if op.Entry.IsEmpty() {
			return nil, fmt.Errorf("cannot add an empty entry")
		}

		_, _, found := findKeyPageEntry(page, &op.Entry)
		if found {
			return nil, fmt.Errorf("cannot have duplicate entries on key page")
		}

		entry := new(protocol.KeySpec)
		entry.PublicKey = op.Entry.PublicKey
		entry.Owner = op.Entry.Owner
		page.Keys = append(page.Keys, entry)

	case *protocol.RemoveKeyOperation:
		index, _, found := findKeyPageEntry(page, &op.Entry)
		if !found {
			return nil, fmt.Errorf("entry to be removed not found on the key page")
		}

		page.Keys = append(page.Keys[:index], page.Keys[index+1:]...)

		if len(page.Keys) == 0 && originPriority == 0 {
			return nil, fmt.Errorf("cannot delete last key of the highest priority page of a key book")
		}

		if page.Threshold > uint64(len(page.Keys)) {
			page.Threshold = uint64(len(page.Keys))
		}

	case *protocol.UpdateKeyOperation:
		if op.NewEntry.IsEmpty() {
			return nil, fmt.Errorf("cannot add an empty entry")
		}

		_, entry, found := findKeyPageEntry(page, &op.OldEntry)
		if !found {
			return nil, fmt.Errorf("entry to be updated not found on the key page")
		}

		_, _, found = findKeyPageEntry(page, &op.NewEntry)
		if found {
			return nil, fmt.Errorf("cannot have duplicate entries on key page")
		}

		entry.PublicKey = op.NewEntry.PublicKey
		entry.Owner = op.NewEntry.Owner

	case *protocol.SetThresholdKeyPageOperation:
		err = page.SetThreshold(op.Threshold)
		if err != nil {
			return nil, err
		}

	case *protocol.UpdateAllowedKeyPageOperation:
		if signerPriority == originPriority {
			return nil, fmt.Errorf("%v cannot modify its own allowed operations", st.OriginUrl)
		}

		if page.TransactionBlacklist == nil {
			page.TransactionBlacklist = new(protocol.AllowedTransactions)
		}

		for _, txn := range op.Allow {
			bit, ok := txn.AllowedTransactionBit()
			if !ok {
				return nil, fmt.Errorf("transaction type %v cannot be (dis)allowed", txn)
			}
			page.TransactionBlacklist.Clear(bit)
		}

		for _, txn := range op.Deny {
			bit, ok := txn.AllowedTransactionBit()
			if !ok {
				return nil, fmt.Errorf("transaction type %v cannot be (dis)allowed", txn)
			}
			page.TransactionBlacklist.Set(bit)
		}

		if *page.TransactionBlacklist == 0 {
			page.TransactionBlacklist = nil
		}

	default:
		return nil, fmt.Errorf("invalid operation: %v", body.Operation.Type())
	}

	didUpdateKeyPage(page)
	st.Update(page)
	return nil, nil
}

func getKeyPageIndex(page *url.URL) (uint64, bool) {
	_, index, ok := protocol.ParseKeyPageUrl(page)
	return index - 1, ok
}

func didUpdateKeyPage(page *protocol.KeyPage) {
	// We're changing the height of the key page, so reset all the nonces
	for _, key := range page.Keys {
		key.LastUsedOn = 0
	}
}

func findKeyPageEntry(page *protocol.KeyPage, search *protocol.KeySpecParams) (int, *protocol.KeySpec, bool) {
	if len(search.PublicKey) > 0 {
		return page.EntryByKeyHash(search.PublicKey)
	}

	if search.Owner != nil {
		return page.EntryByOwner(search.Owner)
	}

	return -1, nil, false
}
