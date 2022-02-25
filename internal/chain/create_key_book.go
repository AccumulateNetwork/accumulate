package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type CreateKeyBook struct{}

func (CreateKeyBook) Type() types.TxType { return types.TxTypeCreateKeyBook }

func (CreateKeyBook) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	if _, ok := st.Origin.(*protocol.ADI); !ok {
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", protocol.AccountTypeIdentity, st.Origin.GetType())
	}

	body, ok := tx.Transaction.Body.(*protocol.CreateKeyBook)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.CreateKeyBook), tx.Transaction.Body)
	}

	if len(body.Pages) == 0 {
		return nil, fmt.Errorf("cannot create empty sig spec group")
	}

	if !body.Url.RootIdentity().Equal(st.OriginUrl) {
		return nil, fmt.Errorf("%q does not belong to %q", body.Url, st.OriginUrl)
	}

	entries := make([]*protocol.KeyPage, len(body.Pages))
	for i, page := range body.Pages {
		entry := new(protocol.KeyPage)
		err := st.LoadUrlAs(page, entry)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch sig spec: %v", err)
		}

		if !page.RootIdentity().Equal(st.OriginUrl) {
			return nil, fmt.Errorf("%q does not belong to %q", page, st.OriginUrl)
		}

		if entry.KeyBook != nil {
			return nil, fmt.Errorf("%q has already been assigned to a key book", page)
		}

		entries[i] = entry
	}

	scc := new(protocol.SyntheticCreateChain)
	scc.Cause = types.Bytes(tx.GetTxHash()).AsBytes32()
	st.Submit(st.OriginUrl, scc)

	book := protocol.NewKeyBook()
	book.Url = body.Url
	book.ManagerKeyBook = body.Manager

	for _, spec := range entries {
		u, err := spec.ParseUrl()
		if err != nil {
			// We already did this, so this should never fail here.
			return nil, fmt.Errorf("invalid sig spec state: bad URL: %v", err)
		}

		book.Pages = append(book.Pages, u)
		spec.KeyBook = body.Url
		err = scc.Update(spec)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal state: %v", err)
		}
	}

	err := scc.Create(book)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %v", err)
	}

	return nil, nil
}
