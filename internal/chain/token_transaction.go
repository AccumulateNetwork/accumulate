package chain

import (
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/types/api"

	"github.com/AccumulateNetwork/accumulated/types"
	anon "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
)

type TokenTx struct{}

func (TokenTx) Type() types.TxType { return types.TxTypeTokenTx }

func (c TokenTx) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	return nil
}

func (c TokenTx) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	res := new(DeliverTxResult)

	if st.ChainHeader == nil {
		return nil, fmt.Errorf("cannot find state for %q", tx.SigInfo.URL)
	}
	if st.AdiState == nil {
		return nil, fmt.Errorf("cannot find identity for %q", tx.SigInfo.URL)
	}

	var adi *state.AdiState
	var err error
	if st.AdiHeader.Type == types.ChainTypeAdi {
		adi = new(state.AdiState)
		err = st.AdiState.As(adi)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal identity: %v", err)
		}
	}

	fromUrl, err := url.Parse(tx.SigInfo.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid sponsor URL: %v", err)
	}

	withdrawal := api.TokenTx{}
	err = withdrawal.UnmarshalBinary(tx.Transaction)
	if err != nil {
		return nil, fmt.Errorf("error with send token, %v", err)
	}

	if withdrawal.From.String != types.String(tx.SigInfo.URL) {
		return nil, fmt.Errorf("withdraw address and transaction sponsor do not match")
	}

	acctState := new(state.TokenAccount)
	err = st.ChainState.As(acctState)
	if err != nil {
		return nil, err
	}

	//now check to see if we can transact
	//really only need to provide one input...
	//now check to see if the account is good to send tokens from
	amt := types.Amount{}
	txAmt := big.NewInt(0)
	for _, val := range withdrawal.To {
		amt.Add(amt.AsBigInt(), txAmt.SetUint64(val.Amount))
	}

	if !acctState.CanTransact(amt.AsBigInt()) {
		return nil, fmt.Errorf("insufficient balance")
	}

	switch st.AdiHeader.Type {
	case types.ChainTypeAnonTokenAccount:
		address := anon.GenerateAcmeAddress(tx.Signature[0].PublicKey)
		if address != tx.SigInfo.URL {
			return nil, fmt.Errorf("TX signature key does not match the sponsor's key")
		}

	case types.ChainTypeAdi:
		if !adi.VerifyKey(tx.Signature[0].PublicKey) {
			return nil, fmt.Errorf("TX signature key does not match the sponsor's key")
		}
	}

	//now build the synthetic transactions.
	txid := types.Bytes(tx.TransactionHash())
	for _, val := range withdrawal.To {
		txAmt.SetUint64(val.Amount)
		//extract the target identity and chain from the url
		destAdi, destChainPath, err := types.ParseIdentityChainPath(val.URL.AsString())
		if err != nil {
			return nil, err
		}
		destUrl := types.String(destChainPath)

		//populate the synthetic transaction, each submission will be signed by BVC leader and dispatched
		gtx := new(transactions.GenTransaction)

		//set the identity chain for the destination
		gtx.Routing = types.GetAddressFromIdentity(&destAdi)
		gtx.ChainID = types.GetChainIdFromChainPath(destUrl.AsString()).Bytes()
		gtx.SigInfo = new(transactions.SignatureInfo)
		gtx.SigInfo.URL = destChainPath

		depositTx := synthetic.NewTokenTransactionDeposit(txid[:], &st.ChainHeader.ChainUrl, &destUrl)
		err = depositTx.SetDeposit(&acctState.TokenUrl.String, txAmt)
		if err != nil {
			return nil, fmt.Errorf("unable to set deposit for synthetic token deposit transaction, %v", err)
		}

		gtx.Transaction, err = depositTx.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("unable to marshal synthetic token transaction deposit, %v", err)
		}

		res.AddSyntheticTransaction(gtx)
	}

	err = acctState.SubBalance(amt.AsBigInt())
	if err != nil {
		return nil, fmt.Errorf("error subtracting balance from account acc://%s, %v", st.AdiHeader.ChainUrl, err)
	}

	txHash := txid.AsBytes32()
	//create a transaction reference chain acme-xxxxx/0, 1, 2, ... n.
	//This will reference the txid to keep the history
	refUrl := fromUrl.JoinPath(fmt.Sprint(acctState.TxCount))
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
	acctState.TxCount++

	//want to pass back just an interface rather than marshaled data, for now just
	//do marshaled data
	data, err := acctState.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal account state: %v", err)
	}
	st.AdiState.Entry = data

	//now update the state
	st.DB.AddStateEntry(st.ChainId, &txHash, st.AdiState)
	st.DB.AddStateEntry(&txRefChainId, &txHash, txRefChain)

	return res, nil
}
