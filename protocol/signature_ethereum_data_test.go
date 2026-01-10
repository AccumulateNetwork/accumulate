// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package protocol

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
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
	_, err := decodeRLPList(data)
	require.Error(t, err, "should fail for non-list")

	// Test short list
	data = []byte{0xc2, 0x80, 0x80} // List of two empty strings
	items, err := decodeRLPList(data)
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

// Test vectors from official Ethereum test suites and ethereumj
// Source: https://github.com/ethereumj/ethereumj/blob/master/ethereumj-core/src/test/java/org/ethereum/core/TransactionTest.java
// Source: https://github.com/ethereum/tests/wiki/Transaction-Tests

func TestEthereumTestVectors_EthereumJ(t *testing.T) {
	// Test vector from ethereumj
	// Private key: sha3("cow") = c85ef7d79691fe79573b1a7064c19c1a9819ebdbd1faaab1a8ec92344438aaf4
	// Expected sender: cd2a3d9f938e13cd947ec05abc7fe734df8dd826
	// Transaction hash: 328ea6d24659dec48adea1aced9a136e5ebdf40258db30d1b1d97ed2b74be34e
	rawTxHex := "f86b8085e8d4a510008227109413978aee95f38490e9769c39b2773ed763d9cd5f872386f26fc10000801ba0eab47c1a49bf2fe5d40e01d313900e19ca485867d462fe06e139e3a536c6d4f4a014a569d327dcda4b29f74f93c0e9729d2f49ad726e703f9cd90dbb0fbf6649f1"
	expectedSender := "cd2a3d9f938e13cd947ec05abc7fe734df8dd826"

	rawTx := hexToBytes(t, rawTxHex)
	entry := &EthereumDataEntry{RawTx: rawTx}

	// Parse the transaction first
	tx, err := ParseEthereumTx(rawTx)
	require.NoError(t, err, "should parse ethereumj test vector")

	// Verify transaction fields
	require.Equal(t, uint8(0), tx.TxType, "should be legacy transaction")
	require.Equal(t, uint64(0), tx.Nonce)
	require.Equal(t, uint64(10000), tx.GasLimit) // 0x2710 = 10000

	// Verify signature recovery
	signerUrl, err := VerifyEthereumDataSignature(entry, 0)
	require.NoError(t, err, "should verify ethereumj test vector signature")
	require.NotNil(t, signerUrl)

	// Extract the Ethereum address from the lite account URL
	liteKey, _, err := ParseLiteTokenAddress(signerUrl)
	require.NoError(t, err)
	require.NotNil(t, liteKey)

	// Compare recovered address with expected sender
	recoveredAddr := bytesToHex(liteKey)
	require.Equal(t, expectedSender, recoveredAddr, "recovered sender should match expected")
}

func TestEthereumTestVectors_Etherscan(t *testing.T) {
	// Real mainnet transaction from Etherscan
	// https://etherscan.io/getRawTx?tx=0xb9d4ad5408f53eac8627f9ccd840ba8fb3469d55cd9cc2a11c6e049f1eef4edd
	rawTxHex := "f86c0a85046c7cfe0083016dea94d1310c1e038bc12865d3d3997275b3e4737c6302880b503be34d9fe80080269fc7eaaa9c21f59adf8ad43ed66cf5ef9ee1c317bd4d32cd65401e7aaca47cfaa0387d79c65b90be6260d09dcfb780f29dd8133b9b1ceb20b83b7e442b4bfc30cb"

	rawTx := hexToBytes(t, rawTxHex)
	entry := &EthereumDataEntry{RawTx: rawTx}

	// Parse the transaction
	tx, err := ParseEthereumTx(rawTx)
	require.NoError(t, err, "should parse Etherscan test vector")

	// Verify it's a legacy transaction with chain ID (EIP-155)
	require.Equal(t, uint8(0), tx.TxType, "should be legacy transaction")
	require.Equal(t, uint64(10), tx.Nonce) // 0x0a = 10

	// This transaction has chain ID encoded in v (mainnet = 1)
	// v = chainId * 2 + 35 + recovery_id, so v = 37 or 38 for mainnet
	// The v value 0x26 = 38 indicates chain ID 1 (mainnet) with recovery_id 1

	// Verify signature recovery (use chain ID 1 for mainnet)
	signerUrl, err := VerifyEthereumDataSignature(entry, 1)
	require.NoError(t, err, "should verify Etherscan test vector signature")
	require.NotNil(t, signerUrl)
}

func TestEthereumTestVectors_ethereum_org(t *testing.T) {
	// Test vector from ethereum.org documentation
	// https://ethereum.org/developers/docs/transactions/
	rawTxHex := "f88380018203339407a565b7ed7d7a678680a4c162885bedbb695fe080a44401a6e4000000000000000000000000000000000000000000000000000000000000001226a0223a7c9bcf5531c99be5ea7082183816eb20cfe0bbc322e97cc5c7f71ab8b20ea02aadee6b34b45bb15bc42d9c09de4a6754e7000908da72d48cc7704971491663"

	rawTx := hexToBytes(t, rawTxHex)
	entry := &EthereumDataEntry{RawTx: rawTx}

	// Parse the transaction
	tx, err := ParseEthereumTx(rawTx)
	require.NoError(t, err, "should parse ethereum.org test vector")

	// Verify transaction fields
	require.Equal(t, uint8(0), tx.TxType, "should be legacy transaction")
	require.Equal(t, uint64(0), tx.Nonce)
	require.Equal(t, uint64(819), tx.GasLimit) // 0x0333 = 819

	// Verify signature recovery (v=0x26=38 indicates mainnet with recovery_id=1)
	signerUrl, err := VerifyEthereumDataSignature(entry, 1)
	require.NoError(t, err, "should verify ethereum.org test vector signature")
	require.NotNil(t, signerUrl)
}

