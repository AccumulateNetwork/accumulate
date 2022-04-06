package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type IssueTokens struct{}

func (IssueTokens) Type() protocol.TransactionType { return protocol.TransactionTypeIssueTokens }

func (IssueTokens) Execute(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	return (IssueTokens{}).Validate(st, tx)
}

func (IssueTokens) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.IssueTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.IssueTokens), tx.Transaction.Body)
	}

	issuer, ok := st.Origin.(*protocol.TokenIssuer)
	if !ok {
		return nil, fmt.Errorf("invalid origin record: want chain type %v, got %v", protocol.AccountTypeTokenIssuer, st.Origin.Type())
	}

	issuer.Issued.Add(&issuer.Issued, &body.Amount)

	if issuer.SupplyLimit != nil && issuer.Issued.Cmp(issuer.SupplyLimit) > 0 {
		return nil, fmt.Errorf("cannot exceed supply limit")
	}

	deposit := new(protocol.SyntheticDepositTokens)
	deposit.Token = issuer.GetUrl()
	deposit.Amount = body.Amount
	st.Submit(body.Recipient, deposit)

	st.Update(issuer)

	return nil, nil
}
