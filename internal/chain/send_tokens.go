package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
)

type SendTokens struct{}

func (SendTokens) Type() protocol.TransactionType { return protocol.TransactionTypeSendTokens }

func (SendTokens) Execute(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	return (SendTokens{}).Validate(st, tx)
}

func (SendTokens) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SendTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SendTokens), tx.Transaction.Body)
	}

	recipients := make([]*url.URL, len(body.To))
	for i, to := range body.To {
		recipients[i] = to.Url
	}

	var account protocol.AccountWithTokens
	switch origin := st.Origin.(type) {
	case *protocol.TokenAccount:
		account = origin
	case *protocol.LiteTokenAccount:
		account = origin
	default:
		return nil, fmt.Errorf("invalid origin record: want %v or %v, got %v", protocol.AccountTypeTokenAccount, protocol.AccountTypeLiteTokenAccount, st.Origin.Type())
	}

	//now check to see if we can transact
	//really only need to provide one input...
	//now check to see if the account is good to send tokens from
	total := types.Amount{}
	for _, to := range body.To {
		total.Add(total.AsBigInt(), &to.Amount)
	}

	if !account.DebitTokens(&total.Int) {
		return nil, fmt.Errorf("insufficient balance: have %v, want %v", account.TokenBalance(), &total.Int)
	}
	st.Update(account)

	for i, u := range recipients {
		deposit := new(protocol.SyntheticDepositTokens)
		deposit.Token = account.GetTokenUrl()
		deposit.Amount = body.To[i].Amount
		st.Submit(u, deposit)
	}

	return nil, nil
}
