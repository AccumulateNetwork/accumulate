package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type IssueTokens struct{}

func (IssueTokens) Type() types.TxType { return types.TxTypeIssueTokens }

func (IssueTokens) Validate(st *StateManager, tx *transactions.Envelope) error {
	body := new(protocol.IssueTokens)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	accountUrl, err := url.Parse(body.Recipient)
	if err != nil {
		return fmt.Errorf("invalid recipient account URL: %v", err)
	}

	issuer, ok := st.Origin.(*protocol.TokenIssuer)
	if !ok {
		return fmt.Errorf("invalid origin record: want chain type %v, got %v", types.ChainTypeTokenIssuer, st.Origin.Header().Type)
	}

	if issuer.Supply.Cmp(&body.Amount) < 0 && issuer.HasSupplyLimit {
		return fmt.Errorf("can't issue more than the limited supply")
	}
	issuer.Supply.Sub(&issuer.Supply, &body.Amount)

	deposit := new(protocol.SyntheticDepositTokens)
	copy(deposit.Cause[:], tx.Transaction.Hash())
	deposit.Token = issuer.Header().GetChainUrl()
	deposit.Amount = body.Amount
	st.Submit(accountUrl, deposit)

	st.Update(issuer)

	return nil
}
