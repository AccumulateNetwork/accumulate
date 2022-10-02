// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"bytes"
	"encoding/json"
	"io"
)

func NewTransactionResult(typ TransactionType) (TransactionResult, error) {
	switch typ {
	case TransactionTypeWriteData:
		return new(WriteDataResult), nil
	case TransactionTypeAddCredits:
		return new(AddCreditsResult), nil
	case TransactionTypeUnknown:
		return new(EmptyResult), nil
	}

	// Is the transaction type valid?
	_, err := NewTransactionBody(typ)
	if err != nil {
		return nil, err
	}

	return new(EmptyResult), nil
}

func EqualTransactionResult(a, b TransactionResult) bool {
	if a == b {
		return true
	}
	// TODO Find a way to generate this
	switch a := a.(type) {
	case *WriteDataResult:
		b, ok := b.(*WriteDataResult)
		return ok && a.Equal(b)
	case *AddCreditsResult:
		b, ok := b.(*AddCreditsResult)
		return ok && a.Equal(b)
	case *EmptyResult:
		b, ok := b.(*EmptyResult)
		return ok && a.Equal(b)
	default:
		return false
	}
}

func UnmarshalTransactionResult(data []byte) (TransactionResult, error) {
	typ, err := UnmarshalTransactionType(bytes.NewReader(data))
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
	typ, err := UnmarshalTransactionType(rd)
	if err != nil {
		return nil, err
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
	var typ *struct{ Type TransactionType }
	err := json.Unmarshal(data, &typ)
	if err != nil {
		return nil, err
	}

	if typ == nil {
		return nil, nil
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
