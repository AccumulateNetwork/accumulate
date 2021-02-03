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

package fat

import (
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/factom"
	"github.com/Factom-Asset-Tokens/factom/fat103"
	"github.com/Factom-Asset-Tokens/factom/jsonlen"
)

var coinbase = factom.FsAddress{}.FAAddress()

func Coinbase() factom.FAAddress {
	return coinbase
}

const MaxPrecision = 18

type Issuance struct {
	Type      Type  `json:"type"`
	Supply    int64 `json:"supply"`
	Precision uint  `json:"precision,omitempty"`

	Symbol string `json:"symbol,omitempty"`

	Metadata json.RawMessage `json:"metadata,omitempty"`

	Entry factom.Entry `json:"-"`
}

func NewIssuance(e factom.Entry, idKey *factom.Bytes32) (Issuance, error) {
	var i Issuance
	if err := i.UnmarshalJSON(e.Content); err != nil {
		return i, err
	}

	if i.Supply == 0 || i.Supply < -1 {
		return i, fmt.Errorf(`invalid "supply": must be positive or -1`)
	}

	if len(i.Symbol) > 4 {
		return i, fmt.Errorf(`invalid "symbol": exceeds 4 characters`)
	}

	switch i.Type {
	case TypeFAT0:
		if i.Precision != 0 && i.Precision > MaxPrecision {
			return i, fmt.Errorf(`invalid "precision": out of range [0-18]`)
		}
	case TypeFAT1:
		if i.Precision != 0 {
			return i, fmt.Errorf(
				`invalid "precision": not allowed for FAT-1`)
		}
	default:
		return i, fmt.Errorf(`invalid "type": %v`, i.Type)
	}

	expected := map[factom.Bytes32]struct{}{*idKey: struct{}{}}
	if err := fat103.Validate(e, expected); err != nil {
		return i, err
	}

	i.Entry = e

	return i, nil
}

func (i Issuance) Sign(idKey factom.RCDSigner) (factom.Entry, error) {
	e := i.Entry
	content, err := json.Marshal(i)
	if err != nil {
		return e, err
	}
	e.Content = content
	return fat103.Sign(e, idKey), nil
}

func (i *Issuance) UnmarshalJSON(data []byte) error {
	data = jsonlen.Compact(data)
	type _i Issuance
	if err := json.Unmarshal(data, (*_i)(i)); err != nil {
		return fmt.Errorf("%T: %w", i, err)
	}
	if i.expectedJSONLength() != len(data) {
		return fmt.Errorf("%T: unexpected JSON length", i)
	}
	return nil
}
func (i Issuance) expectedJSONLength() int {
	l := len(`{}`)
	l += len(`"type":""`) + len(i.Type.String())
	l += len(`,"supply":`) + jsonlen.Int64(i.Supply)
	if i.Precision != 0 {
		l += len(`,"precision":`) + jsonlen.Uint64(uint64(i.Precision))
	}
	if len(i.Symbol) > 0 {
		l += len(`,"symbol":""`) + len(i.Symbol)
	}
	if i.Metadata != nil {
		l += len(`,"metadata":`) + len(i.Metadata)
	}
	return l
}
