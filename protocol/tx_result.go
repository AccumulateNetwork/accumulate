// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
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

func CopyTransactionResult(v TransactionResult) TransactionResult {
	return v.CopyAsInterface().(TransactionResult)
}

func UnmarshalTransactionResult(data []byte) (TransactionResult, error) {
	return UnmarshalTransactionResultFrom(bytes.NewReader(data))
}

func UnmarshalTransactionResultFrom(rd io.Reader) (TransactionResult, error) {
	reader := encoding.NewReader(rd)

	// Read the type code
	var typ TransactionType
	if !reader.ReadEnum(1, &typ) {
		return nil, fmt.Errorf("field Type: missing")
	}

	// Create a new transaction result
	tx, err := NewTransactionResult(typ)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result
	err = tx.UnmarshalFieldsFrom(reader)
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
