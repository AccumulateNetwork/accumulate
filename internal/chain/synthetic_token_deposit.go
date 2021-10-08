package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
)

type SynthTokenDeposit struct{}

func (SynthTokenDeposit) createChain() types.TxType {
	return types.TxTypeSyntheticTokenDeposit
}

func (SynthTokenDeposit) BeginBlock() {}

func (SynthTokenDeposit) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	return nil
}

func (SynthTokenDeposit) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	deposit := new(synthetic.TokenTransactionDeposit)
	err := tx.As(deposit)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal deposit: %v", err)
	}

	// TODO check if the TokenUrl matches the anonymous account URL
	if deposit.TokenUrl != "dc/ACME" {
		return nil, fmt.Errorf("depositing non-ACME tokens has not been implemented")
	}

	//derive the chain for the token account
	adi, _, err := types.ParseIdentityChainPath(deposit.ToUrl.AsString())
	if err != nil {
		return nil, err
	}

	//make sure this is an anonymous address
	if err = anon.IsAcmeAddress(adi); err != nil {
		//need to return to sender
		return nil, fmt.Errorf("deposit token account does not exist and the deposit address is not an anonymous account, %v", err)
	}

	//now check if the anonymous chain already exists.
	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()

	adiChainId := types.GetChainIdFromChainPath(&adi)

	var account *state.TokenAccount

	// if the identity state is nil, then it means we do not have any anon accts setup yet.
	if st.AdiState == nil {
		//setup an anon token account
		object := new(state.Object)
		st.AdiState = object
		account = state.NewTokenAccount(adi, *deposit.TokenUrl.AsString())
		account.Type = types.ChainTypeAnonTokenAccount
	} else {
		//check to see if we have an anon account
		if st.AdiHeader.Type != types.ChainTypeAnonTokenAccount {
			return nil, fmt.Errorf("adi for an anoymous chain is not an anonymous account")
		}
		//now can unmarshal the account
		account = new(state.TokenAccount)
		err := account.UnmarshalBinary(st.AdiState.Entry)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling anon state account object, %v", err)
		}
	}

	//all is good, so subtract the balance
	err = account.AddBalance(deposit.DepositAmount.AsBigInt())
	if err != nil {
		return nil, fmt.Errorf("unable to add deposit balance to account")
	}

	//create a transaction reference chain acme-xxxxx/0, 1, 2, ... n.
	//This will reference the txid to keep the history
	refUrl := fmt.Sprintf("%s/%d", adi, account.TxCount)
	txr := state.NewTxReference(refUrl, txHash[:])
	txrData, err := txr.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to process transaction reference chain %v", err)
	}

	//now create the chain state object.
	txRefChain := new(state.Object)
	txRefChain.Entry = txrData
	txRefChainId := types.GetChainIdFromChainPath(&refUrl)

	//increment the token transaction count
	account.TxCount++

	data, err := account.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to process anon transaction account %v", err)
	}

	st.AdiState.Entry = data

	//if we get here with no errors store the states
	st.DB.AddStateEntry(adiChainId, &txHash, st.AdiState)
	st.DB.AddStateEntry(txRefChainId, &txHash, txRefChain)

	return new(DeliverTxResult), nil
}
