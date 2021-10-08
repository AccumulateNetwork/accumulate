package chain

import (
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulated/types/api"

	"github.com/AccumulateNetwork/accumulated/types"
	types2 "github.com/AccumulateNetwork/accumulated/types/anonaddress"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
)

type AnonToken struct{}

func (AnonToken) Type() types.TxType { return types.TxTypeTokenTx }

func (c AnonToken) BeginBlock() {}

func (c AnonToken) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	return nil
}

func (c AnonToken) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	res := new(DeliverTxResult)

	if st.AdiState == nil {
		return nil, fmt.Errorf("identity state does not exist for anonymous account")
	}

	//now check to make sure this is really an anon account
	if st.AdiHeader.Type != types.ChainTypeAnonTokenAccount {
		return nil, fmt.Errorf("account adi is not an anonymous account type")
	}

	var err error
	withdrawal := api.TokenTx{}
	err = withdrawal.UnmarshalBinary(tx.Transaction)
	if err != nil {
		return nil, fmt.Errorf("error with send token, %v", err)
	}

	//need to derive chain id for coin type account.
	adi, chain, _ := types.ParseIdentityChainPath(withdrawal.From.AsString())
	if adi != chain {
		return nil, fmt.Errorf("cannot specify sub accounts for anonymous token chains")
	}
	//specify the acme tokenUrl
	acmeTokenUrl := types.String("dc/ACME")

	//this is the actual account url the acme tokens are being sent from
	withdrawal.From = types.UrlChain{types.String(adi)}

	//get the ChainId of the acme account for the anon address.
	accountChainId := types.GetChainIdFromChainPath(withdrawal.From.AsString())

	//because we use a different chain for the anonymous account, we need to fetch it.
	st.ChainId = accountChainId
	st.ChainState, err = st.DB.GetCurrentEntry(accountChainId[:])
	if err != nil {
		return nil, fmt.Errorf("chain state for account not esablished")
	}

	tokenAccountState := new(state.TokenAccount)
	err = tokenAccountState.UnmarshalBinary(st.ChainState.Entry)
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

	if !tokenAccountState.CanTransact(amt.AsBigInt()) {
		return nil, fmt.Errorf("insufficient balance")
	}

	//so far, so good.  Now we need to check to make sure the signing address is ok.  maybe look at moving this upstream from here.
	//if we get to this function we know we at least have 1 signature
	address := types2.GenerateAcmeAddress(tx.Signature[0].PublicKey)

	//check the addresses to make sure they match
	if address != string(st.AdiHeader.ChainUrl) {
		return nil, fmt.Errorf("invalid address, public key address is %s but account %s ", address, st.AdiHeader.ChainUrl)
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
		gtx.SigInfo.URL = destAdi

		depositTx := synthetic.NewTokenTransactionDeposit(txid[:], &st.AdiHeader.ChainUrl, &destUrl)
		err = depositTx.SetDeposit(&acmeTokenUrl, txAmt)
		if err != nil {
			return nil, fmt.Errorf("unable to set deposit for synthetic token deposit transaction, %v", err)
		}

		gtx.Transaction, err = depositTx.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("unable to marshal synthetic token transaction deposit, %v", err)
		}

		res.AddSyntheticTransaction(gtx)
	}

	err = tokenAccountState.SubBalance(amt.AsBigInt())
	if err != nil {
		return nil, fmt.Errorf("error subtracting balance from account acc://%s, %v", st.AdiHeader.ChainUrl, err)
	}

	txHash := txid.AsBytes32()
	//create a transaction reference chain acme-xxxxx/0, 1, 2, ... n.
	//This will reference the txid to keep the history
	refUrl := fmt.Sprintf("%s/%d", adi, tokenAccountState.TxCount)
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
	tokenAccountState.TxCount++

	//want to pass back just an interface rather than marshaled data, for now just
	//do marshaled data
	data, err := tokenAccountState.MarshalBinary()
	if err != nil {
		panic("anon token end block, error marshaling account state.")
	}
	st.AdiState.Entry = data

	//now update the state
	st.DB.AddStateEntry(accountChainId, &txHash, st.AdiState)
	st.DB.AddStateEntry(txRefChainId, &txHash, txRefChain)

	return res, nil
}
