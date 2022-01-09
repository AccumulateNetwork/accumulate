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

func (UpdateKeyPage) Validate(st *StateManager, tx *transactions.Envelope) error {
	body := new(protocol.UpdateKeyPage)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	page, ok := st.Origin.(*protocol.KeyPage)
	if !ok {
		return fmt.Errorf("invalid origin record: want chain type %v, got %v", types.ChainTypeKeyPage, st.Origin.Header().Type)
	}

	// We're changing the height of the key page, so reset all the nonces
	for _, key := range page.Keys {
		key.Nonce = 0
	}

	// Find the old key.  Also go ahead and check cases where we must have the
	// old key, can't have the old key, and don't care about the old key.
	var oldKey *protocol.KeySpec
	index := -1
	if len(body.Key) > 0 && body.Operation != protocol.SetThreshold { // SetThreshold doesn't care about the old key
		for i, key := range page.Keys { // Look for the old key
			if bytes.Equal(key.PublicKey, body.Key) { // User must supply an exact match of the key as is on the key page
				oldKey, index = key, i
				break
			}
		}
		if index >= 0 && body.Operation == protocol.AddKey { // AddKey cannot have an old key
			return fmt.Errorf("an existing key cannot be added again to the Key Page")
		}
		if index < 0 { // Everything else must have an old key
			return fmt.Errorf("specified key not found on the key page")
		}
	}

	var book *protocol.KeyBook
	var priority = -1
	if page.KeyBook != "" {
		book = new(protocol.KeyBook)
		u, err := url.Parse(*page.KeyBook.AsString())
		if err != nil {
			return fmt.Errorf("invalid key book url : %s", *page.KeyBook.AsString())
		}
		err = st.LoadUrlAs(u, book)
		if err != nil {
			return fmt.Errorf("invalid key book: %v", err)
		}

		for i, p := range book.Pages {
			u, err := url.Parse(p)
			if err != nil {
				return fmt.Errorf("invalid key page url : %s", p)
			}
			if u.ResourceChain32() == st.OriginChainId {
				priority = i
			}
		}
		if priority < 0 {
			return fmt.Errorf("cannot find %q in key book with ID %X", st.OriginUrl, page.KeyBook)
		}

		// 0 is the highest priority, followed by 1, etc
		if tx.Transaction.KeyPageIndex > uint64(priority) {
			return fmt.Errorf("cannot modify %q with a lower priority key page", st.OriginUrl)
		}
	}

	if body.Owner != "" {
		_, err := url.Parse(body.Owner)
		if err != nil {
			return fmt.Errorf("invalid key book url : %s", body.Owner)
		}
	}

	switch body.Operation {
	case protocol.AddKey:
		key := &protocol.KeySpec{
			PublicKey: body.NewKey,
		}
		if body.Owner != "" {
			key.Owner = body.Owner
		}
		page.Keys = append(page.Keys, key)

	case protocol.UpdateKey:
		oldKey.PublicKey = body.NewKey
		if body.Owner != "" {
			oldKey.Owner = body.Owner
		}

	case protocol.RemoveKey:
		page.Keys = append(page.Keys[:index], page.Keys[index+1:]...)

		if len(page.Keys) == 0 && priority == 0 {
			return fmt.Errorf("cannot delete last key of the highest priority page of a key book")
		}

		if page.Threshold > uint64(len(page.Keys)) {
			page.Threshold = uint64(len(page.Keys))
		}

		// SetThreshold sets the signature threshold for the Key Page
	case protocol.SetThreshold:
		if err := page.SetThreshold(body.Threshold); err != nil {
			return err
		}

	default:
		return fmt.Errorf("invalid operation: %v", body.Operation)
	}

	st.Update(page)
	return nil
}

func (UpdateKeyPage) CheckTx(st *StateManager, tx *transactions.Envelope) error {
	return UpdateKeyPage{}.Validate(st, tx)
}

func (UpdateKeyPage) DeliverTx(st *StateManager, tx *transactions.Envelope) error {
	return UpdateKeyPage{}.Validate(st, tx)
}
