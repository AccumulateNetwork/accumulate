package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type CreateKeyBook struct{}

func (CreateKeyBook) Type() types.TxType { return types.TxTypeCreateKeyBook }

func (CreateKeyBook) Validate(st *StateManager, tx *transactions.Envelope) error {
	if _, ok := st.Origin.(*protocol.ADI); !ok {
		return fmt.Errorf("invalid origin record: want account type %v, got %v", types.AccountTypeIdentity, st.Origin.Header().Type)
	}

	body := new(protocol.CreateKeyBook)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	if len(body.Pages) == 0 {
		return fmt.Errorf("cannot create empty sig spec group")
	}

	sgUrl, err := url.Parse(body.Url)
	if err != nil {
		return fmt.Errorf("invalid target URL: %v", err)
	}

	if !sgUrl.Identity().Equal(st.OriginUrl) {
		return fmt.Errorf("%q does not belong to %q", sgUrl, st.OriginUrl)
	}

	entries := make([]*protocol.KeyPage, len(body.Pages))
	for i, page := range body.Pages {
		u, err := url.Parse(page)
		if err != nil {
			return fmt.Errorf("invalid key page url : %s", page)
		}
		entry := new(protocol.KeyPage)
		err = st.LoadUrlAs(u, entry)
		if err != nil {
			return fmt.Errorf("failed to fetch sig spec: %v", err)
		}

		if !u.Identity().Equal(st.OriginUrl) {
			return fmt.Errorf("%q does not belong to %q", u, st.OriginUrl)
		}

		if entry.KeyBook != "" {
			return fmt.Errorf("%q has already been assigned to a key book", u)
		}

		entries[i] = entry
	}

	scc := new(protocol.SyntheticCreateChain)
	scc.Cause = types.Bytes(tx.Transaction.Hash()).AsBytes32()
	st.Submit(st.OriginUrl, scc)

	book := protocol.NewKeyBook()
	book.ChainUrl = types.String(sgUrl.String())

	for _, spec := range entries {
		u, err := spec.ParseUrl()
		if err != nil {
			// We already did this, so this should never fail here.
			return fmt.Errorf("invalid sig spec state: bad URL: %v", err)
		}

		book.Pages = append(book.Pages, u.String())
		spec.KeyBook = types.String(sgUrl.String())
		err = scc.Update(spec)
		if err != nil {
			return fmt.Errorf("failed to marshal state: %v", err)
		}
	}

	err = scc.Create(book)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %v", err)
	}

	return nil
}
