package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

type CreateKeyBook struct{}

func (CreateKeyBook) Type() types.TxType { return types.TxTypeCreateKeyBook }

func (CreateKeyBook) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	if _, ok := st.Origin.(*state.AdiState); !ok {
		return fmt.Errorf("invalid origin record: want chain type %v, got %v", types.ChainTypeIdentity, st.Origin.Header().Type)
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
	for i, chainId := range body.Pages {
		entry := new(protocol.KeyPage)
		err = st.LoadAs(chainId, entry)
		if err != nil {
			return fmt.Errorf("failed to fetch sig spec: %v", err)
		}

		u, err := entry.ParseUrl()
		if err != nil {
			// This should not happen. Only valid URLs should be stored.
			return fmt.Errorf("invalid sig spec state: bad URL: %v", err)
		}

		if !u.Identity().Equal(st.OriginUrl) {
			return fmt.Errorf("%q does not belong to %q", u, st.OriginUrl)
		}

		if (entry.KeyBook != types.Bytes32{}) {
			return fmt.Errorf("%q has already been assigned to a key book", u)
		}

		entries[i] = entry
	}

	scc := new(protocol.SyntheticCreateChain)
	scc.Cause = types.Bytes(tx.TransactionHash()).AsBytes32()
	st.Submit(st.OriginUrl, scc)

	book := protocol.NewKeyBook()
	book.ChainUrl = types.String(sgUrl.String())

	groupChainId := types.Bytes(sgUrl.ResourceChain()).AsBytes32()
	for _, spec := range entries {
		u, err := spec.ParseUrl()
		if err != nil {
			// We already did this, so this should never fail here.
			return fmt.Errorf("invalid sig spec state: bad URL: %v", err)
		}

		specChainId := types.Bytes(u.ResourceChain()).AsBytes32()
		book.Pages = append(book.Pages, specChainId)
		spec.KeyBook = groupChainId
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
