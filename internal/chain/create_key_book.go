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

	if !body.Url.Identity().Equal(st.OriginUrl) {
		return nil, fmt.Errorf("%q does not belong to %q", body.Url, st.OriginUrl)
	}

	scc := new(protocol.SyntheticCreateChain)
	scc.Cause = types.Bytes(tx.GetTxHash()).AsBytes32()
	st.Submit(st.OriginUrl, scc)

	page := new(protocol.KeyPage)
	page.Url = protocol.FormatKeyPageUrl(body.Url, 0)
	page.KeyBook = body.Url

	key := new(protocol.KeySpec)
	key.PublicKey = body.PublicKeyHash
	page.Keys = []*protocol.KeySpec{key}

	err := scc.Create(page)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state for KeyPage %s: %v", page.Url, err)
	}

	book := protocol.NewKeyBook()
	book.Url = body.Url
	book.PageCount = 1
	book.ManagerKeyBook = body.Manager

	err = scc.Create(book)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state for KeyBook %s: %v", book.Url, err)
	}

	return nil, nil
}
