package chain

import (
	"bytes"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type UpdateKeyPage struct{}

func (UpdateKeyPage) Type() types.TxType {
	return types.TxTypeUpdateKeyPage
}

func (UpdateKeyPage) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body := new(protocol.UpdateKeyPage)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	page, ok := st.Origin.(*protocol.KeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", types.AccountTypeKeyPage, st.Origin.Header().Type)
	}

	// We're changing the height of the key page, so reset all the nonces
	for _, key := range page.Keys {
		key.Nonce = 0
	}

	// Find the old key.  Also go ahead and check cases where we must have the
	// old key, can't have the old key, and don't care about the old key.
	var bodyKey *protocol.KeySpec
	indexKey := -1
	indexNewKey := -1
	if len(body.Key) > 0 { // SetThreshold doesn't care about the old key
		for i, key := range page.Keys { // Look for the old key
			if bytes.Equal(key.PublicKey, body.Key) { // User must supply an exact match of the key as is on the key page
				bodyKey, indexKey = key, i
				break
			}
		}
	}
	if len(body.NewKey) > 0 {
		for i, key := range page.Keys { // Look for the old key
			if bytes.Equal(key.PublicKey, body.NewKey) { // User must supply an exact match of the key as is on the key page
				indexNewKey = i
				break
			}
		}
	}

	var book *protocol.KeyBook
	var priority = -1
	if page.KeyBook != "" {
		book = new(protocol.KeyBook)
		u, err := url.Parse(*page.KeyBook.AsString())
		if err != nil {
			return nil, fmt.Errorf("invalid key book url : %s", *page.KeyBook.AsString())
		}
		err = st.LoadUrlAs(u, book)
		if err != nil {
			return nil, fmt.Errorf("invalid key book: %v", err)
		}

		for i, p := range book.Pages {
			u, err := url.Parse(p)
			if err != nil {
				return nil, fmt.Errorf("invalid key page url : %s", p)
			}
			if u.AccountID32() == st.OriginChainId {
				priority = i
			}
		}
		if priority < 0 {
			return nil, fmt.Errorf("cannot find %q in key book with ID %X", st.OriginUrl, page.KeyBook)
		}

		// 0 is the highest priority, followed by 1, etc
		if tx.Transaction.KeyPageIndex > uint64(priority) {
			return nil, fmt.Errorf("cannot modify %q with a lower priority key page", st.OriginUrl)
		}
	}

	if body.Owner != "" {
		_, err := url.Parse(body.Owner)
		if err != nil {
			return nil, fmt.Errorf("invalid key book url : %s", body.Owner)
		}
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
		if body.Owner != "" {
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
		if body.Owner != "" {
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

	st.Update(page)
	return nil, nil
}
