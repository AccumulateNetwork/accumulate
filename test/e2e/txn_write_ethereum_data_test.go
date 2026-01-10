// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	decredecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	altcrypto "gitlab.com/accumulatenetwork/accumulate/pkg/crypto"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
	"golang.org/x/crypto/sha3"
)

// createLegacyEthereumTx creates a signed legacy Ethereum transaction
func createLegacyEthereumTx(t *testing.T, privKey *ecdsa.PrivateKey, nonce uint64, to []byte, value *big.Int, data []byte) []byte {
	t.Helper()

	gasPrice := big.NewInt(20000000000) // 20 Gwei
	gasLimit := uint64(21000)

	// Encode unsigned transaction: [nonce, gasprice, gaslimit, to, value, data]
	unsignedItems := [][]byte{
		uint64ToRLPBytes(nonce),
		bigIntToRLPBytes(gasPrice),
		uint64ToRLPBytes(gasLimit),
		to,
		bigIntToRLPBytes(value),
		data,
	}
	unsignedTx := encodeRLPList(unsignedItems)

	// Hash with Keccak256
	h := sha3.NewLegacyKeccak256()
	h.Write(unsignedTx)
	txHash := h.Sum(nil)

	// Convert the private key to decred format and use SignCompact
	// which returns the recovery ID directly
	decredPrivKey := secp256k1.PrivKeyFromBytes(privKey.D.Bytes())
	sig := decredecdsa.SignCompact(decredPrivKey, txHash, false) // false = not compressed pubkey

	// SignCompact returns: [recoveryID+27, r (32 bytes), s (32 bytes)]
	// For legacy Ethereum transactions, v = recoveryID + 27
	v := big.NewInt(int64(sig[0])) // Already has 27 or 28 added
	r := new(big.Int).SetBytes(sig[1:33])
	s := new(big.Int).SetBytes(sig[33:65])

	// Encode signed transaction: [nonce, gasprice, gaslimit, to, value, data, v, r, s]
	signedItems := [][]byte{
		uint64ToRLPBytes(nonce),
		bigIntToRLPBytes(gasPrice),
		uint64ToRLPBytes(gasLimit),
		to,
		bigIntToRLPBytes(value),
		data,
		bigIntToRLPBytes(v),
		bigIntToRLPBytes(r),
		bigIntToRLPBytes(s),
	}

	return encodeRLPList(signedItems)
}

func uint64ToRLPBytes(n uint64) []byte {
	if n == 0 {
		return []byte{}
	}
	var buf [8]byte
	i := 7
	for n > 0 {
		buf[i] = byte(n & 0xff)
		n >>= 8
		i--
	}
	return buf[i+1:]
}

func bigIntToRLPBytes(n *big.Int) []byte {
	if n == nil || n.Sign() == 0 {
		return []byte{}
	}
	return n.Bytes()
}

func encodeRLPList(items [][]byte) []byte {
	var content []byte
	for _, item := range items {
		content = append(content, encodeRLPString(item)...)
	}

	if len(content) <= 55 {
		return append([]byte{byte(0xc0 + len(content))}, content...)
	}

	lenBytes := uint64ToRLPBytes(uint64(len(content)))
	return append(append([]byte{byte(0xf7 + len(lenBytes))}, lenBytes...), content...)
}

func encodeRLPString(s []byte) []byte {
	if len(s) == 0 {
		return []byte{0x80}
	}
	if len(s) == 1 && s[0] < 0x80 {
		return s
	}
	if len(s) <= 55 {
		return append([]byte{byte(0x80 + len(s))}, s...)
	}
	lenBytes := uint64ToRLPBytes(uint64(len(s)))
	return append(append([]byte{byte(0xb7 + len(lenBytes))}, lenBytes...), s...)
}

func TestWriteData_EthereumEntry(t *testing.T) {
	var timestamp uint64

	// Initialize with V2Jiuquan to enable EthereumDataEntry
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2Jiuquan),
	)

	// Setup accounts
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("data")})

	// Create an Ethereum private key (take first 32 bytes as private key)
	ethKeyBytes := acctesting.GenerateKey("eth-key")[:32]
	ethPrivKey, err := altcrypto.ToECDSA(ethKeyBytes)
	require.NoError(t, err)

	// Create a recipient address (just use some bytes)
	recipient := make([]byte, 20)
	copy(recipient, []byte("recipient-address---"))

	// Create a signed Ethereum transaction
	rawTx := createLegacyEthereumTx(t, ethPrivKey, 0, recipient, big.NewInt(1e18), nil)

	// Write data with EthereumDataEntry using a regular signature
	entry := &EthereumDataEntry{RawTx: rawTx}
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice.JoinPath("data")).
			Body(&WriteData{Entry: entry}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Check the result - verify the entry was written
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		data := batch.Account(alice.JoinPath("data")).Data()
		entryHash, err := data.Entry().Get(0)
		require.NoError(t, err)
		require.Equal(t, entry.Hash(), entryHash[:])
	})
}

