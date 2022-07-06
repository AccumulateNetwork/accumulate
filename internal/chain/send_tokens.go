package chain

import (
	"fmt"
	"math/big"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type SendTokens struct{}

func (SendTokens) Type() protocol.TransactionType { return protocol.TransactionTypeSendTokens }

func (SendTokens) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	return (SendTokens{}).Validate(st, tx)
}

func (SendTokens) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.SendTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.SendTokens), tx.Transaction.Body)
	}

	var account protocol.AccountWithTokens
	switch origin := st.Origin.(type) {
	case *protocol.TokenAccount:
		account = origin
	case *protocol.LiteTokenAccount:
		account = origin
	default:
		return nil, fmt.Errorf("invalid principal: want %v or %v, got %v", protocol.AccountTypeTokenAccount, protocol.AccountTypeLiteTokenAccount, st.Origin.Type())
	}

	//now check to see if we can transact
	//really only need to provide one input...
	//now check to see if the account is good to send tokens from
	total := new(big.Int)
	for _, to := range body.To {
		total.Add(total, &to.Amount)
	}

	if !account.DebitTokens(total) {
		return nil, fmt.Errorf("insufficient balance: have %v, want %v", account.TokenBalance(), total)
	}
	err := st.Update(account)
	if err != nil {
		return nil, fmt.Errorf("failed to update account %v: %v", account.GetUrl(), err)
	}

	m := make(map[[32]byte]bool)
	for _, to := range body.To {
		id := to.Url.AccountID32()
		_, ok := m[id]
		if !ok {
			m[id] = true
		} else {
			return nil, fmt.Errorf("duplicate recipient passed in request")
		}
		deposit := new(protocol.SyntheticDepositTokens)
		deposit.Token = account.GetTokenUrl()
		deposit.Amount = to.Amount
		st.Submit(to.Url, deposit)
	}

	return nil, nil
}
