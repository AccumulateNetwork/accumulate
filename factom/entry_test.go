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

package factom_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/AdamSLevy/jsonrpc2/v14"
	. "github.com/Factom-Asset-Tokens/factom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var marshalBinaryTests = []struct {
	Name string
	Entry
}{{
	Name: "valid",
	Entry: Entry{
		Hash: func() *Bytes32 {
			b := NewBytes32(
				"72177d733dcd0492066b79c5f3e417aef7f22909674f7dc351ca13b04742bb91")
			return &b
		}(),
		ChainID: func() *Bytes32 {
			c := ComputeChainID([]Bytes{Bytes("test")})
			return &c
		}(),
		ExtIDs:  []Bytes{},
		Content: hexToBytes("5061796c6f616448657265"),
	},
}}

func TestEntryMarshalBinary(t *testing.T) {
	for _, test := range marshalBinaryTests {
		t.Run(test.Name, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)
			e := test.Entry
			data, err := e.MarshalBinary()
			require.NoError(err)
			hash := ComputeEntryHash(data)
			assert.Equal(*e.Hash, hash)
		})
	}
}

var unmarshalBinaryTests = []struct {
	Name  string
	Data  []byte
	Error string
	Hash  *Bytes32
}{{
	Name: "valid",
	Data: hexToBytes(
		"009005bb7dd69fb9910ee0b0db7b8a01198f03623eab6dadf1eba01f9dbc20757700530009436861696e54797065001253494e474c455f50524f4f465f434841494e000448617368002c4a74446f413157476a784f63584a67496365574e6336396a5551524867506835414e337848646b6a7158303d48796742426b32317a79384c576e5a56785a48526c38706b502f366e34377546317664324a4378654238593d"),
	Hash: func() *Bytes32 {
		b := NewBytes32(
			"a5e49c1c14762f067b4132c5aa3abf03efdf2569de5d68a3f7cd539577f54942")
		return &b
	}(),
}, {
	Name: "invalid (too short)",
	Data: hexToBytes(
		"009005bb7dd69fb9910ee0b0db7b8a01198f03623eab6dadf1eba01f9dbc207577"),
	Error: "invalid length",
}, {
	Name: "invalid (version byte)",
	Data: hexToBytes(
		"019005bb7dd69fb9910ee0b0db7b8a01198f03623eab6dadf1eba01f9dbc20757700530009436861696e54797065001253494e474c455f50524f4f465f434841494e000448617368002c4a74446f413157476a784f63584a67496365574e6336396a5551524867506835414e337848646b6a7158303d48796742426b32317a79384c576e5a56785a48526c38706b502f366e34377546317664324a4378654238593d"),
	Error: "invalid version byte",
}, {
	Name: "invalid (ext ID Total Size)",
	Data: hexToBytes(
		"009005bb7dd69fb9910ee0b0db7b8a01198f03623eab6dadf1eba01f9dbc20757700010009436861696e54797065001253494e474c455f50524f4f465f434841494e000448617368002c4a74446f413157476a784f63584a67496365574e6336396a5551524867506835414e337848646b6a7158303d48796742426b32317a79384c576e5a56785a48526c38706b502f366e34377546317664324a4378654238593d"),
	Error: "invalid ExtIDs length",
}, {
	Name: "invalid (ext ID Total Size)",
	Data: hexToBytes(
		"009005bb7dd69fb9910ee0b0db7b8a01198f03623eab6dadf1eba01f9dbc207577ffff0009436861696e54797065001253494e474c455f50524f4f465f434841494e000448617368002c4a74446f413157476a784f63584a67496365574e6336396a5551524867506835414e337848646b6a7158303d48796742426b32317a79384c576e5a56785a48526c38706b502f366e34377546317664324a4378654238593d"),
	Error: "invalid ExtIDs length",
}, {
	Name: "invalid (ext ID len)",
	Data: hexToBytes(
		"009005bb7dd69fb9910ee0b0db7b8a01198f03623eab6dadf1eba01f9dbc20757700530008436861696e54797065001253494e474c455f50524f4f465f434841494e000448617368002c4a74446f413157476a784f63584a67496365574e6336396a5551524867506835414e337848646b6a7158303d48796742426b32317a79384c576e5a56785a48526c38706b502f366e34377546317664324a4378654238593d"),
	Error: "error parsing ExtIDs",
}, {
	Name: "invalid (ext ID len)",
	Data: hexToBytes(
		"009005bb7dd69fb9910ee0b0db7b8a01198f03623eab6dadf1eba01f9dbc2075770053000a436861696e54797065001253494e474c455f50524f4f465f434841494e000448617368002c4a74446f413157476a784f63584a67496365574e6336396a5551524867506835414e337848646b6a7158303d48796742426b32317a79384c576e5a56785a48526c38706b502f366e34377546317664324a4378654238593d"),
	Error: "error parsing ExtIDs",
}, {
	Name: "invalid (ext ID len)",
	Data: hexToBytes(
		"009005bb7dd69fb9910ee0b0db7b8a01198f03623eab6dadf1eba01f9dbc20757700530009436861696e54797065001253494e474c455f50524f4f465f434841494e000448617368002b4a74446f413157476a784f63584a67496365574e6336396a5551524867506835414e337848646b6a7158303d48796742426b32317a79384c576e5a56785a48526c38706b502f366e34377546317664324a4378654238593d"),
	Error: "error parsing ExtIDs",
}}

