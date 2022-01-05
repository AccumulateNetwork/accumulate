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

	// Find the old key
	var oldKey *protocol.KeySpec
	index := -1
	if len(body.Key) > 0 && body.Operation < protocol.AddKey {
		for i, key := range page.Keys {
			// We could support (double) SHA-256 but I think it's fine to
			// require the user to provide an exact match.
			if bytes.Equal(key.PublicKey, body.Key) {
				oldKey, index = key, i
				break
			}
		}
		if index < 0 {
			return fmt.Errorf("key not found on the key page")
		}
	}

	var book *protocol.KeyBook
	var priority = -1
	if page.KeyBook != "" {
		book = new(protocol.KeyBook)
		u, err := url.Parse(*page.KeyBook.AsString())
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

	switch body.Operation {
	case protocol.AddKey:
		if len(body.Key) > 0 {
			return fmt.Errorf("trying to add a new key but you gave me an existing key")
		}

		page.Keys = append(page.Keys, &protocol.KeySpec{
			PublicKey: body.NewKey,
		})

	case protocol.UpdateKey:
		if len(body.Key) == 0 {
			return fmt.Errorf("trying to update a new key but you didn't give me an existing key")
		}
		if oldKey == nil {
			return fmt.Errorf("no matching key found")
		}

		oldKey.PublicKey = body.NewKey

	case protocol.RemoveKey:
		if len(page.Keys) == 1 {
			return fmt.Errorf("cannot remove the last key") // TODO: allow removal of the last key for all but the first page
		}
		if len(body.Key) == 0 {
			return fmt.Errorf("trying to update a new key but you didn't give me an existing key")
		}
		if oldKey == nil {
			return fmt.Errorf("no matching key found")
		}
		page.Keys = append(page.Keys[:index], page.Keys[index+1:]...)

		if len(page.Keys) == 0 && priority == 0 {
			return fmt.Errorf("cannot delete last key of the highest priority page of a key book")
		}

		if page.Threshold > uint64(len(page.Keys)) {
			page.Threshold = uint64(len(page.Keys))
		}

		// SetThreshold sets the signature threshold for the Key Page
	case protocol.SetM:
		if err := page.SetThreshold(body.Threshold); err != nil {
			return err
		}

	default:
		return fmt.Errorf("invalid operation: %v", body.Operation)
	}
	// We're changing the height of the key page, so reset all the nonces
	for _, key := range page.Keys {
		key.Nonce = 0
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
