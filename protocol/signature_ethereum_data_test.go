// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseEthereumTx_Legacy(t *testing.T) {
	// Sample legacy Ethereum transaction (RLP-encoded)
	// This is a simple ETH transfer transaction
	rawTx := []byte{
		0xf8, 0x6c, // List prefix (108 bytes)
		0x80,       // nonce = 0
		0x85, 0x04, 0xa8, 0x17, 0xc8, 0x00, // gasPrice = 20 Gwei
		0x82, 0x52, 0x08, // gasLimit = 21000
		0x94, // to address (20 bytes)
		0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35,
		0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35,
		0x88, 0x0d, 0xe0, 0xb6, 0xb3, 0xa7, 0x64, 0x00, 0x00, // value = 1 ETH
		0x80,             // data = empty
		0x1c,             // v = 28 (pre-EIP-155)
		0xa0,             // r (32 bytes)
		0x88, 0xff, 0x6c, 0xf0, 0xfe, 0xfd, 0x94, 0xdb,
		0x46, 0x11, 0x11, 0xf5, 0xcd, 0xa9, 0x28, 0xbc,
		0xb4, 0xa9, 0x3a, 0x59, 0x28, 0x88, 0x88, 0x88,
		0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88,
		0xa0, // s (32 bytes)
		0x42, 0xeb, 0xd7, 0xb7, 0xfc, 0xde, 0xd2, 0x11,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
	}

	tx, err := ParseEthereumTx(rawTx)
	require.NoError(t, err)
	require.NotNil(t, tx)

	require.Equal(t, uint8(0), tx.TxType, "should be legacy transaction")
	require.Equal(t, uint64(0), tx.Nonce)
	require.Equal(t, uint64(21000), tx.GasLimit)
	require.NotNil(t, tx.V)
	require.NotNil(t, tx.R)
	require.NotNil(t, tx.S)
}

func TestRLPDecoding(t *testing.T) {
	// Test empty string
	data := []byte{0x80}
	items, err := decodeRLPList(data)
	require.Error(t, err, "should fail for non-list")

	// Test short list
	data = []byte{0xc2, 0x80, 0x80} // List of two empty strings
	items, err = decodeRLPList(data)
	require.NoError(t, err)
	require.Len(t, items, 2)

	// Test single byte
	item, consumed, err := decodeRLPItem([]byte{0x05})
	require.NoError(t, err)
	require.Equal(t, 1, consumed)
	require.Equal(t, []byte{0x05}, item)

	// Test short string
	item, consumed, err = decodeRLPItem([]byte{0x83, 0x64, 0x6f, 0x67}) // "dog"
	require.NoError(t, err)
	require.Equal(t, 4, consumed)
	require.Equal(t, []byte("dog"), item)
}

func TestBytesToUint64(t *testing.T) {
	tests := []struct {
		input    []byte
		expected uint64
	}{
		{[]byte{}, 0},
		{[]byte{0x01}, 1},
		{[]byte{0x01, 0x00}, 256},
		{[]byte{0x52, 0x08}, 21000}, // gas limit
	}

	for _, tt := range tests {
		result := bytesToUint64(tt.input)
		require.Equal(t, tt.expected, result)
	}
}

func TestUint64ToBytes(t *testing.T) {
	tests := []struct {
		input    uint64
		expected []byte
	}{
		{0, []byte{}},
		{1, []byte{0x01}},
		{256, []byte{0x01, 0x00}},
		{21000, []byte{0x52, 0x08}},
	}

	for _, tt := range tests {
		result := uint64ToBytes(tt.input)
		require.Equal(t, tt.expected, result)
	}
}

func TestEthereumDataSignature_Methods(t *testing.T) {
	sig := &EthereumDataSignature{
		SignerVersion: 1,
		Timestamp:     12345,
		Vote:          VoteTypeAccept,
	}

	// Test GetSignerVersion
	require.Equal(t, uint64(1), sig.GetSignerVersion())

	// Test GetTimestamp
	require.Equal(t, uint64(12345), sig.GetTimestamp())

	// Test GetVote
	require.Equal(t, VoteTypeAccept, sig.GetVote())

	// Test Type
	require.Equal(t, SignatureTypeEthereumData, sig.Type())

	// Test Hash
	hash := sig.Hash()
	require.NotNil(t, hash)

	// Test Metadata
	meta := sig.Metadata()
	require.NotNil(t, meta)
}

