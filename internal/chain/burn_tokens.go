package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
)

type BurnTokens struct{}

func (BurnTokens) Type() types.TxType { return types.TxTypeBurnTokens }

func (BurnTokens) Validate(st *StateManager, tx *transactions.Envelope) (protocol.TransactionResult, error) {
	body := new(protocol.BurnTokens)
	err := tx.As(body)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %v", err)
	}

	var account tokenChain
	switch origin := st.Origin.(type) {
	case *protocol.LiteTokenAccount:
		account = origin
	case *protocol.TokenAccount:
		account = origin
	default:
		return nil, fmt.Errorf("invalid origin record: want chain type %v or %v, got %v", types.AccountTypeLiteTokenAccount, types.AccountTypeTokenAccount, origin.Header().Type)
	}

	tokenUrl, err := account.ParseTokenUrl()
	if err != nil {
		return nil, fmt.Errorf("invalid token url: %v", err)
	}

	burn := new(protocol.SyntheticBurnTokens)
	copy(burn.Cause[:], tx.Transaction.Hash())
	burn.Amount = body.Amount
	st.Submit(tokenUrl, burn)

	if !account.DebitTokens(&body.Amount) {
		return nil, fmt.Errorf("unable to debit balance from account")
	}
	st.Update(account)
	return nil, nil
}
