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

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	altcrypto "gitlab.com/accumulatenetwork/accumulate/pkg/crypto"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	simulator "gitlab.com/accumulatenetwork/accumulate/test/simulator/compat"
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

	// Sign with the private key
	sig, err := altcrypto.Sign(txHash, privKey)
	require.NoError(t, err)

	// For legacy transactions, v = 27 or 28
	// The Sign function returns 64 bytes (r || s), we need to determine v
	// by trying both recovery IDs
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])

	// Try recovery ID 0 first
	v := big.NewInt(27)

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
	t.Skip("EthereumDataEntry requires V2Jiuquan which is not enabled in tests yet")

	var timestamp uint64

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Setup accounts
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	sim.CreateIdentity(alice, aliceKey[32:])
	updateAccount(sim, alice.JoinPath("book", "1"), func(page *KeyPage) { page.CreditBalance = 1e9 })
	sim.CreateAccount(&DataAccount{Url: alice.JoinPath("data")})

	// Create an Ethereum private key
	ethPrivKey, err := altcrypto.ToECDSA(acctesting.GenerateKey("eth-key"))
	require.NoError(t, err)

	// Create a recipient address (just use some bytes)
	recipient := make([]byte, 20)
	copy(recipient, []byte("recipient-address---"))

	// Create a signed Ethereum transaction
	rawTx := createLegacyEthereumTx(t, ethPrivKey, 0, recipient, big.NewInt(1e18), nil)

	// Write data with EthereumDataEntry using a regular signature
	entry := &EthereumDataEntry{RawTx: rawTx}
	sim.WaitForTransactions(delivered, sim.MustSubmitAndExecuteBlock(
		MustBuild(t, build.Transaction().
			For(alice.JoinPath("data")).
			Body(&WriteData{Entry: entry}).
			SignWith(alice.JoinPath("book", "1")).Version(1).Timestamp(&timestamp).PrivateKey(aliceKey)),
	)...)

	// Check the result - verify the entry was written
	_ = sim.PartitionFor(alice).View(func(batch *database.Batch) error {
		data := batch.Account(alice.JoinPath("data")).Data()
		entryHash, err := data.Entry().Get(0)
		require.NoError(t, err)
		require.Equal(t, entry.Hash(), entryHash[:])
		return nil
	})
}