func TestEthereumTestVectors_PreEIP155(t *testing.T) {
	// Pre-EIP-155 transaction (v = 27 or 28, no chain ID)
	// This tests legacy transaction format before replay protection
	//
	// Transaction details:
	// nonce: 0
	// gasPrice: 20 Gwei (0x04a817c800)
	// gasLimit: 21000 (0x5208)
	// to: 0x3535353535353535353535353535353535353535
	// value: 1 ETH (0x0de0b6b3a7640000)
	// data: empty
	// v: 28 (0x1c) - pre-EIP-155
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

	// Parse the transaction
	tx, err := ParseEthereumTx(rawTx)
	require.NoError(t, err, "should parse pre-EIP-155 test vector")

	// Verify transaction fields
	require.Equal(t, uint8(0), tx.TxType, "should be legacy transaction")
	require.Equal(t, uint64(0), tx.Nonce)
	require.Equal(t, uint64(21000), tx.GasLimit)

	// Pre-EIP-155 transactions don't have chain ID, so v should be 27 or 28
	require.True(t, tx.V.Int64() == 27 || tx.V.Int64() == 28, "v should be 27 or 28 for pre-EIP-155")

	// Verify signature recovery with no chain ID requirement
	signerUrl, err := VerifyEthereumDataSignature(entry, 0)
	require.NoError(t, err, "should verify pre-EIP-155 test vector signature")
	require.NotNil(t, signerUrl)
}

func TestEthereumDataEntry_Hash(t *testing.T) {
	// Test that Hash() returns Keccak256 of the full signed raw transaction
	// Note: The ethereumj HASH_TX (328ea6d2...) is the hash of the UNSIGNED transaction,
	// not the signed transaction. Our EthereumDataEntry stores the full signed transaction
	// and hashes all of it for replay protection.

	// Signed transaction from ethereumj test
	rawSignedTxHex := "f86b8085e8d4a510008227109413978aee95f38490e9769c39b2773ed763d9cd5f872386f26fc10000801ba0eab47c1a49bf2fe5d40e01d313900e19ca485867d462fe06e139e3a536c6d4f4a014a569d327dcda4b29f74f93c0e9729d2f49ad726e703f9cd90dbb0fbf6649f1"

	// Expected hash of the full SIGNED transaction (not the unsigned one)
	// This is computed as keccak256(rawSignedTx)
	expectedHashHex := "5d3466b457f3480945474de8e2df3c01ceaa55a12d0347d2e17a3f3444651f86"

	rawTx := hexToBytes(t, rawSignedTxHex)
	entry := &EthereumDataEntry{RawTx: rawTx}

	hash := entry.Hash()
	require.Len(t, hash, 32)

	actualHashHex := bytesToHex(hash)
	require.Equal(t, expectedHashHex, actualHashHex, "hash should match Keccak256 of full signed transaction")
}

func TestEthereumDataEntry_UnsignedTxHash(t *testing.T) {
	// Verify we can compute the Ethereum-standard unsigned transaction hash
	// which is used for transaction identification on the Ethereum network.
	// This is different from EthereumDataEntry.Hash() which hashes the full signed tx.

	// Unsigned transaction from ethereumj (RLP_ENCODED_RAW_TX)
	rawUnsignedTxHex := "e88085e8d4a510008227109413978aee95f38490e9769c39b2773ed763d9cd5f872386f26fc1000080"
	// Expected hash from ethereumj HASH_TX
	expectedHashHex := "328ea6d24659dec48adea1aced9a136e5ebdf40258db30d1b1d97ed2b74be34e"

	rawTx := hexToBytes(t, rawUnsignedTxHex)

	// Compute keccak256 hash
	h := sha3.NewLegacyKeccak256()
	h.Write(rawTx)
	hash := h.Sum(nil)

	require.Len(t, hash, 32)
	actualHashHex := bytesToHex(hash)
	require.Equal(t, expectedHashHex, actualHashHex, "unsigned tx hash should match ethereumj HASH_TX")
}

// Helper functions for test vectors

func hexToBytes(t *testing.T, hex string) []byte {
	t.Helper()
	// Remove 0x prefix if present
	if len(hex) >= 2 && hex[:2] == "0x" {
		hex = hex[2:]
	}
	if len(hex)%2 != 0 {
		t.Fatalf("invalid hex string length: %d", len(hex))
	}
	result := make([]byte, len(hex)/2)
	for i := 0; i < len(hex); i += 2 {
		var b byte
		for j := 0; j < 2; j++ {
			c := hex[i+j]
			switch {
			case c >= '0' && c <= '9':
				b = b*16 + (c - '0')
			case c >= 'a' && c <= 'f':
				b = b*16 + (c - 'a' + 10)
			case c >= 'A' && c <= 'F':
				b = b*16 + (c - 'A' + 10)
			default:
				t.Fatalf("invalid hex character: %c", c)
			}
		}
		result[i/2] = b
	}
	return result
}

func bytesToHex(b []byte) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, len(b)*2)
	for i, v := range b {
		result[i*2] = hexChars[v>>4]
		result[i*2+1] = hexChars[v&0x0f]
	}
	return string(result)
}
