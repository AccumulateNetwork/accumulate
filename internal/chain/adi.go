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

func (ADI) createChain() types.TxType { return types.TxTypeIdentityCreate }

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

	switch st.AdiHeader.Type {
	case types.ChainTypeAnonTokenAccount:
		account := new(state.TokenAccount)
		err := st.AdiState.As(account)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling anon state account object, %v", err)
		}

	case types.ChainTypeAdi:
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

	default:
		return nil, fmt.Errorf("chain type %d cannot sponsor ADIs", st.AdiHeader.Type)
	}

	ic := api.ADI{}
	err := ic.UnmarshalBinary(tx.Transaction)
	if err != nil {
		return nil, fmt.Errorf("transaction is not a valid identity create message")
	}

	fromUrl := types.String(tx.SigInfo.URL)
	isc := synthetic.NewAdiStateCreate(tx.TransactionHash(), &fromUrl, &ic.URL, &ic.PublicKeyHash)
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
