package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type IssueTokens struct{}

func (IssueTokens) Type() types.TxType { return types.TxTypeIssueTokens }

func (IssueTokens) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.IssueTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.IssueTokens), tx.Transaction.Body)
	}

	issuer, ok := st.Origin.(*protocol.TokenIssuer)
	if !ok {
		return nil, fmt.Errorf("invalid origin record: want chain type %v, got %v", protocol.AccountTypeTokenIssuer, st.Origin.GetType())
	}

	if issuer.Supply.Cmp(&body.Amount) < 0 && issuer.HasSupplyLimit {
		return nil, fmt.Errorf("can't issue more than the limited supply")
	}
	issuer.Supply.Sub(&issuer.Supply, &body.Amount)

	deposit := new(protocol.SyntheticDepositTokens)
	deposit.Source = st.nodeUrl
	copy(deposit.Cause[:], tx.GetTxHash())
	deposit.Token = issuer.Header().Url
	deposit.Amount = body.Amount
	st.Submit(body.Recipient, deposit)

	st.Update(issuer)

	return nil, nil
}
