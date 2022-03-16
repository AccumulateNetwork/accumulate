package chain

import (
	"bytes"
	"fmt"

	"github.com/tendermint/tendermint/crypto/ed25519"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type UpdateKey struct{}

func (UpdateKey) Type() protocol.TransactionType {
	return protocol.TransactionTypeUpdateKey
}

func (UpdateKey) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.UpdateKey)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.UpdateKey), tx.Transaction.Body)
	}

	page, ok := st.Origin.(*protocol.KeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", protocol.AccountTypeKeyPage, st.Origin.GetType())
	}

	if page.KeyBook == nil {
		return nil, fmt.Errorf("invalid origin record: page %s does not have a KeyBook", page.Url)
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

	for i, key := range page.Keys { // Look for the old key
		if bytes.Equal(key.PublicKey, tx.Signatures[0].GetPublicKey()) { // User must supply an exact match of the key as is on the key page
			bodyKey, indexKey = key, i
			break
		}
	}

	book := new(protocol.KeyBook)
	err := st.LoadUrlAs(page.KeyBook, book)
	if err != nil {
		return nil, fmt.Errorf("invalid key book: %v", err)
	}

	priority := getPriority(st, book)
	if priority < 0 {
		return nil, fmt.Errorf("cannot find %q in key book with ID %X", st.OriginUrl, page.KeyBook)
	}

	// 0 is the highest priority, followed by 1, etc
	if tx.Transaction.KeyPageIndex > uint64(priority) {
		return nil, fmt.Errorf("cannot modify %q with a lower priority key page", st.OriginUrl)
	}

	// check that the Key to update is on the key Page, and the new Key
	// is not already on the Key Page
	if indexKey < 0 { // The Key to update is on key page
		return nil, fmt.Errorf("key to be updated not found on the key page")
	}
	if indexNewKey >= 0 { // The new key is not on the key page
		return nil, fmt.Errorf("key must be updated to a key not found on key page")
	}

	bodyKey.PublicKey = body.Key

	if len(body.Key) == ed25519.PubKeySize && st.nodeUrl.JoinPath(protocol.ValidatorBook).Equal(book.Url) {
		st.DisableValidator(body.Key)
		st.AddValidator(body.Key)
	}

	st.Update(page)
	return nil, nil
}
