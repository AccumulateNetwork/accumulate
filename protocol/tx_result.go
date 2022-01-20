package protocol

import (
	"encoding"
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulate/types"
)

func NewTransactionResult(typ types.TransactionType) (TransactionResult, error) {
	switch typ {
	case types.TxTypeWriteData:
		return new(WriteDataResult), nil

	case types.TxTypeUnknown:
		return new(EmptyResult), nil
	}

	// Is the transaction type valid?
	_, err := NewTransaction(typ)
	if err != nil {
		return nil, err
	}

	return new(EmptyResult), nil
}

func UnmarshalTransactionResult(data []byte) (TransactionResult, error) {
	var typ types.TransactionType
	err := typ.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	tx, err := NewTransactionResult(typ)
	if err != nil {
		return nil, err
	}

	err = tx.UnmarshalBinary(data)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func UnmarshalTransactionResultJSON(data []byte) (TransactionResult, error) {
	var typ struct{ Type types.TransactionType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	tx, err := NewTransactionResult(typ.Type)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, tx)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

type TransactionResult interface {
	GetType() types.TxType
	BinarySize() int
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

type EmptyResult struct{}

func (r *EmptyResult) GetType() types.TxType {
	return types.TxTypeUnknown
}

func (r *EmptyResult) BinarySize() int {
	return types.TxTypeUnknown.BinarySize()
}

func (r *EmptyResult) MarshalBinary() (data []byte, err error) {
	return types.TxTypeUnknown.MarshalBinary()
}

func (r *EmptyResult) UnmarshalBinary(data []byte) error {
	var typ types.TransactionType
	err := typ.UnmarshalBinary(data)
	if err != nil {
		return err
	}
	if typ != types.TxTypeUnknown {
		return fmt.Errorf("want %v, got %v", types.TxTypeUnknown, typ)
	}
	return nil
}
