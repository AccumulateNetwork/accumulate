package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateKeyPage struct{}

func (CreateKeyPage) Type() protocol.TransactionType { return protocol.TransactionTypeCreateKeyPage }

func (CreateKeyPage) Execute(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	return (CreateKeyPage{}).Validate(st, tx)
}

func (CreateKeyPage) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	var book *protocol.KeyBook
	switch origin := st.Origin.(type) {
	case *protocol.KeyBook:
		book = origin
	default:
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", protocol.AccountTypeKeyBook, origin.Type())
	}

	body, ok := tx.Transaction.Body.(*protocol.CreateKeyPage)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateKeyPage), tx.Transaction.Body)
	}

	if len(body.Keys) == 0 {
		return nil, fmt.Errorf("cannot create empty sig spec")
	}

	scc := new(protocol.SyntheticCreateChain)
	st.Submit(st.OriginUrl, scc)

	page := new(protocol.KeyPage)
	page.Version = 1
	page.Url = protocol.FormatKeyPageUrl(book.Url, book.PageCount)
	page.AcceptThreshold = 1 // Require one signature from the Key Page
	book.PageCount++

	err := scc.Update(book)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state for KeyBook %s: %v", book.Url, err)
	}

	for _, sig := range body.Keys {
		ss := new(protocol.KeySpec)
		ss.PublicKeyHash = sig.KeyHash
		page.Keys = append(page.Keys, ss)
	}

	err = scc.Create(page)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state for KeyPage` %s: %v", page.Url, err)
	}

	return nil, nil
}
