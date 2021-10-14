package chain

import (
	"fmt"

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

func (SyntheticTokenDeposit) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	return nil
}

func (SyntheticTokenDeposit) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	deposit := new(synthetic.TokenTransactionDeposit)
	err := tx.As(deposit)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal deposit: %v", err)
	}

	tokenUrl, err := url.Parse(*deposit.TokenUrl.AsString())
	if err != nil {
		return nil, fmt.Errorf("invalid token URL: %v", err)
	} else if !protocol.AcmeUrl().Equal(tokenUrl) {
		// TODO support non-acme tokens
		return nil, fmt.Errorf("depositing non-ACME tokens has not been implemented")
	}

	if deposit.ToUrl != types.String(tx.SigInfo.URL) {
		return nil, fmt.Errorf("deposit destination does not match TX sponsor")
	}

	toUrl, err := url.Parse(*deposit.ToUrl.AsString())
	if err != nil {
		return nil, fmt.Errorf("invalid sponsor URL: %v", err)
	}

	acctObj := st.ChainState
	account := new(state.TokenAccount)
	if st.ChainHeader != nil {
		switch st.ChainHeader.Type {
		case types.ChainTypeTokenAccount, types.ChainTypeAnonTokenAccount:
			// ok
		default:
			return nil, fmt.Errorf("sponsor is not an account")
		}

		err = st.ChainState.As(account)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal token account: %v", err)
		}
	} else if keyHash, tok, err := protocol.ParseAnonymousAddress(toUrl); err != nil {
		return nil, fmt.Errorf("invalid anonymous token account URL: %v", err)
	} else if keyHash == nil {
		return nil, fmt.Errorf("could not find token account")
	} else if !tokenUrl.Equal(tok) {
		return nil, fmt.Errorf("token URL does not match anonymous token account URL")
	} else {
		// Address is anonymous and the account doesn't exist, so create one
		account = state.NewTokenAccount(toUrl.String(), tokenUrl.String())
		account.Type = types.ChainTypeAnonTokenAccount
		acctObj = new(state.Object)
	}

	//all is good, so subtract the balance
	err = account.AddBalance(deposit.DepositAmount.AsBigInt())
	if err != nil {
		return nil, fmt.Errorf("unable to add deposit balance to account")
	}

	//create a transaction reference chain acme-xxxxx/0, 1, 2, ... n.
	//This will reference the txid to keep the history
	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()
	refUrl := toUrl.JoinPath(fmt.Sprint(account.TxCount))
	txr := state.NewTxReference(refUrl.String(), txHash[:])
	txrData, err := txr.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to process transaction reference chain %v", err)
	}

	//now create the chain state object.
	txRefChain := new(state.Object)
	txRefChain.Entry = txrData
	txRefChainId := types.Bytes(refUrl.ResourceChain()).AsBytes32()

	//increment the token transaction count
	account.TxCount++

	acctChainId := types.Bytes(toUrl.ResourceChain()).AsBytes32()
	acctObj.Entry, err = account.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to process anon transaction account %v", err)
	}

	//if we get here with no errors store the states
	st.DB.AddStateEntry(&acctChainId, &txHash, acctObj)
	st.DB.AddStateEntry(&txRefChainId, &txHash, txRefChain)

	return new(DeliverTxResult), nil
}
