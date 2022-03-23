package chain

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type BurnTokens struct{}

func (BurnTokens) Type() protocol.TransactionType { return protocol.TransactionTypeBurnTokens }

func (BurnTokens) Validate(st *StateManager, tx *protocol.Envelope) (protocol.TransactionResult, error) {
	body, ok := tx.Transaction.Body.(*protocol.BurnTokens)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.BurnTokens), tx.Transaction.Body)
	}

	var account tokenChain
	switch origin := st.Origin.(type) {
	case *protocol.LiteTokenAccount:
		account = origin
	case *protocol.TokenAccount:
		account = origin
	default:
		return nil, fmt.Errorf("invalid origin record: want chain type %v or %v, got %v", protocol.AccountTypeLiteTokenAccount, protocol.AccountTypeTokenAccount, origin.GetType())
	}

	//ensure user cannot burn more than is in the account
	if !account.CanDebitTokens(&body.Amount) {
		return nil, fmt.Errorf("cannot burn more tokens than is available in account")
	}

	if !account.DebitTokens(&body.Amount) {
		return nil, fmt.Errorf("unable to debit balance from account")
	}

	burn := new(protocol.SyntheticBurnTokens)
	copy(burn.Cause[:], tx.GetTxHash())
	burn.Amount = body.Amount
	st.Submit(account.GetTokenUrl(), burn)

	st.Update(account)
	return nil, nil
}
