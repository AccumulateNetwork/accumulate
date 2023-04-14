// Copyright 2022 The Accumulate Authors
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
	merkle "gitlab.com/accumulatenetwork/accumulate/smt/managed"
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
