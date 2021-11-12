package chain

import (
	"fmt"
	"math/big"

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

func checkSyntheticTokenDeposit(st *StateManager, tx *transactions.GenTransaction) (*big.Int, tokenChain, *url.URL, error) {
	body := new(synthetic.TokenTransactionDeposit)
	err := tx.As(body)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	if body.ToUrl != types.String(tx.SigInfo.URL) {
		return nil, nil, nil, fmt.Errorf("deposit destination does not match TX sponsor")
	}

	accountUrl, err := url.Parse(tx.SigInfo.URL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid recipient URL: %v", err)
	}

	tokenUrl, err := url.Parse(*body.TokenUrl.AsString())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid token URL: %v", err)
	}

	var account tokenChain
	if st.Sponsor != nil {
		switch sponsor := st.Sponsor.(type) {
		case *protocol.AnonTokenAccount:
			account = sponsor
		case *state.TokenAccount:
			account = sponsor
		default:
			return nil, nil, nil, fmt.Errorf("cannot deposit tokens to %v", sponsor.Header().Type)
		}
	} else if keyHash, tok, err := protocol.ParseAnonymousAddress(accountUrl); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid anonymous token account URL: %v", err)
	} else if keyHash == nil {
		return nil, nil, nil, fmt.Errorf("could not find token account")
	} else if !tokenUrl.Equal(tok) {
		return nil, nil, nil, fmt.Errorf("token URL does not match anonymous token account URL")
	} else {
		// Address is anonymous and the account doesn't exist, so create one
		anon := protocol.NewAnonTokenAccount()
		anon.ChainUrl = types.String(accountUrl.String())
		anon.TokenUrl = tokenUrl.String()
		account = anon
	}

	return &body.DepositAmount.Int, account, accountUrl, nil
}

func (SyntheticTokenDeposit) CheckTx(st *StateManager, tx *transactions.GenTransaction) error {
	_, _, _, err := checkSyntheticTokenDeposit(st, tx)
	return err
}

func (SyntheticTokenDeposit) DeliverTx(st *StateManager, tx *transactions.GenTransaction) error {
	amount, account, accountUrl, err := checkSyntheticTokenDeposit(st, tx)
	if err != nil {
		return err
	}

	if !account.CreditTokens(amount) {
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
