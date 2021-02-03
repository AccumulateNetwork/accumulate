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

package fat0_test

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AccumulateNetwork/accumulated/factom"
	"github.com/AccumulateNetwork/accumulated/factom/fat"
	. "github.com/AccumulateNetwork/accumulated/factom/fat0"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var transactionTests = []struct {
	Name      string
	Error     string
	IssuerKey factom.ID1Key
	Coinbase  bool
	Tx        *Transaction
}{{
	Name: "valid",
	Tx:   validTx(),
}, {
	Name: "valid (single outputs)",
	Tx: func() *Transaction {
		out := outputs()
		out[outputAddresses[0].FAAddress().String()] +=
			out[outputAddresses[1].FAAddress().String()] +
				out[outputAddresses[2].FAAddress().String()]
		delete(out, outputAddresses[1].FAAddress().String())
		delete(out, outputAddresses[2].FAAddress().String())
		return setFieldTransaction("outputs", out)
	}(),
}, {
	Name:      "valid (coinbase)",
	IssuerKey: issuerKey,
	Tx:        coinbaseTx(),
}, {
	Name: "valid (omit metadata)",
	Tx:   omitFieldTransaction("metadata"),
}, {
	Name:  "invalid JSON (nil)",
	Error: "unexpected end of JSON input",
	Tx:    transaction(nil),
}, {
	Name:  "invalid JSON",
	Error: `invalid character '}' after object key`,
	Tx:    transaction([]byte(`{"asdfie"}`)),
}, {
	Name:  "invalid JSON (unknown field)",
	Error: `*fat0.Transaction: unexpected JSON length`,
	Tx:    setFieldTransaction("invalid", 5),
}, {
	Name:  "invalid JSON (invalid inputs type)",
	Error: "*fat0.Transaction.Inputs: json: cannot unmarshal array into Go value of type map[string]uint64",
	Tx:    invalidField("inputs"),
}, {
	Name:  "invalid JSON (invalid outputs type)",
	Error: "*fat0.Transaction.Outputs: json: cannot unmarshal array into Go value of type map[string]uint64",
	Tx:    invalidField("outputs"),
}, {
	Name:  "invalid JSON (invalid inputs, duplicate)",
	Error: "*fat0.Transaction.Inputs: *fat0.AddressAmountMap: unexpected JSON length",
	Tx:    transaction([]byte(`{"inputs":{"FA3tM2R3T2ZT2gPrTfxjqhnFsdiqQUyKboKxvka3z5c1JF9yQck5":100,"FA3tM2R3T2ZT2gPrTfxjqhnFsdiqQUyKboKxvka3z5c1JF9yQck5":100,"FA3rCRnpU95ieYCwh7YGH99YUWPjdVEjk73mpjqnVpTDt3rUUhX8":10},"metadata":[0],"outputs":{"FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC":10,"FA3sjgNF4hrJAiD9tQxAVjWS9Ca1hMqyxtuVSZTBqJiPwD7bnHkn":90,"FA2uyZviB3vs28VkqkfnhoXRD8XdKP1zaq7iukq2gBfCq3hxeuE8":10}}`)),
}, {
	Name:  "invalid JSON (two objects)",
	Error: "invalid character '{' after top-level value",
	Tx:    transaction([]byte(`{"inputs":{"FA2HaNAq1f85f1cxzywDa7etvtYCGZUztERvExzQik3CJrGBM4sx":100,"FA3rCRnpU95ieYCwh7YGH99YUWPjdVEjk73mpjqnVpTDt3rUUhX8":10},"metadata":[0],"outputs":{"FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC":10,"FA3sjgNF4hrJAiD9tQxAVjWS9Ca1hMqyxtuVSZTBqJiPwD7bnHkn":90,"FA2uyZviB3vs28VkqkfnhoXRD8XdKP1zaq7iukq2gBfCq3hxeuE8":10}}{}`)),
}, {
	Name:  "invalid address",
	Error: "*fat0.Transaction.Inputs: *fat0.AddressAmountMap: invalid prefix",
	Tx:    transaction([]byte(`{"inputs":{"Fs2HaNAq1f85f1cxzywDa7etvtYCGZUztERvExzQik3CJrGBM4sx":100,"FA3rCRnpU95ieYCwh7YGH99YUWPjdVEjk73mpjqnVpTDt3rUUhX8":10},"metadata":[0],"outputs":{"FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC":10,"FA3sjgNF4hrJAiD9tQxAVjWS9Ca1hMqyxtuVSZTBqJiPwD7bnHkn":90,"FA2uyZviB3vs28VkqkfnhoXRD8XdKP1zaq7iukq2gBfCq3hxeuE8":10}}`)),
}, {
	Name:  "invalid data (no inputs)",
	Error: "*fat0.Transaction.Inputs: *fat0.AddressAmountMap: empty",
	Tx:    setFieldTransaction("inputs", json.RawMessage(`{}`)),
}, {
	Name:  "invalid data (no inputs)",
	Error: "*fat0.Transaction.Inputs: *fat0.AddressAmountMap: empty",
	Tx:    setFieldTransaction("inputs", json.RawMessage(`null`)),
}, {
	Name:  "invalid data (no outputs)",
	Error: "*fat0.Transaction.Outputs: *fat0.AddressAmountMap: empty",
	Tx:    setFieldTransaction("outputs", json.RawMessage(`{}`)),
}, {
	Name:  "invalid data (omit inputs)",
	Error: "*fat0.Transaction.Inputs: unexpected end of JSON input",
	Tx:    omitFieldTransaction("inputs"),
}, {
	Name:  "invalid data (omit outputs)",
	Error: "*fat0.Transaction.Outputs: unexpected end of JSON input",
	Tx:    omitFieldTransaction("outputs"),
}, {
	Name:  "invalid data (sum mismatch)",
	Error: "*fat0.Transaction: sum(inputs) != sum(outputs)",
	Tx: func() *Transaction {
		out := outputs()
		out[outputAddresses[0].FAAddress().String()]++
		return setFieldTransaction("outputs", out)
	}(),
}, {
	Name:      "invalid data (coinbase)",
	Error:     "*fat0.Transaction: invalid coinbase transaction",
	IssuerKey: issuerKey,
	Tx: func() *Transaction {
		m := validCoinbaseTxEntryContentMap()
		in := coinbaseInputs()
		in[inputAddresses[0].FAAddress().String()] = 1
		out := coinbaseOutputs()
		out[outputAddresses[0].FAAddress().String()]++
		m["inputs"] = in
		m["outputs"] = out
		return transaction(marshal(m))
	}(),
}, {
	Name:  "invalid ExtIDs (timestamp)",
	Error: "timestamp salt expired",
	Tx: func() *Transaction {
		t := validTx()
		t.ExtIDs[0] = factom.Bytes("100")
		return t
	}(),
}, {
	Name:  "invalid ExtIDs (length)",
	Error: "invalid number of ExtIDs",
	Tx: func() *Transaction {
		t := validTx()
		t.ExtIDs = append(t.ExtIDs, factom.Bytes{})
		return t
	}(),
}, {
	Name:  "invalid coinbase issuer key",
	Error: "invalid RCD",
	Tx:    coinbaseTx(),
}, {
	Name:  "RCD input mismatch",
	Error: "invalid RCDs",
	Tx: func() *Transaction {
		t := validTx()
		adrs := twoAddresses()
		t.Sign(adrs[0], adrs[1])
		return t
	}(),
}}

