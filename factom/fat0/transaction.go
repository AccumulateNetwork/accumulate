// MIT License
//
// Copyright 2018 Canonical Ledgers, LLC
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS
// IN THE SOFTWARE.

package fat0

import (
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/factom"
	"github.com/AccumulateNetwork/accumulated/factom/fat"
	"github.com/AccumulateNetwork/accumulated/factom/fat103"
	"github.com/AccumulateNetwork/accumulated/factom/jsonlen"
)

const Type = fat.TypeFAT0

// Transaction represents a fat0 transaction, which can be a normal account
// transaction or a coinbase transaction depending on the Inputs and the
// RCD/signature pair.
type Transaction struct {
	Inputs  AddressAmountMap `json:"inputs"`
	Outputs AddressAmountMap `json:"outputs"`

	Metadata json.RawMessage `json:"metadata,omitempty"`

	Entry factom.Entry `json:"-"`
}

func NewTransaction(e factom.Entry, idKey *factom.Bytes32) (Transaction, error) {
	var t Transaction
	if err := t.UnmarshalJSON(e.Content); err != nil {
		return t, err
	}

	if t.Inputs.Sum() != t.Outputs.Sum() {
		return t, fmt.Errorf("sum(inputs) != sum(outputs)")
	}

	var expected map[factom.Bytes32]struct{}
	// Coinbase transactions must only have one input.
	if t.IsCoinbase() {
		if len(t.Inputs) != 1 {
			return t, fmt.Errorf("invalid coinbase transaction")
		}

		expected = map[factom.Bytes32]struct{}{*idKey: struct{}{}}
	} else {
		expected = make(map[factom.Bytes32]struct{}, len(t.Inputs))
		for adr := range t.Inputs {
			expected[factom.Bytes32(adr)] = struct{}{}
		}
	}

	if err := fat103.Validate(e, expected); err != nil {
		return t, err
	}

	t.Entry = e

	return t, nil
}

func (t *Transaction) UnmarshalJSON(data []byte) error {
	data = jsonlen.Compact(data)
	var tRaw struct {
		Inputs   json.RawMessage `json:"inputs"`
		Outputs  json.RawMessage `json:"outputs"`
		Metadata json.RawMessage `json:"metadata,omitempty"`
	}
	if err := json.Unmarshal(data, &tRaw); err != nil {
		return fmt.Errorf("%T: %w", t, err)
	}
	if err := t.Inputs.UnmarshalJSON(tRaw.Inputs); err != nil {
		return fmt.Errorf("%T.Inputs: %w", t, err)
	}
	if err := t.Outputs.UnmarshalJSON(tRaw.Outputs); err != nil {
		return fmt.Errorf("%T.Outputs: %w", t, err)
	}
	t.Metadata = tRaw.Metadata

	expectedJSONLen := len(`{"inputs":,"outputs":}`) +
		len(tRaw.Inputs) + len(tRaw.Outputs)
	if tRaw.Metadata != nil {
		expectedJSONLen += len(`,"metadata":`) + len(tRaw.Metadata)
	}
	if expectedJSONLen != len(data) {
		return fmt.Errorf("%T: unexpected JSON length", t)
	}

	return nil
}

func (t Transaction) IsCoinbase() bool {
	_, ok := t.Inputs[fat.Coinbase()]
	return ok
}

func (t Transaction) String() string {
	data, err := json.Marshal(t)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

func (t Transaction) Sign(signingSet ...factom.RCDSigner) (factom.Entry, error) {
	e := t.Entry
	content, err := json.Marshal(t)
	if err != nil {
		return e, err
	}
	e.Content = content
	return fat103.Sign(e, signingSet...), nil
}