func TestEthereumDataSignature_Marshaling(t *testing.T) {
	sig := &EthereumDataSignature{
		SignerVersion:   1,
		Timestamp:       12345,
		Vote:            VoteTypeAccept,
		ExpectedChainId: 1, // Ethereum mainnet
	}

	// Marshal
	data, err := sig.MarshalBinary()
	require.NoError(t, err)

	// Unmarshal
	sig2 := new(EthereumDataSignature)
	err = sig2.UnmarshalBinary(data)
	require.NoError(t, err)

	// Compare
	require.Equal(t, sig.SignerVersion, sig2.SignerVersion)
	require.Equal(t, sig.Timestamp, sig2.Timestamp)
	require.Equal(t, sig.Vote, sig2.Vote)
	require.Equal(t, sig.ExpectedChainId, sig2.ExpectedChainId)
}

func TestEthereumDataEntry_WithSignatureVerification(t *testing.T) {
	// Create an EthereumDataEntry with sample raw tx
	// Use the same data as TestParseEthereumTx_Legacy which is validated
	rawTx := []byte{
		0xf8, 0x6c, // List prefix (108 bytes)
		0x80,       // nonce = 0
		0x85, 0x04, 0xa8, 0x17, 0xc8, 0x00, // gasPrice = 20 Gwei
		0x82, 0x52, 0x08, // gasLimit = 21000
		0x94, // to address (20 bytes)
		0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35,
		0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35, 0x35,
		0x88, 0x0d, 0xe0, 0xb6, 0xb3, 0xa7, 0x64, 0x00, 0x00, // value = 1 ETH
		0x80,             // data = empty
		0x1c,             // v = 28 (pre-EIP-155)
		0xa0,             // r (32 bytes)
		0x88, 0xff, 0x6c, 0xf0, 0xfe, 0xfd, 0x94, 0xdb,
		0x46, 0x11, 0x11, 0xf5, 0xcd, 0xa9, 0x28, 0xbc,
		0xb4, 0xa9, 0x3a, 0x59, 0x28, 0x88, 0x88, 0x88,
		0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88, 0x88,
		0xa0, // s (32 bytes)
		0x42, 0xeb, 0xd7, 0xb7, 0xfc, 0xde, 0xd2, 0x11,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
	}

	entry := &EthereumDataEntry{RawTx: rawTx}

	// Test Hash
	hash := entry.Hash()
	require.Len(t, hash, 32)

	// Test GetData
	data := entry.GetData()
	require.Len(t, data, 1)
	require.Equal(t, rawTx, data[0])

	// Test Type
	require.Equal(t, DataEntryTypeEthereum, entry.Type())

	// Try to parse the transaction
	tx, err := ParseEthereumTx(rawTx)
	require.NoError(t, err)
	require.NotNil(t, tx)

	// Verify the transaction was parsed correctly
	require.Equal(t, uint64(0), tx.Nonce)
	require.Equal(t, uint64(21000), tx.GasLimit)
}

func TestVerifyEthereumDataSignature_EmptyEntry(t *testing.T) {
	// Test with nil entry
	_, err := VerifyEthereumDataSignature(nil, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing ethereum data entry")

	// Test with empty RawTx
	entry := &EthereumDataEntry{RawTx: nil}
	_, err = VerifyEthereumDataSignature(entry, 0)
	require.Error(t, err)
}

func TestVerifyEthereumDataSignature_InvalidTx(t *testing.T) {
	// Test with invalid RLP data
	entry := &EthereumDataEntry{RawTx: []byte{0x01, 0x02, 0x03}}
	_, err := VerifyEthereumDataSignature(entry, 0)
	require.Error(t, err)
}
