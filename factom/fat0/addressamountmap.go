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
	"github.com/AccumulateNetwork/accumulated/factom/jsonlen"
)

// AddressAmountMap relates a factom.FAAddress to its amount for the Inputs and
// Outputs of a Transaction.
type AddressAmountMap map[factom.FAAddress]uint64

// MarshalJSON marshals a list of addresses and amounts used in the inputs or
// outputs of a transaction. Addresses with a 0 amount are omitted.
func (m AddressAmountMap) MarshalJSON() ([]byte, error) {
	if len(m) == 0 {
		return nil, fmt.Errorf("empty")
	}
	adrStrAmountMap := make(map[string]uint64, len(m))
	for adr, amount := range m {
		adrStrAmountMap[adr.String()] = amount
	}
	return json.Marshal(adrStrAmountMap)
}

var adrStrLen = len(factom.FAAddress{}.String())

// UnmarshalJSON unmarshals a list of addresses and amounts used in the inputs
// or outputs of a transaction. Duplicate addresses or addresses with a 0
// amount cause an error.
func (m *AddressAmountMap) UnmarshalJSON(data []byte) error {
	var adrStrAmountMap map[string]uint64
	if err := json.Unmarshal(data, &adrStrAmountMap); err != nil {
		return err
	}
	if len(adrStrAmountMap) == 0 {
		return fmt.Errorf("%T: empty", m)
	}
	adrJSONLen := len(`"":,`) + adrStrLen
	expectedJSONLen := len(`{}`) + len(adrStrAmountMap)*adrJSONLen - len(`,`)
	*m = make(AddressAmountMap, len(adrStrAmountMap))
	var adr factom.FAAddress
	for adrStr, amount := range adrStrAmountMap {
		if err := adr.Set(adrStr); err != nil {
			return fmt.Errorf("%T: %w", m, err)
		}
		(*m)[adr] = amount
		expectedJSONLen += jsonlen.Uint64(amount)
	}
	if expectedJSONLen != len(data) {
		return fmt.Errorf("%T: unexpected JSON length", m)
	}
	return nil
}

// Sum returns the sum of all amount values.
func (m AddressAmountMap) Sum() uint64 {
	var sum uint64
	for _, amount := range m {
		sum += amount
	}
	return sum
}

func (m AddressAmountMap) noAddressIntersection(n AddressAmountMap) error {
	short, long := m, n
	if len(short) > len(long) {
		short, long = long, short
	}
	for adr, amount := range short {
		if amount == 0 {
			continue
		}
		if amount := long[adr]; amount != 0 {
			return fmt.Errorf("duplicate address: %v", adr)
		}
	}
	return nil
}
