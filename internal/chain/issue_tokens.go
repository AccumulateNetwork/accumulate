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

	if issuer.Issued.Cmp(&body.Amount) < 0 && issuer.SupplyLimit == nil {
		return nil, fmt.Errorf("can't issue more than the limited supply")
	}
	issuer.Issued.Sub(&issuer.Issued, &body.Amount)

	deposit := new(protocol.SyntheticDepositTokens)
	copy(deposit.Cause[:], tx.GetTxHash())
	deposit.Token = issuer.Header().Url
	deposit.Amount = body.Amount
	st.Submit(body.Recipient, deposit)

	st.Update(issuer)

	return nil, nil
}
