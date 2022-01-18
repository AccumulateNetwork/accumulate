package chain

import (
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type SendTokens struct{}

func (SendTokens) Type() types.TxType { return types.TxTypeSendTokens }

func (SendTokens) Validate(st *StateManager, tx *transactions.Envelope) error {
	body := new(protocol.SendTokens)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	recipients := make([]*url.URL, len(body.To))
	for i, to := range body.To {
		recipients[i], err = url.Parse(to.Url)
		if err != nil {
			return fmt.Errorf("invalid destination URL: %v", err)
		}
	}

	var account tokenChain
	switch origin := st.Origin.(type) {
	case *protocol.TokenAccount:
		account = origin
	case *protocol.LiteTokenAccount:
		account = origin
	default:
		return fmt.Errorf("invalid origin record: want %v or %v, got %v", types.AccountTypeTokenAccount, types.AccountTypeLiteTokenAccount, st.Origin.Header().Type)
	}

	tokenUrl, err := account.ParseTokenUrl()
	if err != nil {
		return fmt.Errorf("invalid token URL: %v", err)
	}

	//now check to see if we can transact
	//really only need to provide one input...
	//now check to see if the account is good to send tokens from
	total := types.Amount{}
	for _, to := range body.To {
		total.Add(total.AsBigInt(), new(big.Int).SetUint64(to.Amount))
	}

	if !account.CanDebitTokens(&total.Int) {
		return fmt.Errorf("insufficient balance: have %v, want %v", account.TokenBalance(), &total.Int)
	}

	for i, u := range recipients {
		deposit := new(protocol.SyntheticDepositTokens)
		copy(deposit.Cause[:], tx.Transaction.Hash())
		deposit.Token = tokenUrl.String()
		deposit.Amount = *new(big.Int).SetUint64(body.To[i].Amount)
		st.Submit(u, deposit)
	}

	if !account.DebitTokens(&total.Int) {
		return fmt.Errorf("%q balance is insufficient", st.OriginUrl)
	}
	st.Update(account)

	return nil
}
