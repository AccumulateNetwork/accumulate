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

package fat1_test

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Factom-Asset-Tokens/factom"
	"github.com/Factom-Asset-Tokens/factom/fat"
	. "github.com/Factom-Asset-Tokens/factom/fat1"
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
		out[outputAddresses[0].FAAddress().String()].
			Append(out[outputAddresses[1].FAAddress().String()])
		out[outputAddresses[0].FAAddress().String()].
			Append(out[outputAddresses[2].FAAddress().String()])
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
	Name:  "invalid JSON (unknown field)",
	Error: `*fat1.Transaction: unexpected JSON length`,
	Tx:    setFieldTransaction("invalid", 5),
}, {
	Name:  "invalid JSON (invalid inputs type)",
	Error: "*fat1.Transaction.Inputs: *fat1.AddressNFTokensMap: json: cannot unmarshal array into Go value of type map[string]json.RawMessage",
	Tx:    invalidField("inputs"),
}, {
	Name:  "invalid JSON (invalid outputs type)",
	Error: "*fat1.Transaction.Outputs: *fat1.AddressNFTokensMap: json: cannot unmarshal array into Go value of type map[string]json.RawMessage",
	Tx:    invalidField("outputs"),
}, {
	Name:  "invalid JSON (invalid inputs, duplicate address)",
	Error: "*fat1.Transaction.Inputs: *fat1.AddressNFTokensMap: unexpected JSON length",
	Tx:    transaction([]byte(`{"inputs":{"FA2HaNAq1f85f1cxzywDa7etvtYCGZUztERvExzQik3CJrGBM4sx":[0],"FA2HaNAq1f85f1cxzywDa7etvtYCGZUztERvExzQik3CJrGBM4sx":[1],"FA3rCRnpU95ieYCwh7YGH99YUWPjdVEjk73mpjqnVpTDt3rUUhX8":[2]},"metadata":[0],"outputs":{"FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC":[1],"FA2PJRLbuVDyAKire9BRnJYkh2NZc2Fjco4FCrPtXued7F26wGBP":[0],"FA2uyZviB3vs28VkqkfnhoXRD8XdKP1zaq7iukq2gBfCq3hxeuE8":[2]}}`)),
}, {
	Name:  "invalid JSON (invalid inputs, duplicate ids)",
	Error: "*fat1.Transaction.Inputs: *fat1.AddressNFTokensMap: duplicate NFTokenID: 0: ",
	Tx:    transaction([]byte(`{"inputs":{"FA2HaNAq1f85f1cxzywDa7etvtYCGZUztERvExzQik3CJrGBM4sx":[0],"FA3rCRnpU95ieYCwh7YGH99YUWPjdVEjk73mpjqnVpTDt3rUUhX8":[0,1,2]},"metadata":[0],"outputs":{"FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC":[1],"FA2PJRLbuVDyAKire9BRnJYkh2NZc2Fjco4FCrPtXued7F26wGBP":[0],"FA2uyZviB3vs28VkqkfnhoXRD8XdKP1zaq7iukq2gBfCq3hxeuE8":[2]}}`)),
}, {
	Name:  "invalid JSON (two objects)",
	Error: "invalid character '{' after top-level value",
	Tx:    transaction([]byte(`{"inputs":{"FA2HaNAq1f85f1cxzywDa7etvtYCGZUztERvExzQik3CJrGBM4sx":100,"FA3rCRnpU95ieYCwh7YGH99YUWPjdVEjk73mpjqnVpTDt3rUUhX8":10},"metadata":[0],"outputs":{"FA1zT4aFpEvcnPqPCigB3fvGu4Q4mTXY22iiuV69DqE1pNhdF2MC":10,"FA2PJRLbuVDyAKire9BRnJYkh2NZc2Fjco4FCrPtXued7F26wGBP":90,"FA2uyZviB3vs28VkqkfnhoXRD8XdKP1zaq7iukq2gBfCq3hxeuE8":10}}{}`)),
}, {
	Name:  "invalid data (no inputs)",
	Error: "*fat1.Transaction.Inputs: *fat1.AddressNFTokensMap: empty",
	Tx:    setFieldTransaction("inputs", json.RawMessage(`{}`)),
}, {
	Name:  "invalid data (no outputs)",
	Error: "*fat1.Transaction.Outputs: *fat1.AddressNFTokensMap: empty",
	Tx:    setFieldTransaction("outputs", json.RawMessage(`{}`)),
}, {
	Name:  "invalid data (omit inputs)",
	Error: "*fat1.Transaction.Inputs: *fat1.AddressNFTokensMap: unexpected end of JSON input",
	Tx:    omitFieldTransaction("inputs"),
}, {
	Name:  "invalid data (omit outputs)",
	Error: "*fat1.Transaction.Outputs: *fat1.AddressNFTokensMap: unexpected end of JSON input",
	Tx:    omitFieldTransaction("outputs"),
}, {
	Name:  "invalid data (Input Output mismatch)",
	Error: "*fat1.Transaction: Inputs and Outputs mismatch: number of NFTokenIDs differ",
	Tx: func() *Transaction {
		out := outputs()
		NFTokenID(1000).Set(out[outputAddresses[0].FAAddress().String()])
		return setFieldTransaction("outputs", out)
	}(),
}, {
	Name:  "invalid data (Input Output mismatch)",
	Error: "*fat1.Transaction: Inputs and Outputs mismatch: missing NFTokenID: 1000",
	Tx: func() *Transaction {
		in := inputs()
		NFTokenID(1001).Set(in[inputAddresses[0].FAAddress().String()])
		out := outputs()
		NFTokenID(1000).Set(out[outputAddresses[0].FAAddress().String()])
		m := validTxEntryContentMap()
		m["inputs"] = in
		m["outputs"] = out
		return transaction(marshal(m))
	}(),
}, {
	Name:      "invalid data (coinbase)",
	Error:     "*fat1.Transaction: invalid coinbase transaction",
	IssuerKey: issuerKey,
	Tx: func() *Transaction {
		m := validCoinbaseTxEntryContentMap()
		in := coinbaseInputs()
		in[inputAddresses[0].FAAddress().String()] = newNFTokens(NFTokenID(1000))
		out := coinbaseOutputs()
		out[outputAddresses[0].FAAddress().String()] = newNFTokens(NFTokenID(1000))
		m["inputs"] = in
		m["outputs"] = out
		return transaction(marshal(m))
	}(),
}, {
	Name:      "invalid data (coinbase, coinbase outputs)",
	Error:     "*fat1.Transaction: Inputs and Outputs intersect: duplicate address: ",
	IssuerKey: issuerKey,
	Tx: func() *Transaction {
		m := validCoinbaseTxEntryContentMap()
		in := coinbaseInputs()
		out := coinbaseOutputs()
		in[fat.Coinbase().String()] = newNFTokens(NFTokenID(1000))
		out[fat.Coinbase().String()] = newNFTokens(NFTokenID(1000))
		m["inputs"] = in
		m["outputs"] = out
		delete(m, "tokenmetadata")
		return transaction(marshal(m))
	}(),
}, {
	Name:      "invalid data (coinbase, tokenmetadata)",
	Error:     "*fat1.Transaction.TokenMetadata: too many NFTokenIDs",
	IssuerKey: issuerKey,
	Tx: func() *Transaction {
		m := validCoinbaseTxEntryContentMap()
		in := coinbaseInputs()
		delete(in[fat.Coinbase().String()], NFTokenID(0))
		out := coinbaseOutputs()
		delete(out[coinbaseOutputAddresses[0].FAAddress().String()], NFTokenID(0))

		m["inputs"] = in
		m["outputs"] = out
		return transaction(marshal(m))
	}(),
}, {
	Name:  "invalid data (inputs outputs overlap)",
	Error: "*fat1.Transaction: Inputs and Outputs intersect: duplicate address: ",
	Tx: func() *Transaction {
		m := validTxEntryContentMap()
		in := inputs()
		in[outputAddresses[0].FAAddress().String()] =
			in[inputAddresses[0].FAAddress().String()]
		delete(in, inputAddresses[0].FAAddress().String())
		m["inputs"] = in
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
			require.NoError(t, err, string(test.Tx.Content))
			if test.Coinbase {
				assert.True(tx.IsCoinbase(), string(test.Tx.Content))
			}
		})
	}
}

