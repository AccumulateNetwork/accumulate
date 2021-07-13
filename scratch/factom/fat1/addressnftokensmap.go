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

package fat1

import (
	"encoding/json"
	"fmt"

	"github.com/AccumulateNetwork/accumulated/factom"
	"github.com/AccumulateNetwork/accumulated/factom/jsonlen"
)

// AddressTokenMap relates the RCDHash of an address to its NFTokenIDs.
type AddressNFTokensMap map[factom.FAAddress]NFTokens

func (m AddressNFTokensMap) MarshalJSON() ([]byte, error) {
	adrStrTknsMap := make(map[string]NFTokens, len(m))
	for adr, tkns := range m {
		adrStrTknsMap[adr.String()] = tkns
	}
	return json.Marshal(adrStrTknsMap)
}

var adrStrLen = len(factom.FAAddress{}.String())

func (m *AddressNFTokensMap) UnmarshalJSON(data []byte) error {
	var adrStrDataMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &adrStrDataMap); err != nil {
		return fmt.Errorf("%T: %w", m, err)
	}
	if len(adrStrDataMap) == 0 {
		return fmt.Errorf("%T: empty", m)
	}
	adrJSONLen := len(`"":,`) + adrStrLen
	expectedJSONLen := len(`{}`) - len(`,`) + len(adrStrDataMap)*adrJSONLen
	*m = make(AddressNFTokensMap, len(adrStrDataMap))
	var numTkns int
	for adrStr, data := range adrStrDataMap {
		adr, err := factom.NewFAAddress(adrStr)
		if err != nil {
			return fmt.Errorf("%T: %#v: %w", m, adrStr, err)
		}
		var tkns NFTokens
		if err := tkns.UnmarshalJSON(data); err != nil {
			return fmt.Errorf("%T: %v: %w", m, err, adr)
		}
		numTkns += len(tkns)
		if numTkns > maxCapacity {
			return fmt.Errorf("%T(len:%v): %T(len:%v): %v",
				m, numTkns-len(tkns), tkns, len(tkns), ErrorCapacity)
		}
		(*m)[adr] = tkns
		expectedJSONLen += len(jsonlen.Compact(data))
	}
	if expectedJSONLen != len(jsonlen.Compact(data)) {
		return fmt.Errorf("%T: unexpected JSON length", m)
	}
	return nil
}

func (m AddressNFTokensMap) nfTokenIDsConserved(n AddressNFTokensMap) error {
	if m.Sum() != n.Sum() {
		return fmt.Errorf("number of NFTokenIDs differ")
	}
	allTkns, err := m.AllNFTokens()
	if err != nil {
		return err
	}
	for _, tkns := range n {
		if err := allTkns.ContainsAll(tkns); err != nil {
			return err
		}
	}
	return nil
}

func (m AddressNFTokensMap) AllNFTokens() (NFTokens, error) {
	allTkns := make(NFTokens, m.Sum())
	for _, tkns := range m {
		for tknID := range tkns {
			if err := allTkns.set(tknID); err != nil {
				return nil, err
			}
		}
	}
	return allTkns, nil
}

func (m AddressNFTokensMap) Sum() uint64 {
	var sum uint64
	for _, tkns := range m {
		sum += uint64(len(tkns))
	}
	return sum
}

func (m AddressNFTokensMap) Owner(tknID NFTokenID) factom.FAAddress {
	var adr factom.FAAddress
	var tkns NFTokens
	for adr, tkns = range m {
		if _, ok := tkns[tknID]; ok {
			break
		}
	}
	return adr
}
