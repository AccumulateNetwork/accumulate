package chain

import (
	"fmt"
	"math/big"

	"github.com/AccumulateNetwork/accumulated/internal/url"
	"github.com/AccumulateNetwork/accumulated/protocol"
	"github.com/AccumulateNetwork/accumulated/types/api"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
)

type TokenTx struct{}

func (TokenTx) Type() types.TxType { return types.TxTypeTokenTx }

func checkTokenTx(st *state.StateEntry, tx *transactions.GenTransaction) (*api.TokenTx, tokenChain, *big.Int, error) {
	if st.ChainHeader == nil {
		return nil, nil, nil, fmt.Errorf("sponsor not found")
	}

	body := new(api.TokenTx)
	err := tx.As(body)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid payload: %v", err)
	}

	var account tokenChain
	switch st.ChainHeader.Type {
	case types.ChainTypeTokenAccount:
		account = new(state.TokenAccount)
	case types.ChainTypeAnonTokenAccount:
		account = new(protocol.AnonTokenAccount)
	default:
		return nil, nil, nil, fmt.Errorf("%v cannot sponsor token transactions", st.ChainHeader.Type)
	}

	err = st.ChainState.As(account)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to decode sponsor: %v", err)
	}

	//now check to see if we can transact
	//really only need to provide one input...
	//now check to see if the account is good to send tokens from
	amt := types.Amount{}
	txAmt := big.NewInt(0)
	for _, val := range body.To {
		amt.Add(amt.AsBigInt(), txAmt.SetUint64(val.Amount))
	}

	if !account.CanDebitTokens(&amt.Int) {
		return nil, nil, nil, fmt.Errorf("insufficient balance")
	}

	return body, account, &amt.Int, nil
}

func (c TokenTx) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	_, _, _, err := checkTokenTx(st, tx)
	return err
}

func (c TokenTx) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	body, account, debit, err := checkTokenTx(st, tx)
	if err != nil {
		return nil, err
	}

	fromUrl, err := url.Parse(tx.SigInfo.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid sponsor URL: %v", err)
	}

	tokenUrl, err := account.ParseTokenUrl()
	if err != nil {
		return nil, fmt.Errorf("invalid token URL: %v", err)
	}
	tokStr := types.String(tokenUrl.String())

	if body.From.String != types.String(tx.SigInfo.URL) {
		return nil, fmt.Errorf("withdraw address and transaction sponsor do not match")
	}

	//now build the synthetic transactions.
	txid := types.Bytes(tx.TransactionHash())
	res := new(DeliverTxResult)
	for _, val := range body.To {
		txAmt := new(big.Int)
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
		err = depositTx.SetDeposit(&tokStr, txAmt)
		if err != nil {
			return nil, fmt.Errorf("unable to set deposit for synthetic token deposit transaction, %v", err)
		}

		gtx.Transaction, err = depositTx.MarshalBinary()
		if err != nil {
			return nil, fmt.Errorf("unable to marshal synthetic token transaction deposit, %v", err)
		}

		res.AddSyntheticTransaction(gtx)
	}

	if !account.DebitTokens(debit) {
		return nil, fmt.Errorf("%q balance is insufficient", fromUrl)
	}

	txHash := txid.AsBytes32()
	//create a transaction reference chain acme-xxxxx/0, 1, 2, ... n.
	//This will reference the txid to keep the history
	refUrl := fromUrl.JoinPath(fmt.Sprint(account.NextTx()))
	txr := state.NewTxReference(refUrl.String(), txHash[:])
	txrData, err := txr.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("unable to process transaction reference chain %v", err)
	}

	//now create the chain state object.
	txRefChain := new(state.Object)
	txRefChain.Entry = txrData
	txRefChainId := types.Bytes(refUrl.ResourceChain()).AsBytes32()

	//want to pass back just an interface rather than marshaled data, for now just
	//do marshaled data
	data, err := account.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal account state: %v", err)
	}

	//now update the state
	st.DB.AddStateEntry(st.ChainId, &txHash, &state.Object{Entry: data})
	st.DB.AddStateEntry(&txRefChainId, &txHash, txRefChain)

	return res, nil
}
