package protocol

import (
	"encoding"
	"encoding/json"
	"fmt"
	"io"

	accenc "gitlab.com/accumulatenetwork/accumulate/internal/encoding"
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

func UnmarshalTransactionResultFrom(rd io.ReadSeeker) (TransactionResult, error) {
	// Get the reader's current position
	pos, err := rd.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	// Read the type code
	reader := accenc.NewReader(rd)
	typ, ok := reader.ReadUint(1)
	_, err = reader.Reset([]string{"Type"})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("field Type: missing")
	}

	// Reset the reader's position
	_, err = rd.Seek(pos, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// Create a new transaction result
	tx, err := NewTransactionResult(TransactionType(typ))
	if err != nil {
		return nil, err
	}

	// Unmarshal the result
	err = tx.UnmarshalBinaryFrom(rd)
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
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
	UnmarshalBinaryFrom(io.Reader) error
}