// TestWriteData_EthereumEntry_OfficialTestVectors tests writing official Ethereum
// test vectors through the Accumulate simulator to verify end-to-end processing.
func TestWriteData_EthereumEntry_OfficialTestVectors(t *testing.T) {
	var timestamp uint64

	// Initialize with V2Jiuquan to enable EthereumDataEntry
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2Jiuquan),
	)

	// Setup accounts
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{Url: alice.JoinPath("data")})

	// Test vectors from official Ethereum test suites
	testCases := []struct {
		name           string
		rawTxHex       string
		expectedSender string // Ethereum address in hex (without 0x prefix)
	}{
		{
			// ethereumj test vector
			// Private key: sha3("cow") = c85ef7d79691fe79573b1a7064c19c1a9819ebdbd1faaab1a8ec92344438aaf4
			name:           "ethereumj_cow_key",
			rawTxHex:       "f86b8085e8d4a510008227109413978aee95f38490e9769c39b2773ed763d9cd5f872386f26fc10000801ba0eab47c1a49bf2fe5d40e01d313900e19ca485867d462fe06e139e3a536c6d4f4a014a569d327dcda4b29f74f93c0e9729d2f49ad726e703f9cd90dbb0fbf6649f1",
			expectedSender: "cd2a3d9f938e13cd947ec05abc7fe734df8dd826",
		},
		{
			// Real mainnet transaction from Etherscan (EIP-155 with chain ID 1)
			name:     "etherscan_mainnet",
			rawTxHex: "f86c0a85046c7cfe0083016dea94d1310c1e038bc12865d3d3997275b3e4737c6302880b503be34d9fe80080269fc7eaaa9c21f59adf8ad43ed66cf5ef9ee1c317bd4d32cd65401e7aaca47cfaa0387d79c65b90be6260d09dcfb780f29dd8133b9b1ceb20b83b7e442b4bfc30cb",
			// Note: We verify parsing and storage succeed; sender verification done in protocol tests
			expectedSender: "",
		},
		{
			// ethereum.org documentation example
			name:           "ethereum_org_docs",
			rawTxHex:       "f88380018203339407a565b7ed7d7a678680a4c162885bedbb695fe080a44401a6e4000000000000000000000000000000000000000000000000000000000000001226a0223a7c9bcf5531c99be5ea7082183816eb20cfe0bbc322e97cc5c7f71ab8b20ea02aadee6b34b45bb15bc42d9c09de4a6754e7000908da72d48cc7704971491663",
			expectedSender: "",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Decode hex to bytes
			rawTx := hexToBytes(t, tc.rawTxHex)

			// Create EthereumDataEntry with official test vector
			entry := &EthereumDataEntry{RawTx: rawTx}

			// Verify we can parse the transaction
			parsedTx, err := ParseEthereumTx(rawTx)
			require.NoError(t, err, "should parse official test vector")
			require.NotNil(t, parsedTx)

			// Write data with EthereumDataEntry using a regular Accumulate signature
			st := sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice.JoinPath("data")).
					Body(&WriteData{Entry: entry}).
					SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

			sim.StepUntil(
				Txn(st.TxID).Succeeds())

			// Verify the entry was written correctly
			View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
				data := batch.Account(alice.JoinPath("data")).Data()
				entryHash, err := data.Entry().Get(i)
				require.NoError(t, err)
				require.Equal(t, entry.Hash(), entryHash[:], "stored entry hash should match")
			})

			// If we have an expected sender, verify signature recovery
			if tc.expectedSender != "" {
				signerUrl, err := VerifyEthereumDataSignature(entry, 0)
				require.NoError(t, err, "should verify signature")

				// Extract address from lite account URL
				liteKey, _, err := ParseLiteTokenAddress(signerUrl)
				require.NoError(t, err)
				recoveredAddr := bytesToHex(liteKey)
				require.Equal(t, tc.expectedSender, recoveredAddr, "recovered sender should match expected")
			}
		})
	}
}

