package chain

import (
	"fmt"

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

	// If Key or NewKey are specified, find the corresponding entry. The user
	// must supply an exact match of the key as is on the key page.
	var bodyKey *protocol.KeySpec
	indexKey := -1
	indexNewKey := -1
	if len(body.Key) > 0 {
		indexKey, bodyKey, _ = page.FindKeyHash(body.Key)
	}
	if len(body.NewKey) > 0 {
		indexNewKey, _, _ = page.FindKeyHash(body.NewKey)
	}

	book := new(protocol.KeyBook)
	err := st.LoadUrlAs(page.KeyBook, book)
	if err != nil {
		return nil, fmt.Errorf("invalid key book: %v", err)
	}

	if st.nodeUrl.JoinPath(protocol.ValidatorBook).Equal(book.Url) {
		return nil, fmt.Errorf("UpdateKeyPage cannot be used to modify the validator key book")
	}

	priority := getPriority(st, book)
	if priority < 0 {
		return nil, fmt.Errorf("cannot find %q in key book with ID %X", st.OriginUrl, page.KeyBook)
	}

	// 0 is the highest priority, followed by 1, etc
	if tx.Transaction.KeyPageIndex > uint64(priority) {
		return nil, fmt.Errorf("cannot modify %q with a lower priority key page", st.OriginUrl)
	}

	switch body.Operation {
	case protocol.KeyPageOperationAdd:
		// Check that a NewKey was provided, and that the key isn't already on
		// the Key Page
		if len(body.NewKey) == 0 { // Provided
			return nil, fmt.Errorf("must provide a new key")
		}
		if indexNewKey > 0 { // Not on the Key Page
			return nil, fmt.Errorf("cannot have duplicate keys on key page")
		}

		key := &protocol.KeySpec{
			PublicKey: body.NewKey,
		}
		if body.Owner != nil {
			key.Owner = body.Owner
		}
		page.Keys = append(page.Keys, key)

	case protocol.KeyPageOperationUpdate:
		// check that the Key to update is on the key Page, and the new Key
		// is not already on the Key Page
		if indexKey < 0 { // The Key to update is on key page
			return nil, fmt.Errorf("key to be updated not found on the key page")
		}
		if indexNewKey >= 0 { // The new key is not on the key page
			return nil, fmt.Errorf("key must be updated to a key not found on key page")
		}

		bodyKey.PublicKey = body.NewKey
		if body.Owner != nil {
			bodyKey.Owner = body.Owner
		}

	case protocol.KeyPageOperationRemove:
		// Make sure the key to be removed is on the Key Page
		if indexKey < 0 {
			return nil, fmt.Errorf("key to be removed not found on the key page")
		}

		page.Keys = append(page.Keys[:indexKey], page.Keys[indexKey+1:]...)

		if len(page.Keys) == 0 && priority == 0 {
			return nil, fmt.Errorf("cannot delete last key of the highest priority page of a key book")
		}

		if page.Threshold > uint64(len(page.Keys)) {
			page.Threshold = uint64(len(page.Keys))
		}

		// SetThreshold sets the signature threshold for the Key Page
	case protocol.KeyPageOperationSetThreshold:
		// Don't care what values are provided by keys....
		if err := page.SetThreshold(body.Threshold); err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("invalid operation: %v", body.Operation)
	}

	didUpdateKeyPage(page)
	st.Update(page)
	return nil, nil
}

func getPriority(st *StateManager, book *protocol.KeyBook) int {
	var priority = -1
	for i := uint64(0); i < book.PageCount; i++ {
		pageUrl := protocol.FormatKeyPageUrl(book.Url, i)
		if pageUrl.AccountID32() == st.OriginChainId {
			priority = int(i)
		}
	}
	return priority
}

func didUpdateKeyPage(page *protocol.KeyPage) {
	// We're changing the height of the key page, so reset all the nonces
	for _, key := range page.Keys {
		key.Nonce = 0
	}
}