var (
	inputAddresses  = twoAddresses()
	outputAddresses = append(twoAddresses(), factom.FsAddress{})

	inputNFTokens = []NFTokens{newNFTokens(NewNFTokenIDRange(0, 10)),
		newNFTokens(NFTokenID(11))}
	outputNFTokens = []NFTokens{newNFTokens(NewNFTokenIDRange(0, 5)),
		newNFTokens(NewNFTokenIDRange(6, 10)),
		newNFTokens(NFTokenID(11))}

	coinbaseInputAddresses  = []factom.FAAddress{fat.Coinbase()}
	coinbaseOutputAddresses = twoAddresses()

	coinbaseInputNFTokens  = []NFTokens{newNFTokens(NewNFTokenIDRange(0, 11))}
	coinbaseOutputNFTokens = []NFTokens{newNFTokens(NewNFTokenIDRange(0, 5)),
		newNFTokens(NewNFTokenIDRange(6, 11))}

	identityChainID = factom.NewBytes32(validIdentityChainID())
	tokenChainID    = fat.ComputeChainID("test", identityChainID)
)

func newNFTokens(ids ...NFTokensSetter) NFTokens {
	nfTkns, err := NewNFTokens(ids...)
	if err != nil {
		panic(err)
	}
	return nfTkns
}

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
		"inputs":        coinbaseInputs(),
		"outputs":       coinbaseOutputs(),
		"metadata":      []int{0},
		"tokenmetadata": tokenMetadata(),
	}
}

