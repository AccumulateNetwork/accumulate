package chain

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulated/types"
	"github.com/AccumulateNetwork/accumulated/types/api"
	"github.com/AccumulateNetwork/accumulated/types/api/transactions"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/AccumulateNetwork/accumulated/types/synthetic"
)

type ADI struct{}

func (ADI) chainType() types.ChainType { return types.ChainTypeAdi }

func (ADI) instruction() types.TxType { return types.TxTypeIdentityCreate }

func (ADI) BeginBlock() {}

func (ADI) CheckTx(st *state.StateEntry, tx *transactions.GenTransaction) error {
	if st == nil {
		return fmt.Errorf("current state not defined")
	}
	return nil
}

func (ADI) DeliverTx(st *state.StateEntry, tx *transactions.GenTransaction) (*DeliverTxResult, error) {
	if st == nil {
		return nil, fmt.Errorf("current state not defined")
	}
	if st.AdiChain == nil {
		return nil, fmt.Errorf("missing sponsor identity")
	}
	if tx.Signature == nil {
		return nil, fmt.Errorf("no signatures available")
	}

	adiState := state.AdiState{}
	err := adiState.UnmarshalBinary(st.AdiState.Entry)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal adi state entry, %v", err)
	}

	//this should be done at a higher level...
	if !adiState.VerifyKey(tx.Signature[0].PublicKey) {
		return nil, fmt.Errorf("key is not supported by current ADI state")
	}

	if !adiState.VerifyAndUpdateNonce(tx.SigInfo.Nonce) {
		return nil, fmt.Errorf("invalid nonce, adi state %d but provided %d", adiState.Nonce, tx.SigInfo.Nonce)
	}

	ic := api.ADI{}
	err = ic.UnmarshalBinary(tx.Transaction)
	if err != nil {
		return nil, fmt.Errorf("transaction is not a valid identity create message")
	}

	isc := synthetic.NewAdiStateCreate(tx.TransactionHash(), &adiState.ChainUrl, &ic.URL, &ic.PublicKeyHash)
	iscData, err := isc.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal synthetic transaction")
	}

	syn := new(transactions.GenTransaction)
	syn.Routing = types.GetAddressFromIdentity(isc.ToUrl.AsString())
	syn.ChainID = types.GetChainIdFromChainPath(isc.ToUrl.AsString()).Bytes()
	syn.SigInfo = &transactions.SignatureInfo{}
	syn.SigInfo.URL = *isc.ToUrl.AsString()
	syn.Transaction = iscData

	res := new(DeliverTxResult)
	res.AddSyntheticTransaction(syn)
	return res, nil
}
