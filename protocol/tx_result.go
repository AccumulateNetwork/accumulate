package protocol

import (
	"encoding"
	"encoding/json"
	"fmt"
)

func NewTransactionResult(typ TransactionType) (TransactionResult, error) {
	switch typ {
	case TransactionTypeWriteData:
		return new(WriteDataResult), nil

	case TransactionTypeUnknown:
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
	var typ TransactionType
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
	var typ struct{ Type TransactionType }
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
	GetType() TransactionType
	BinarySize() int
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

type EmptyResult struct{}

func (r *EmptyResult) GetType() TransactionType {
	return TransactionTypeUnknown
}

func (r *EmptyResult) BinarySize() int {
	return TransactionTypeUnknown.BinarySize()
}

func (r *EmptyResult) MarshalBinary() (data []byte, err error) {
	return TransactionTypeUnknown.MarshalBinary()
}

func (r *EmptyResult) UnmarshalBinary(data []byte) error {
	var typ TransactionType
	err := typ.UnmarshalBinary(data)
	if err != nil {
		return err
	}
	if typ != TransactionTypeUnknown {
		return fmt.Errorf("want %v, got %v", TransactionTypeUnknown, typ)
	}
	return nil
}