// inputs/outputs
func inputs() map[string]NFTokens {
	inputs := make(map[string]NFTokens)
	for i := range inputAddresses {
		tkns := newNFTokens()
		tkns.Append(inputNFTokens[i])
		inputs[inputAddresses[i].FAAddress().String()] = tkns
	}
	return inputs
}
func outputs() map[string]NFTokens {
	outputs := make(map[string]NFTokens)
	for i := range outputAddresses {
		tkns := newNFTokens()
		tkns.Append(outputNFTokens[i])
		outputs[outputAddresses[i].FAAddress().String()] = tkns
	}
	return outputs
}
func coinbaseInputs() map[string]NFTokens {
	inputs := make(map[string]NFTokens)
	for i := range coinbaseInputAddresses {
		tkns := newNFTokens()
		tkns.Append(coinbaseInputNFTokens[i])
		inputs[coinbaseInputAddresses[i].String()] = tkns
	}
	return inputs
}
func coinbaseOutputs() map[string]NFTokens {
	outputs := make(map[string]NFTokens)
	for i := range coinbaseOutputAddresses {
		tkns := newNFTokens()
		tkns.Append(coinbaseOutputNFTokens[i])
		outputs[coinbaseOutputAddresses[i].FAAddress().String()] = tkns
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
		t.Inputs[fat.Coinbase()], _ = NewNFTokens()
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
	Error: "json: error calling MarshalJSON for type *fat1.Transaction: Inputs and Outputs mismatch: number of NFTokenIDs differ",
	Tx: func() *Transaction {
		t := newTransaction()
		t.Inputs[inputAddresses[0].FAAddress()].Set(NFTokenID(12345))
		return t
	}(),
}, {
	Name:  "invalid metadata JSON",
	Error: "json: error calling MarshalJSON for type *fat1.Transaction: json: error calling MarshalJSON for type json.RawMessage: invalid character 'a' looking for beginning of object key string",
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
				assert.NoError(err, string(test.Tx.Content))
			} else {
				assert.EqualError(err, test.Error, string(test.Tx.Content))
			}
		})
	}
}

func newTransaction() *Transaction {
	return &Transaction{
		Inputs:  inputAddressNFTokensMap(),
		Outputs: outputAddressNFTokensMap(),
	}
}
func inputAddressNFTokensMap() AddressNFTokensMap {
	return addressNFTokensMap(inputs())
}
func outputAddressNFTokensMap() AddressNFTokensMap {
	return addressNFTokensMap(outputs())
}
func addressNFTokensMap(aas map[string]NFTokens) AddressNFTokensMap {
	m := make(AddressNFTokensMap)
	for adrStr, amount := range aas {
		var a factom.FAAddress
		if err := a.Set(adrStr); err != nil {
			panic(err.Error() + " " + adrStr)
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

func tokenMetadata() NFTokenIDMetadataMap {
	m := make(NFTokenIDMetadataMap, len(coinbaseInputNFTokens[0]))
	for i, tkns := range inputNFTokens {
		m.Set(NFTokenMetadata{Tokens: tkns,
			Metadata: json.RawMessage(fmt.Sprintf("%v", i))})
	}
	return m
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