func TestEntry(t *testing.T) {
	for _, test := range unmarshalBinaryTests {
		t.Run("UnmarshalBinary/"+test.Name, func(t *testing.T) {
			require := require.New(t)
			e := Entry{}
			err := e.UnmarshalBinary(test.Data)
			if len(test.Error) == 0 {
				require.NoError(err)
				require.NotNil(e.ChainID)
				return
			}
			require.EqualError(err, test.Error)
		})
	}

	var ecAddressStr = "EC1zANmWuEMYoH6VizJg6uFaEdi8Excn1VbLN99KRuxh3GSvB7YQ"
	ec, _ := NewECAddress(ecAddressStr)
	chainID := ComputeChainID([]Bytes{Bytes(ec[:])})
	t.Run("ComposeCreate", func(t *testing.T) {
		c := NewClient()
		es, err := ec.GetEsAddress(nil, c)
		if _, ok := err.(jsonrpc2.Error); ok {
			// Skip if the EC address is not in the wallet.
			t.SkipNow()
		}
		assert := assert.New(t)
		assert.NoError(err)
		balance, err := ec.GetBalance(nil, c)
		assert.NoError(err)
		if balance == 0 {
			// Skip if the EC address is not funded.
			t.SkipNow()
		}

		randData, err := GenerateEsAddress()
		assert.NoError(err)
		e := Entry{Content: Bytes(randData[:]),
			ExtIDs:  []Bytes{Bytes(ec[:])},
			ChainID: &chainID}
		tx, err := e.ComposeCreate(nil, c, es)
		assert.NoError(err)
		assert.NotNil(tx)
		assert.NotNil(e.Hash)
		fmt.Println("Tx: ", tx)
		fmt.Println("Entry Hash: ", e.Hash)
		fmt.Println("Chain ID: ", e.ChainID)

		e.Hash = nil
		e.ChainID = nil
		e.Content = Bytes(randData[:])
		e.ExtIDs = []Bytes{Bytes(randData[:])}
		tx, err = e.ComposeCreate(nil, c, es)
		assert.NoError(err)
		assert.NotNil(tx)
		assert.NotNil(e.Hash)
		assert.NotNil(e.ChainID)
		fmt.Println("Tx: ", tx)
		fmt.Println("Entry Hash: ", e.Hash)
		fmt.Println("Chain ID: ", e.ChainID)
	})
	t.Run("Create", func(t *testing.T) {
		c := NewClient()
		//c.Factomd.DebugRequest = true
		//c.Walletd.DebugRequest = true
		balance, err := ec.GetBalance(nil, c)
		assert := assert.New(t)
		require := require.New(t)
		require.NoError(err)
		if balance == 0 {
			// Skip if the EC address is not funded.
			t.SkipNow()
		}

		randData, err := GenerateEsAddress()
		assert.NoError(err)
		e := Entry{Content: Bytes(randData[:]),
			ExtIDs:  []Bytes{Bytes(ec[:])},
			ChainID: &chainID}
		tx, err := e.Create(nil, c, ec)
		assert.NoError(err)
		assert.NotNil(tx)
		assert.NotNil(e.Hash)
		fmt.Println("Tx: ", tx)
		fmt.Println("Entry Hash: ", e.Hash)
		fmt.Println("Chain ID: ", e.ChainID)

		e.ChainID = nil
		e.Content = Bytes(randData[:])
		e.ExtIDs = []Bytes{Bytes(randData[:])}
		tx, err = e.Create(nil, c, ec)
		assert.NoError(err)
		assert.NotNil(tx)
		assert.NotNil(e.Hash)
		assert.NotNil(e.ChainID)
		fmt.Println("Tx: ", tx)
		fmt.Println("Entry Hash: ", e.Hash)
		fmt.Println("Chain ID: ", e.ChainID)
	})
	t.Run("Compose/too large", func(t *testing.T) {
		assert := assert.New(t)
		e := Entry{Content: make(Bytes, 11000),
			ExtIDs:  []Bytes{Bytes(ec[:])},
			ChainID: &chainID}
		_, _, _, err := e.Compose(EsAddress(ec))
		assert.EqualError(err, "factom.Entry.MarshalBinary(): length exceeds 10275")
	})
	t.Run("EntryCost", func(t *testing.T) {
		assert := assert.New(t)
		_, err := EntryCost(11000, false)
		assert.EqualError(err, "Entry cannot be larger than 10KB")
		_, err = EntryCost(0, false)
		assert.EqualError(err, "invalid size")
		cost, err := EntryCost(1200, false)
		require.NoError(t, err)
		assert.Equal(uint8(2), cost)
	})
}

func hexToBytes(hexStr string) Bytes {
	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return Bytes(raw)
}
