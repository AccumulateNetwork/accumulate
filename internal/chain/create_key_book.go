package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type CreateKeyBook struct{}

func (CreateKeyBook) Type() protocol.TransactionType { return protocol.TransactionTypeCreateKeyBook }

func (CreateKeyBook) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	if _, ok := st.Origin.(*protocol.ADI); !ok {
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", protocol.AccountTypeIdentity, st.Origin.Type())
	}

	body, ok := tx.Transaction.Body.(*protocol.CreateKeyBook)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateKeyBook), tx.Transaction.Body)
	}

	if !body.Url.Identity().Equal(st.OriginUrl) {
		return nil, fmt.Errorf("%q does not belong to %q", body.Url, st.OriginUrl)
	}

	scc := new(protocol.SyntheticCreateChain)
	st.Submit(st.OriginUrl, scc)

	book := new(protocol.KeyBook)
	book.Url = body.Url
	book.AddAuthority(body.Url)
	book.PageCount = 1

	if body.Manager != nil {
		err := st.AddAuthority(book, body.Manager)
		if err != nil {
			return nil, err
		}
	}

	err := scc.Create(book)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state for KeyBook %s: %v", book.Url, err)
	}

	page := new(protocol.KeyPage)
	page.Version = 1
	page.Url = protocol.FormatKeyPageUrl(body.Url, 0)

	key := new(protocol.KeySpec)
	key.PublicKeyHash = body.PublicKeyHash
	page.Keys = []*protocol.KeySpec{key}

	err = scc.Create(page)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state for KeyPage %s: %v", page.Url, err)
	}

	return nil, nil
}
