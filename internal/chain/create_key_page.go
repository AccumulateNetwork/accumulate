package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

type CreateKeyPage struct{}

func (CreateKeyPage) Type() protocol.TransactionType { return protocol.TransactionTypeCreateKeyPage }

func (CreateKeyPage) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	var book *protocol.KeyBook
	switch origin := st.Origin.(type) {
	case *protocol.KeyBook:
		book = origin
	default:
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", protocol.AccountTypeKeyBook, origin.GetType())
	}

	body, ok := tx.Transaction.Body.(*protocol.CreateKeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateKeyPage), tx.Transaction.Body)
	}

	if len(body.Keys) == 0 {
		return nil, fmt.Errorf("cannot create empty sig spec")
	}

	scc := new(protocol.SyntheticCreateChain)
	scc.Cause = types.Bytes(tx.GetTxHash()).AsBytes32()
	st.Submit(st.OriginUrl, scc)

	page := protocol.NewKeyPage()
	page.Url = protocol.FormatKeyPageUrl(book.Url, book.PageCount)
	page.KeyBook = book.Url
	page.Threshold = 1 // Require one signature from the Key Page
	page.ManagerKeyBook = body.Manager
	book.PageCount++

	err := scc.Update(book)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state for KeyBook %s: %v", book.Url, err)
	}

	for _, sig := range body.Keys {
		ss := new(protocol.KeySpec)
		if len(sig.KeyHash) > 0 && len(sig.KeyHash) != 32 {
			return nil, fmt.Errorf("Invalid Key Hash: length must be equal to 32 bytes")
		}
		ss.PublicKeyHash = sig.KeyHash

		page.Keys = append(page.Keys, ss)
	}

	err = scc.Create(page)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state for KeyPage` %s: %v", page.Url, err)
	}

	return nil, nil
}
