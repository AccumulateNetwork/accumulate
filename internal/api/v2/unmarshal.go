package api

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/state"
)

func unmarshalState(b []byte) (*state.Object, state.Chain, error) {
	var obj state.Object
	err := obj.UnmarshalBinary(b)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid state response: %v", err)
	}

	chain, err := protocol.UnmarshalAccount(obj.Entry)
	if err != nil {
		return nil, nil, err
	}

	return &obj, chain, nil
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
		payload = main.Transaction

	case pend == nil:
		// TX state can be nil missing if the transaction is pending. TX pending
		// state can be purged. It is only an error if both are missing.
		return nil, nil, nil, fmt.Errorf("invalid TX response: %v", err)

	case pend.TransactionState == nil:
		return nil, nil, nil, fmt.Errorf("no transaction state for transaction on pending or main chains")

	default: // pend != nil && pend.TransactionState != nil
		payload = pend.TransactionState.Transaction
	}

	return main, pend, payload, nil
}
