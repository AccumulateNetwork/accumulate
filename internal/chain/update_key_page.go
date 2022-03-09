package chain

import (
	"bytes"
	"fmt"

	"github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type UpdateKeyPage struct{}

func (UpdateKeyPage) Type() types.TxType {
	return types.TxTypeUpdateKeyPage
}

func (UpdateKeyPage) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateKeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateKeyPage), tx.Transaction.Body)
	}

	page, ok := st.Origin.(*protocol.KeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", protocol.AccountTypeKeyPage, st.Origin.GetType())
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
	var bookUrl *url.URL
	var priority = -1
	if page.KeyBook != nil {
		book = new(protocol.KeyBook)
		err := st.LoadUrlAs(page.KeyBook, book)
		if err != nil {
			return nil, fmt.Errorf("invalid key book: %v", err)
		}

		for i, p := range book.Pages {
			if p.AccountID32() == st.OriginChainId {
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

		bookUrl = book.Url
	}

	isValidatorBook := st.nodeUrl.JoinPath(protocol.ValidatorBook).Equal(bookUrl)
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

		if isValidatorBook && len(body.NewKey) == ed25519.PubKeySize {
			st.AddValidator(body.NewKey)
			page.Threshold = protocol.GetValidatorsMOfN(len(page.Keys))
		}

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

		if len(body.NewKey) == ed25519.PubKeySize && isValidatorBook {
			st.DisableValidator(body.Key)
			st.AddValidator(body.NewKey)
		}

	case protocol.KeyPageOperationRemove:
		// Make sure the key to be removed is on the Key Page
		if indexKey < 0 {
			return nil, fmt.Errorf("key to be removed not found on the key page")
		}

		page.Keys = append(page.Keys[:indexKey], page.Keys[indexKey+1:]...)

		keyCount := len(page.Keys)
		if keyCount == 0 && priority == 0 {
			return nil, fmt.Errorf("cannot delete last key of the highest priority page of a key book")
		}

		if isValidatorBook {
			st.DisableValidator(body.Key)
			page.Threshold = protocol.GetValidatorsMOfN(keyCount)
		} else {
			if page.Threshold > uint64(keyCount) {
				page.Threshold = uint64(keyCount)
			}
		}

		// SetThreshold sets the signature threshold for the Key Page
	case protocol.KeyPageOperationSetThreshold:
		if isValidatorBook {
			return nil, fmt.Errorf("the threshold of a validator book is controlled automatically and therefore it can't be set manually")
		}

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
