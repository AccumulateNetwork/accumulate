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

func (SynthTokenDeposit) Type() types.TxType {
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

	if deposit.ToUrl != types.String(tx.SigInfo.URL) {
		return nil, fmt.Errorf("deposit destination does not match TX sponsor")
	}

	_, chainPath, _ := types.ParseIdentityChainPath(&tx.SigInfo.URL)
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
	} else if anon.IsAcmeAddress(tx.SigInfo.URL) == nil {
		// Address is anonymous and the account doesn't exist, so create one
		account = state.NewTokenAccount(chainPath, *deposit.TokenUrl.AsString())
		account.Type = types.ChainTypeAnonTokenAccount
		acctObj = new(state.Object)
	} else {
		return nil, fmt.Errorf("could not find token account")
	}

	//all is good, so subtract the balance
	err = account.AddBalance(deposit.DepositAmount.AsBigInt())
	if err != nil {
		return nil, fmt.Errorf("unable to add deposit balance to account")
	}

	//create a transaction reference chain acme-xxxxx/0, 1, 2, ... n.
	//This will reference the txid to keep the history
	txHash := types.Bytes(tx.TransactionHash()).AsBytes32()
	refUrl := fmt.Sprintf("%s/%d", chainPath, account.TxCount)
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

	acctObj.Entry, err = account.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to process anon transaction account %v", err)
	}

	//if we get here with no errors store the states
	st.DB.AddStateEntry(st.ChainId, &txHash, acctObj)
	st.DB.AddStateEntry(txRefChainId, &txHash, txRefChain)

	return new(DeliverTxResult), nil
}