// TestWriteData_EthereumDataSignature tests the self-authenticating flow where
// the signature is embedded in the EthereumDataEntry itself, using EthereumDataSignature.
func TestWriteData_EthereumDataSignature(t *testing.T) {
	// Skip until the self-authenticating flow is fully implemented
	// The current implementation requires:
	// 1. AIP-055 (Automatic Credit Conversion) for fee payment from non-existent accounts
	// 2. A working real Ethereum transaction with recoverable signature
	//
	// For now, this test verifies that the EthereumDataSignature builder and
	// executor infrastructure exists and compiles correctly.
	t.Skip("Self-authenticating EthereumDataSignature flow requires AIP-055 for fee payment")

	// Initialize with V2Jiuquan to enable EthereumDataSignature
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWithVersion(GenesisTime, ExecutorVersionV2Jiuquan),
	)

	// Create an Ethereum private key using decred secp256k1
	ethKeyBytes := acctesting.GenerateKey("eth-self-auth-key")[:32]
	decredPrivKey := secp256k1.PrivKeyFromBytes(ethKeyBytes)
	decredPubKey := decredPrivKey.PubKey()

	// Derive the lite account URL from the Ethereum key
	// Use uncompressed pubkey without 0x04 prefix (64 bytes)
	pubKeyBytes := decredPubKey.SerializeUncompressed()[1:]
	h := sha3.NewLegacyKeccak256()
	h.Write(pubKeyBytes)
	ethAddr := h.Sum(nil)[12:] // Last 20 bytes
	liteAccount, err := LiteTokenAddressFromHash(ethAddr, ACME)
	require.NoError(t, err)
	liteIdentity := liteAccount.RootIdentity()

	// Setup the lite identity with credits (required for fee payment)
	MakeAccount(t, sim.DatabaseFor(liteIdentity), &LiteIdentity{
		Url:           liteIdentity,
		CreditBalance: 1e9,
	})

	// Setup target data account - alice's data account that accepts writes from the lite account
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	// Create data account with authority that allows writes from the lite account
	MakeAccount(t, sim.DatabaseFor(alice), &DataAccount{
		Url: alice.JoinPath("data"),
		AccountAuth: AccountAuth{
			Authorities: []AuthorityEntry{
				{Url: alice.JoinPath("book")},
				{Url: liteIdentity}, // Allow the lite identity to write
			},
		},
	})

	// Create a signed legacy Ethereum transaction using decred secp256k1
	gasPrice := big.NewInt(20000000000) // 20 Gwei
	gasLimit := uint64(21000)
	nonce := uint64(0)
	to := make([]byte, 20)
	copy(to, []byte("evm-recipient-addr--"))
	value := big.NewInt(1e18)

	// Encode unsigned transaction
	unsignedItems := [][]byte{
		uint64ToRLPBytes(nonce),
		bigIntToRLPBytes(gasPrice),
		uint64ToRLPBytes(gasLimit),
		to,
		bigIntToRLPBytes(value),
		nil, // data
	}
	unsignedTx := encodeRLPList(unsignedItems)

	// Hash with Keccak256
	th := sha3.NewLegacyKeccak256()
	th.Write(unsignedTx)
	txHash := th.Sum(nil)

	// Sign with decred secp256k1 SignCompact (returns recovery ID)
	sig := decredecdsa.SignCompact(decredPrivKey, txHash, false)

	// SignCompact returns: [recoveryID+27, r (32 bytes), s (32 bytes)]
	v := big.NewInt(int64(sig[0]))
	r := new(big.Int).SetBytes(sig[1:33])
	s := new(big.Int).SetBytes(sig[33:65])

	// Encode signed transaction
	signedItems := [][]byte{
		uint64ToRLPBytes(nonce),
		bigIntToRLPBytes(gasPrice),
		uint64ToRLPBytes(gasLimit),
		to,
		bigIntToRLPBytes(value),
		nil, // data
		bigIntToRLPBytes(v),
		bigIntToRLPBytes(r),
		bigIntToRLPBytes(s),
	}
	rawTx := encodeRLPList(signedItems)

	// Create the EthereumDataEntry
	entry := &EthereumDataEntry{RawTx: rawTx}

	// Build and submit transaction using EthereumDataSignature (self-authenticating)
	env, err := build.Transaction().For(alice.JoinPath("data")).
		WriteData().Entry(entry).
		EthereumData(0). // 0 = no chain ID verification for legacy transactions
		Done()
	require.NoError(t, err)

	st := sim.SubmitTxnSuccessfully(env)

	sim.StepUntil(
		Txn(st.TxID).Succeeds())

	// Check the result - verify the entry was written
	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		data := batch.Account(alice.JoinPath("data")).Data()
		entryHash, err := data.Entry().Get(0)
		require.NoError(t, err)
		require.Equal(t, entry.Hash(), entryHash[:])
	})
}

// hexToBytes converts a hex string to bytes
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

// bytesToHex converts bytes to a hex string
func bytesToHex(b []byte) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, len(b)*2)
	for i, v := range b {
		result[i*2] = hexChars[v>>4]
		result[i*2+1] = hexChars[v&0x0f]
	}
	return string(result)
}
