package api

import (
	"fmt"

	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/common"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/state"
)

func unmarshalState(b []byte) (*state.Object, state.Chain, error) {
	var obj state.Object
	err := obj.UnmarshalBinary(b)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid state response: %v", err)
	}

	chain, err := protocol.UnmarshalChain(obj.Entry)
	if err != nil {
		return nil, nil, err
	}

	return &obj, chain, nil
}

func unmarshalTxType(b []byte) types.TxType {
	v, _ := common.BytesUint64(b)
	return types.TxType(v)
}

func unmarshalTxPayload(b []byte) (protocol.TransactionPayload, error) {
	var payload protocol.TransactionPayload
	switch typ := unmarshalTxType(b); typ {
	case types.TxTypeSendTokens:
		payload = new(protocol.SendTokens)
	case types.TxTypeSyntheticDepositTokens:
		payload = new(protocol.SyntheticDepositTokens)
	case types.TxTypeCreateIdentity:
		payload = new(protocol.IdentityCreate)
	case types.TxTypeCreateToken:
		payload = new(protocol.CreateToken)
	case types.TxTypeCreateTokenAccount:
		payload = new(protocol.TokenAccountCreate)
	case types.TxTypeCreateKeyPage:
		payload = new(protocol.CreateKeyPage)
	case types.TxTypeCreateKeyBook:
		payload = new(protocol.CreateKeyBook)
	case types.TxTypeAddCredits:
		payload = new(protocol.AddCredits)
	case types.TxTypeUpdateKeyPage:
		payload = new(protocol.UpdateKeyPage)
	case types.TxTypeSyntheticCreateChain:
		payload = new(protocol.SyntheticCreateChain)
	case types.TxTypeSyntheticDepositCredits:
		payload = new(protocol.SyntheticDepositCredits)
	case types.TxTypeSyntheticGenesis:
		payload = new(protocol.SyntheticGenesis)
	case types.TxTypeAcmeFaucet:
		payload = new(protocol.AcmeFaucet)
	case types.TxTypeCreateDataAccount:
		payload = new(protocol.CreateDataAccount)
	case types.TxTypeWriteData:
		payload = new(protocol.WriteData)
	default:
		return nil, fmt.Errorf("unknown TX type %v", typ)
	}

	err := payload.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}

	return payload, nil
}

func unmarshalTxResponse(mainData, pendData []byte) (*state.Transaction, *state.PendingTransaction, protocol.TransactionPayload, error) {
	var mainObj, pendObj state.Object
	var main *state.Transaction
	var pend *state.PendingTransaction

	err := mainObj.UnmarshalBinary(mainData)
	if err == nil {
		main = new(state.Transaction)
		err = mainObj.As(main)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid TX response: %v", err)
		}
	}

	err = pendObj.UnmarshalBinary(pendData)
	if err == nil {
		pend = new(state.PendingTransaction)
		err = pendObj.As(pend)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid TX response: %v", err)
		}
	}

	var payload protocol.TransactionPayload
	switch {
	case main != nil:
		payload, err = unmarshalTxPayload(*main.Transaction)

	case pend == nil:
		// TX state can be nil missing if the transaction is pending. TX pending
		// state can be purged. It is only an error if both are missing.
		return nil, nil, nil, fmt.Errorf("invalid TX response: %v", err)

	case pend.TransactionState == nil:
		return nil, nil, nil, fmt.Errorf("no transaction state for transaction on pending or main chains")

	default: // pend != nil && pend.TransactionState != nil
		payload, err = unmarshalTxPayload(*pend.TransactionState.Transaction)
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid TX response: %v", err)
	}

	return main, pend, payload, nil
}
