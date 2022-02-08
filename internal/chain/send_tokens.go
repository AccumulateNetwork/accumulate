package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
)

type SendTokens struct{}

func (SendTokens) Type() types.TxType { return types.TxTypeSendTokens }

func (SendTokens) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body := new(protocol.SendTokens)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	recipients := make([]*url.URL, len(body.To))
	for i, to := range body.To {
		recipients[i], err = url.Parse(to.Url)
		if err != nil {
			return nil, fmt.Errorf("invalid destination URL: %v", err)
		}
	}

	var account tokenChain
	switch origin := st.Origin.(type) {
	case *protocol.TokenAccount:
		account = origin
	case *protocol.LiteTokenAccount:
		account = origin
	default:
		return nil, fmt.Errorf("invalid origin record: want %v or %v, got %v", protocol.AccountTypeTokenAccount, protocol.AccountTypeLiteTokenAccount, st.Origin.GetType())
	}

	tokenUrl, err := account.ParseTokenUrl()
	if err != nil {
		return nil, fmt.Errorf("invalid token URL: %v", err)
	}

	//now check to see if we can transact
	//really only need to provide one input...
	//now check to see if the account is good to send tokens from
	total := types.Amount{}
	for _, to := range body.To {
		total.Add(total.AsBigInt(), &to.Amount)
	}

	if !account.CanDebitTokens(&total.Int) {
		return nil, fmt.Errorf("insufficient balance: have %v, want %v", account.TokenBalance(), &total.Int)
	}

	for i, u := range recipients {
		deposit := new(protocol.SyntheticDepositTokens)
		copy(deposit.Cause[:], tx.GetTxHash())
		deposit.Token = tokenUrl.String()
		deposit.Amount = body.To[i].Amount
		st.Submit(u, deposit)
	}

	if !account.DebitTokens(&total.Int) {
		return nil, fmt.Errorf("%q balance is insufficient", st.OriginUrl)
	}
	st.Update(account)

	return nil, nil
}