func TestTransaction(t *testing.T) {
	for _, test := range transactionTests {
		t.Run(test.Name, func(t *testing.T) {
			assert := assert.New(t)
			tx := test.Tx
			key := test.IssuerKey
			err := tx.Validate(&key)
			if len(test.Error) != 0 {
				assert.Contains(err.Error(), test.Error)
				return
			}
			require.NoError(t, err)
			if test.Coinbase {
				assert.True(tx.IsCoinbase())
			}
		})
	}
}

var (
	inputAddresses  = twoAddresses()
	outputAddresses = append(twoAddresses(), factom.FsAddress{})

	inputAmounts  = []uint64{100, 10}
	outputAmounts = []uint64{90, 10, 10}

	coinbaseInputAddresses  = []factom.FsAddress{factom.FsAddress{}}
	coinbaseOutputAddresses = twoAddresses()

	coinbaseInputAmounts  = []uint64{110}
	coinbaseOutputAmounts = []uint64{90, 20}

	tokenChainID = fat.ComputeChainID("test", identityChainID)

	identityChainID = factom.NewBytes32(validIdentityChainID())
)

// Transactions
func omitFieldTransaction(field string) *Transaction {
	m := validTxEntryContentMap()
	delete(m, field)
	return transaction(marshal(m))
}
func setFieldTransaction(field string, value interface{}) *Transaction {
	m := validTxEntryContentMap()
	m[field] = value
	return transaction(marshal(m))
}
func validTx() *Transaction {
	return transaction(marshal(validTxEntryContentMap()))
}
func coinbaseTx() *Transaction {
	t := transaction(marshal(validCoinbaseTxEntryContentMap()))
	t.Sign(issuerSecret)
	return t
}
func transaction(content factom.Bytes) *Transaction {
	e := factom.Entry{
		ChainID: &tokenChainID,
		Content: content,
	}
	t := NewTransaction(e)
	adrs := make([]factom.RCDPrivateKey, len(inputAddresses))
	for i, adr := range inputAddresses {
		adrs[i] = adr
	}
	t.Sign(adrs...)
	return t
}
func invalidField(field string) *Transaction {
	m := validTxEntryContentMap()
	m[field] = []int{0}
	return transaction(marshal(m))
}

