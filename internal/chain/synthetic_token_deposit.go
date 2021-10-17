package chain

import (
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
)

type SyntheticTokenDeposit struct{}

func (SyntheticTokenDeposit) Type() types.TxType {
	return types.TxTypeSyntheticTokenDeposit
}

func checkSyntheticTokenDeposit(st *state.StateEntry, tx *transactions.GenTransaction) (*big.Int, tokenChain, *url.URL, error) {
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
	if st.ChainHeader != nil {
		switch st.ChainHeader.Type {
		case types.ChainTypeAnonTokenAccount:
			account = new(protocol.AnonTokenAccount)
		case types.ChainTypeTokenAccount:
			account = new(state.TokenAccount)
		default:
			return nil, nil, nil, fmt.Errorf("cannot deposit tokens to %v", st.ChainHeader.Type)
		}

		err = st.ChainState.As(account)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to decode recipient: %v", err)
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

func (SyntheticTokenDeposit) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	_, _, _, err := checkSyntheticTokenDeposit(st, tx)
	return err
}

func (SyntheticTokenDeposit) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	amount, account, accountUrl, err := checkSyntheticTokenDeposit(st, tx)
	if err != nil {
		return nil, err
	}

	//all is good, so add the balance
	if !account.CreditTokens(amount) {
		return nil, fmt.Errorf("unable to add deposit balance to account")
	}

	//create a transaction reference chain acme-xxxxx/0, 1, 2, ... n.
	//This will reference the txid to keep the history
	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()
	refUrl := accountUrl.JoinPath(fmt.Sprint(account.NextTx()))
	txr := state.NewTxReference(refUrl.String(), txHash[:])
	txrData, err := txr.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to process transaction reference chain %v", err)
	}

	//now create the chain state object.
	txRefChain := new(state.Object)
	txRefChain.Entry = txrData
	txRefChainId := types.Bytes(refUrl.ResourceChain()).AsBytes32()

	acctChainId := types.Bytes(accountUrl.ResourceChain()).AsBytes32()
	data, err := account.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to process anon transaction account %v", err)
	}

	//if we get here with no errors store the states
	st.DB.AddStateEntry(&acctChainId, &txHash, &state.Object{Entry: data})
	st.DB.AddStateEntry(&txRefChainId, &txHash, txRefChain)

	return new(DeliverTxResult), nil
}
