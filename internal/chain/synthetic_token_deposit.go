package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/AccumulateNetwork/accumulate/types/synthetic"
)

type SyntheticTokenDeposit struct{}

func (SyntheticTokenDeposit) Type() types.TxType {
	return types.TxTypeSyntheticDepositTokens
}

func (SyntheticTokenDeposit) Validate(st *StateManager, tx *transactions.GenTransaction) error {
	// *big.Int, tokenChain, *url.URL
	body := new(synthetic.TokenTransactionDeposit)
	err := tx.As(body)
	if err != nil {
		return fmt.Errorf("invalid payload: %v", err)
	}

	if body.ToUrl != types.String(tx.SigInfo.URL) {
		return fmt.Errorf("deposit destination does not match TX sponsor")
	}

	accountUrl, err := url.Parse(tx.SigInfo.URL)
	if err != nil {
		return fmt.Errorf("invalid recipient URL: %v", err)
	}

	tokenUrl, err := url.Parse(*body.TokenUrl.AsString())
	if err != nil {
		return fmt.Errorf("invalid token URL: %v", err)
	}

	var account tokenChain
	if st.Sponsor != nil {
		switch sponsor := st.Sponsor.(type) {
		case *protocol.LiteTokenAccount:
			account = sponsor
		case *state.TokenAccount:
			account = sponsor
		default:
			return fmt.Errorf("invalid sponsor: want chain type %v or %v, got %v", types.ChainTypeLiteTokenAccount, types.ChainTypeTokenAccount, sponsor.Header().Type)
		}
	} else if keyHash, tok, err := protocol.ParseAnonymousAddress(accountUrl); err != nil {
		return fmt.Errorf("invalid anonymous token account URL: %v", err)
	} else if keyHash == nil {
		return fmt.Errorf("could not find token account")
	} else if !tokenUrl.Equal(tok) {
		return fmt.Errorf("token URL does not match anonymous token account URL")
	} else {
		// Address is anonymous and the account doesn't exist, so create one
		anon := protocol.NewLiteTokenAccount()
		anon.ChainUrl = types.String(accountUrl.String())
		anon.TokenUrl = tokenUrl.String()
		account = anon
	}

	if !account.CreditTokens(&body.DepositAmount.Int) {
		return fmt.Errorf("unable to add deposit balance to account")
	}
	st.Update(account)

	//create a transaction reference chain acme-xxxxx/0, 1, 2, ... n.
	//This will reference the txid to keep the history
	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()
	refUrl := accountUrl.JoinPath(fmt.Sprint(account.NextTx()))
	txr := state.NewTxReference(refUrl.String(), txHash[:])
	st.Update(txr)

	return nil
}