// Content maps
func validTxEntryContentMap() map[string]interface{} {
	return map[string]interface{}{
		"inputs":   inputs(),
		"outputs":  outputs(),
		"metadata": []int{0},
	}
}
func validCoinbaseTxEntryContentMap() map[string]interface{} {
	return map[string]interface{}{
		"inputs":   coinbaseInputs(),
		"outputs":  coinbaseOutputs(),
		"metadata": []int{0},
	}
}

// inputs/outputs
func inputs() map[string]uint64 {
	inputs := make(map[string]uint64)
	for i := range inputAddresses {
		inputs[inputAddresses[i].FAAddress().String()] = inputAmounts[i]
	}
	return inputs
}
func outputs() map[string]uint64 {
	outputs := make(map[string]uint64)
	for i := range outputAddresses {
		outputs[outputAddresses[i].FAAddress().String()] = outputAmounts[i]
	}
	return outputs
}
func coinbaseInputs() map[string]uint64 {
	inputs := make(map[string]uint64)
	for i := range coinbaseInputAddresses {
		inputs[coinbaseInputAddresses[i].FAAddress().String()] =
			coinbaseInputAmounts[i]
	}
	return inputs
}
func coinbaseOutputs() map[string]uint64 {
	outputs := make(map[string]uint64)
	for i := range coinbaseOutputAddresses {
		outputs[coinbaseOutputAddresses[i].FAAddress().String()] =
			coinbaseOutputAmounts[i]
	}
	return outputs
}

var transactionMarshalEntryTests = []struct {
	Name  string
	Error string
	Tx    *Transaction
}{{
	Name: "valid",
	Tx:   newTransaction(),
}, {
	Name: "valid (omit zero balances)",
	Tx: func() *Transaction {
		t := newTransaction()
		t.Inputs[fat.Coinbase()] = 0
		return t
	}(),
}, {
	Name: "valid (metadata)",
	Tx: func() *Transaction {
		t := newTransaction()
		t.Metadata = json.RawMessage(`{"memo":"Rent for Dec 2018"}`)
		return t
	}(),
}, {
	Name:  "invalid data",
	Error: "json: error calling MarshalJSON for type *fat0.Transaction: sum(inputs) != sum(outputs)",
	Tx: func() *Transaction {
		t := newTransaction()
		t.Inputs[inputAddresses[0].FAAddress()]++
		return t
	}(),
}, {
	Name:  "invalid data",
	Error: "json: error calling MarshalJSON for type *fat0.Transaction: json: error calling MarshalJSON for type fat0.AddressAmountMap: empty",
	Tx: func() *Transaction {
		t := newTransaction()
		t.Inputs = make(AddressAmountMap)
		t.Outputs = make(AddressAmountMap)
		return t
	}(),
}, {
	Name:  "invalid metadata JSON",
	Error: "json: error calling MarshalJSON for type *fat0.Transaction: json: error calling MarshalJSON for type json.RawMessage: invalid character 'a' looking for beginning of object key string",
	Tx: func() *Transaction {
		t := newTransaction()
		t.Metadata = json.RawMessage("{asdf")
		return t
	}(),
}}

func TestTransactionMarshalEntry(t *testing.T) {
	for _, test := range transactionMarshalEntryTests {
		t.Run(test.Name, func(t *testing.T) {
			assert := assert.New(t)
			tx := test.Tx
			err := tx.MarshalEntry()
			if len(test.Error) == 0 {
				assert.NoError(err)
			} else {
				assert.EqualError(err, test.Error)
			}
		})
	}
}

func newTransaction() *Transaction {
	return &Transaction{
		Inputs:  inputAddressAmountMap(),
		Outputs: outputAddressAmountMap(),
	}
}
func inputAddressAmountMap() AddressAmountMap {
	return addressAmountMap(inputs())
}
func outputAddressAmountMap() AddressAmountMap {
	return addressAmountMap(outputs())
}
func addressAmountMap(aas map[string]uint64) AddressAmountMap {
	m := make(AddressAmountMap)
	for addressStr, amount := range aas {
		var a factom.FAAddress
		if err := json.Unmarshal(
			[]byte(fmt.Sprintf("%#v", addressStr)), &a); err != nil {
			panic(err)
		}
		m[a] = amount
	}
	return m
}

var issuerSecret = func() factom.SK1Key {
	a, _ := factom.GenerateSK1Key()
	return a
}()
var issuerKey = issuerSecret.ID1Key()

func twoAddresses() []factom.FsAddress {
	adrs := make([]factom.FsAddress, 2)
	for i := range adrs {
		adr, err := factom.GenerateFsAddress()
		if err != nil {
			panic(err)
		}
		adrs[i] = adr
	}
	return adrs
}

func marshal(v map[string]interface{}) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

var validIdentityChainIDStr = "88888807e4f3bbb9a2b229645ab6d2f184224190f83e78761674c2362aca4425"

func validIdentityChainID() factom.Bytes {
	return hexToBytes(validIdentityChainIDStr)
}
func hexToBytes(hexStr string) factom.Bytes {
	raw, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return factom.Bytes(raw)
}
