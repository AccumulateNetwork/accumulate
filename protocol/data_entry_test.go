// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"encoding"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/merkle"
)

func TestDataEntry(t *testing.T) {
	de := AccumulateDataEntry{}

	de.Data = append(de.Data, []byte("test data entry"))
	for i := 0; i < 10; i++ {
		de.Data = append(de.Data, []byte(fmt.Sprintf("extid %d", i)))
	}

	expectedHash := "29f613df53d1e38dcfea87b2582985cae5265699ef8fc5c500b0bee8f32974ed"
	entryHash := fmt.Sprintf("%x", de.Hash())
	if entryHash != expectedHash {
		t.Fatalf("expected hash %v, but received %v", expectedHash, entryHash)
	}

	cost, err := DataEntryCost(&de)
	if err != nil {
		t.Fatal(err)
	}
	if cost != FeeData.AsUInt64() {
		t.Fatalf("expected a cost of 10 credits, but computed %d", cost)
	}

	//now make the data entry larger and compute cost
	for i := 0; i < 100; i++ {
		de.Data = append(de.Data, []byte(fmt.Sprintf("extid %d", i)))
	}

	cost, err = DataEntryCost(&de)
	if err != nil {
		t.Fatal(err)
	}

	//the size is now 987 bytes so it should cost 50 credits
	if cost != 5*FeeData.AsUInt64() {
		t.Fatalf("expected a cost of 50 credits, but computed %d", cost)
	}

	//now let's blow up the size of the entry to > 20kB to make sure it fails.
	for i := 0; i < 2000; i++ {
		de.Data = append(de.Data, []byte(fmt.Sprintf("extid %d", i)))
	}

	//now the size of the entry is 20480 bytes, so the cost should fail.
	cost, err = DataEntryCost(&de)
	if err == nil {
		t.Fatalf("expected failure on data to large, but it passed and returned a cost of %d", cost)
	}
}

func TestDataEntryEmpty(t *testing.T) {
	de := new(AccumulateDataEntry)
	de.Data = [][]byte{nil, []byte("foo")}

	marshalled, err := de.MarshalBinary()
	require.NoError(t, err)

	de2 := new(AccumulateDataEntry)
	require.NoError(t, de2.UnmarshalBinary(marshalled))
	require.True(t, de.Equal(de2))
}

func TestDoubleHashEntryProof(t *testing.T) {
	relaxed := &merkle.ValidateOptions{Relaxed: true}

	hash := doSha256([]byte("foo"))
	entry := new(DoubleHashDataEntry)
	entry.Data = [][]byte{append(doSha256(nil), hash...)}

	txn := new(Transaction)
	txn.Header.Principal = AccountUrl("foo", "data")
	txn.Body = &WriteData{Entry: entry}

	// Bad receipt without the double hash entry
	receipt := new(merkle.Receipt)
	receipt.Start = hash
	receipt.Entries = []*merkle.ReceiptEntry{
		{Hash: doSha256(nil), Right: false},
		{Hash: marshalHash(&WriteData{}), Right: false},
		{Hash: marshalHash(&txn.Header), Right: false},
	}
	receipt.Anchor = txn.GetHash()
	assert.False(t, receipt.Validate(nil), "Proof is invalid (strict)")
	assert.False(t, receipt.Validate(relaxed), "Proof is invalid (relaxed)")

	// Good receipt with the double hash entry
	receipt = new(merkle.Receipt)
	receipt.Start = hash
	receipt.Entries = []*merkle.ReceiptEntry{
		{Hash: doSha256(nil), Right: false},
		{Hash: nil, Right: false}, // Double hash
		{Hash: marshalHash(&WriteData{}), Right: false},
		{Hash: marshalHash(&txn.Header), Right: false},
	}
	receipt.Anchor = txn.GetHash()
	assert.False(t, receipt.Validate(nil), "Strict mode does not allow double hashes")
	assert.True(t, receipt.Validate(relaxed), "Relaxed mode allows double hashes")
}

