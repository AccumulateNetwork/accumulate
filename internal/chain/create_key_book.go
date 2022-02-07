package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type CreateKeyBook struct{}

func (CreateKeyBook) Type() types.TxType { return types.TxTypeCreateKeyBook }

func (CreateKeyBook) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	if _, ok := st.Origin.(*protocol.ADI); !ok {
		return nil, fmt.Errorf("invalid origin record: want account type %v, got %v", types.AccountTypeIdentity, st.Origin.Header().Type)
	}

	body := new(protocol.CreateKeyBook)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	if len(body.Pages) == 0 {
		return nil, fmt.Errorf("cannot create empty sig spec group")
	}

	sgUrl, err := url.Parse(body.Url)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL: %v", err)
	}

	if !sgUrl.Identity().Equal(st.OriginUrl) {
		return nil, fmt.Errorf("%q does not belong to %q", sgUrl, st.OriginUrl)
	}

	entries := make([]*protocol.KeyPage, len(body.Pages))
	for i, page := range body.Pages {
		u, err := url.Parse(page)
		if err != nil {
			return nil, fmt.Errorf("invalid key page url : %s", page)
		}
		entry := new(protocol.KeyPage)
		err = st.LoadUrlAs(u, entry)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch sig spec: %v", err)
		}

		if !u.Identity().Equal(st.OriginUrl) {
			return nil, fmt.Errorf("%q does not belong to %q", u, st.OriginUrl)
		}

		if entry.KeyBook != "" {
			return nil, fmt.Errorf("%q has already been assigned to a key book", u)
		}

		entries[i] = entry
	}

	scc := new(protocol.SyntheticCreateChain)
	scc.Cause = types.Bytes(tx.GetTxHash()).AsBytes32()
	st.Submit(st.OriginUrl, scc)

	book := protocol.NewKeyBook()
	book.Url = sgUrl.String()

	for _, spec := range entries {
		u, err := spec.ParseUrl()
		if err != nil {
			// We already did this, so this should never fail here.
			return nil, fmt.Errorf("invalid sig spec state: bad URL: %v", err)
		}

		book.Pages = append(book.Pages, u.String())
		spec.KeyBook = sgUrl.String()
		err = scc.Update(spec)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal state: %v", err)
		}
	}

	err = scc.Create(book)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %v", err)
	}

	return nil, nil
}
