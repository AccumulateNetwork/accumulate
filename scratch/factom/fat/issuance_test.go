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
	"testing"

	"github.com/AccumulateNetwork/accumulated/factom"
	"github.com/AccumulateNetwork/accumulated/factom/fat103"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var coinbaseAddressStr = "FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC"

func TestCoinbase(t *testing.T) {
	a := Coinbase()
	assert.Equal(t, coinbaseAddressStr, a.String())
}

var (
	issuerKey    = issuerSecret.ID1Key()
	issuerSecret = func() factom.SK1Key {
		a, _ := factom.GenerateSK1Key()
		return a
	}()
	chainID = factom.Bytes32{1: 1, 2: 2}
)

type issuanceTest struct {
	Name      string
	Error     string
	Entry     factom.Entry
	IssuerKey *factom.ID1Key
}

var issuanceTests = []issuanceTest{{
	Name: "valid",
	Entry: func() factom.Entry {
		e, err := Issuance{
			Type:      TypeFAT0,
			Supply:    100000,
			Precision: 3,
			Symbol:    "test",
			Metadata:  json.RawMessage(`"memo"`),

			Entry: factom.Entry{ChainID: &chainID},
		}.Sign(issuerSecret)
		if err != nil {
			panic(err)
		}
		return e
	}(),
}, {
	Name: "valid (omit symbol)",
	Entry: func() factom.Entry {
		e, err := Issuance{
			Type:      TypeFAT0,
			Supply:    100000,
			Precision: 3,
			Metadata:  json.RawMessage(`"memo"`),

			Entry: factom.Entry{ChainID: &chainID},
		}.Sign(issuerSecret)
		if err != nil {
			panic(err)
		}
		return e
	}(),
}, {
	Name: "valid (omit metadata)",
	Entry: func() factom.Entry {
		e, err := Issuance{
			Type:      TypeFAT0,
			Supply:    100000,
			Precision: 3,
			Symbol:    "test",

			Entry: factom.Entry{ChainID: &chainID},
		}.Sign(issuerSecret)
		if err != nil {
			panic(err)
		}
		return e
	}(),
}, {
	Name: "valid (type 1)",
	Entry: func() factom.Entry {
		e, err := Issuance{
			Type:     TypeFAT1,
			Supply:   100000,
			Symbol:   "test",
			Metadata: json.RawMessage(`"memo"`),

			Entry: factom.Entry{ChainID: &chainID},
		}.Sign(issuerSecret)
		if err != nil {
			panic(err)
		}
		return e
	}(),
}, {
	Name: "valid (unlimited)",
	Entry: func() factom.Entry {
		e, err := Issuance{
			Type:     TypeFAT0,
			Supply:   -1,
			Symbol:   "test",
			Metadata: json.RawMessage(`"memo"`),

			Entry: factom.Entry{ChainID: &chainID},
		}.Sign(issuerSecret)
		if err != nil {
			panic(err)
		}
		return e
	}(),
}, {
	Name:  "invalid JSON (unknown field)",
	Error: `*fat.Issuance: unexpected JSON length`,
	Entry: func() factom.Entry {
		e, err := Issuance{
			Type:     TypeFAT0,
			Supply:   -1,
			Symbol:   "test",
			Metadata: json.RawMessage(`"memo"`),

			Entry: factom.Entry{ChainID: &chainID},
		}.Sign(issuerSecret)
		if err != nil {
			panic(err)
		}
		var m map[string]interface{}
		if err := json.Unmarshal(e.Content, &m); err != nil {
			panic(err)
		}
		m["newfield"] = 5
		content, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}
		e.Content = content
		return fat103.Sign(e, issuerSecret)
	}(),
}, {
	Name:  "invalid (supply<-1)",
	Error: `invalid "supply": must be positive or -1`,
	Entry: func() factom.Entry {
		e, err := Issuance{
			Type:      TypeFAT1,
			Supply:    -10,
			Symbol:    "test",
			Metadata:  json.RawMessage(`"memo"`),
			Precision: 3,

			Entry: factom.Entry{ChainID: &chainID},
		}.Sign(issuerSecret)
		if err != nil {
			panic(err)
		}
		return e
	}(),
}, {
	Name:  "invalid (symbol)",
	Error: `invalid "symbol": exceeds 4 characters`,
	Entry: func() factom.Entry {
		e, err := Issuance{
			Type:      TypeFAT0,
			Supply:    100000,
			Precision: 3,
			Symbol:    "testasdfdfsa",
			Metadata:  json.RawMessage(`"memo"`),

			Entry: factom.Entry{ChainID: &chainID},
		}.Sign(issuerSecret)
		if err != nil {
			panic(err)
		}
		return e
	}(),
}, {
	Name:  "invalid (supply=0)",
	Error: `invalid "supply": must be positive or -1`,
	Entry: func() factom.Entry {
		e, err := Issuance{
			Type:      TypeFAT1,
			Symbol:    "test",
			Metadata:  json.RawMessage(`"memo"`),
			Precision: 3,

			Entry: factom.Entry{ChainID: &chainID},
		}.Sign(issuerSecret)
		if err != nil {
			panic(err)
		}
		return e
	}(),
}, {
	Name:  "invalid (fat1, precision)",
	Error: `invalid "precision": not allowed for FAT-1`,
	Entry: func() factom.Entry {
		e, err := Issuance{
			Type:      TypeFAT1,
			Supply:    -1,
			Symbol:    "test",
			Metadata:  json.RawMessage(`"memo"`),
			Precision: 3,

			Entry: factom.Entry{ChainID: &chainID},
		}.Sign(issuerSecret)
		if err != nil {
			panic(err)
		}
		return e
	}(),
}, {
	Name:  "invalid (fat0, precision>18 )",
	Error: `invalid "precision": out of range [0-18]`,
	Entry: func() factom.Entry {
		e, err := Issuance{
			Type:      TypeFAT0,
			Supply:    -1,
			Symbol:    "test",
			Metadata:  json.RawMessage(`"memo"`),
			Precision: 30,

			Entry: factom.Entry{ChainID: &chainID},
		}.Sign(issuerSecret)
		if err != nil {
			panic(err)
		}
		return e
	}(),
}, {
	Name:  "invalid (type)",
	Error: `invalid "type": FAT-1000000000`,
	Entry: func() factom.Entry {
		e, err := Issuance{
			Type:      1000000000,
			Supply:    -1,
			Symbol:    "test",
			Metadata:  json.RawMessage(`"memo"`),
			Precision: 3,

			Entry: factom.Entry{ChainID: &chainID},
		}.Sign(issuerSecret)
		if err != nil {
			panic(err)
		}
		return e
	}(),
}, {
	Name:      "invalid (issuer key)",
	Error:     `ExtIDs[1]: unexpected or duplicate RCD Hash`,
	IssuerKey: new(factom.ID1Key),
	Entry: func() factom.Entry {
		e, err := Issuance{
			Type:      TypeFAT0,
			Supply:    100000,
			Precision: 3,
			Symbol:    "test",
			Metadata:  json.RawMessage(`"memo"`),

			Entry: factom.Entry{ChainID: &chainID},
		}.Sign(issuerSecret)
		if err != nil {
			panic(err)
		}
		return e
	}(),
}}

func TestIssuance(t *testing.T) {
	for _, test := range issuanceTests {
		test := test
		t.Run(test.Name, func(t *testing.T) { testIssuance(t, test) })
	}
}

func testIssuance(t *testing.T, test issuanceTest) {
	assert := assert.New(t)
	require := require.New(t)
	issuerKey := &issuerKey
	if test.IssuerKey != nil {
		issuerKey = test.IssuerKey
	}
	i, err := NewIssuance(test.Entry, (*factom.Bytes32)(issuerKey))
	if len(test.Error) == 0 {
		require.NoError(err)
		assert.NotEqual(Issuance{}, i)
		return
	}
	assert.EqualError(err, test.Error)
}