func TestAppendZeros(t *testing.T) {
	b, err := new(BurnTokens).MarshalBinary()
	require.NoError(t, err)
	b = append(b, 2)
	c := make([]byte, 64-len(b))
	c[0] = byte(len(c) - 1)
	for i := range c[1:] {
		c[i+1] = byte(i) + 1
	}
	b = append(b, c...)

	// Tack a zero on the end
	b = append(b, 0)

	body := new(BurnTokens)
	require.NoError(t, body.UnmarshalBinary(b))
	b, err = body.MarshalBinary()
	require.NoError(t, err)
	require.Len(t, b, 65) // Sanity check

	// Marshal and unmarshal
	b, err = (&Transaction{Body: body}).MarshalBinary()
	require.NoError(t, err)
	txn := new(Transaction)
	require.NoError(t, txn.UnmarshalBinary(b))

	// Verify the body is still 65 bytes
	b, err = txn.Body.MarshalBinary()
	require.NoError(t, err)
	require.Len(t, b, 65) // Sanity check
}

func marshalHash(v encoding.BinaryMarshaler) []byte {
	b, err := v.MarshalBinary()
	if err != nil {
		panic(err)
	}
	return doSha256(b)
}

func TestEthereumDataEntry(t *testing.T) {
	// Sample raw Ethereum transaction bytes (RLP-encoded, including signature)
	// This is a simple transfer transaction for testing
	rawTx := []byte{
		0xf8, 0x6c, 0x80, 0x85, 0x04, 0xa8, 0x17, 0xc8, 0x00,
		0x82, 0x52, 0x08, 0x94, 0x35, 0x35, 0x35, 0x35, 0x35,
		0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35,
		0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x88, 0x0d, 0xe0,
		0xb6, 0xb3, 0xa7, 0x64, 0x00, 0x00, 0x80, 0x1c, 0xa0,
		0x88, 0xff, 0x6c, 0xf0, 0xfe, 0xfd, 0x94, 0xdb, 0x46,
		0x11, 0x11, 0xf5, 0xcd, 0xa9, 0x28, 0xbc, 0xb4, 0xa9,
		0x3a, 0x59, 0x28, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88,
		0x88, 0x88, 0x88, 0x88, 0x88, 0xa0, 0x42, 0xeb, 0xd7,
		0xb7, 0xfc, 0xde, 0xd2, 0x11, 0x11, 0x11, 0x11, 0x11,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
	}

	entry := &EthereumDataEntry{RawTx: rawTx}

	// Test Hash() returns Keccak256 hash
	hash := entry.Hash()
	require.Len(t, hash, 32, "Hash should be 32 bytes (Keccak256)")

	// Test GetData() returns the raw transaction bytes
	data := entry.GetData()
	require.Len(t, data, 1, "GetData should return single-element slice")
	require.Equal(t, rawTx, data[0], "GetData should return the raw transaction bytes")

	// Test Type() returns correct type
	require.Equal(t, DataEntryTypeEthereum, entry.Type())

	// Test marshaling/unmarshaling
	marshalled, err := entry.MarshalBinary()
	require.NoError(t, err)

	entry2 := new(EthereumDataEntry)
	require.NoError(t, entry2.UnmarshalBinary(marshalled))
	require.True(t, entry.Equal(entry2), "Unmarshaled entry should equal original")
}

func TestEthereumDataEntryCost(t *testing.T) {
	// Small transaction
	entry := &EthereumDataEntry{RawTx: make([]byte, 100)}
	cost, err := DataEntryCost(entry)
	require.NoError(t, err)
	require.Equal(t, FeeData.AsUInt64(), cost, "Cost for small entry should be 1 unit")

	// Larger transaction (300 bytes)
	entry = &EthereumDataEntry{RawTx: make([]byte, 300)}
	cost, err = DataEntryCost(entry)
	require.NoError(t, err)
	require.Equal(t, 2*FeeData.AsUInt64(), cost, "Cost for 300 byte entry should be 2 units")
}

func TestEthereumDataEntryEmpty(t *testing.T) {
	entry := &EthereumDataEntry{RawTx: nil}

	marshalled, err := entry.MarshalBinary()
	require.NoError(t, err)

	entry2 := new(EthereumDataEntry)
	require.NoError(t, entry2.UnmarshalBinary(marshalled))
	require.True(t, entry.Equal(entry2))
}

func TestEthereumDataEntryJSON(t *testing.T) {
	rawTx := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	entry := &EthereumDataEntry{RawTx: rawTx}

	// Test JSON marshaling
	jsonBytes, err := entry.MarshalJSON()
	require.NoError(t, err)
	require.Contains(t, string(jsonBytes), "ethereum")

	// Test JSON unmarshaling
	entry2 := new(EthereumDataEntry)
	require.NoError(t, entry2.UnmarshalJSON(jsonBytes))
	require.True(t, entry.Equal(entry2))
}
