// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestAccumulateTransactionGenerator_New(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	gen, err := NewAccumulateTransactionGenerator(priv)
	require.NoError(t, err)

	// Verify the lite token account URL is valid
	assert.NotNil(t, gen.GetLiteTokenAccount())
	assert.Contains(t, gen.GetLiteTokenAccount().String(), "acc://")
	assert.Contains(t, gen.GetLiteTokenAccount().String(), "/ACME")
}

func TestAccumulateTransactionGenerator_GenerateSendTokens(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	gen, err := NewAccumulateTransactionGenerator(priv)
	require.NoError(t, err)

	// Generate a send tokens transaction
	env, err := gen.GenerateSelfSendTokens(1)
	require.NoError(t, err)
	require.NotNil(t, env)

	// Verify the envelope structure
	assert.Len(t, env.Transaction, 1)
	assert.Len(t, env.Signatures, 1)

	// Verify the transaction type
	txn := env.Transaction[0]
	assert.Equal(t, protocol.TransactionTypeSendTokens, txn.Body.Type())

	// Verify the transaction body
	sendTokens, ok := txn.Body.(*protocol.SendTokens)
	require.True(t, ok)
	assert.NotEmpty(t, sendTokens.To)
}

func TestAccumulateTransactionGenerator_GenerateMultipleTransactions(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	gen, err := NewAccumulateTransactionGenerator(priv)
	require.NoError(t, err)

	// Generate multiple transactions
	hashes := make(map[[32]byte]bool)
	for i := 0; i < 10; i++ {
		env, err := gen.GenerateSelfSendTokens(1)
		require.NoError(t, err)
		require.NotNil(t, env)

		// Each transaction should have a unique hash (due to different timestamps)
		hash := ExtractTransactionHash(env)
		assert.False(t, hashes[hash], "Transaction hash should be unique")
		hashes[hash] = true
	}

	assert.Len(t, hashes, 10)
}

func TestAccumulateTransactionGenerator_MarshalUnmarshal(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	gen, err := NewAccumulateTransactionGenerator(priv)
	require.NoError(t, err)

	// Generate a transaction
	env, err := gen.GenerateSelfSendTokens(1)
	require.NoError(t, err)

	// Marshal the envelope
	data, err := env.MarshalBinary()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// The executor should be able to process this
	executor, err := NewExecutor(ExecutorConfig{
		Validators:    []ed25519.PublicKey{gen.GetPublicKey()},
		BlockInterval: 1 * time.Second,
		TxRate:        100,
	})
	require.NoError(t, err)
	defer executor.Cleanup()

	// Process the transaction
	err = executor.ProcessTransaction(data)
	require.NoError(t, err)

	// Verify it was processed as an Accumulate transaction
	assert.Equal(t, uint64(1), executor.GetAccumulateTxCount())
	assert.Equal(t, uint64(0), executor.GetLegacyTxCount())
}

func TestValidateEnvelope(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	gen, err := NewAccumulateTransactionGenerator(priv)
	require.NoError(t, err)

	// Generate a valid transaction
	env, err := gen.GenerateSelfSendTokens(1)
	require.NoError(t, err)

	// Validate the envelope
	assert.True(t, ValidateEnvelope(env))

	// Test with nil envelope
	assert.False(t, ValidateEnvelope(nil))
}

func TestCreateGenesisAccounts(t *testing.T) {
	// Generate some keys
	keys := make([]ed25519.PrivateKey, 3)
	for i := range keys {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys[i] = priv
	}

	// Create genesis accounts
	accounts, err := CreateGenesisAccounts(keys, 1000)
	require.NoError(t, err)
	assert.Len(t, accounts, 3)

	// Verify each account has a valid URL and balance
	for _, acc := range accounts {
		assert.NotNil(t, acc.URL)
		assert.Contains(t, acc.URL.String(), "acc://")
		assert.NotNil(t, acc.PublicKey)
		assert.True(t, acc.Balance.Sign() > 0)
	}
}

func TestExecutor_ProcessAccumulateAndLegacyTransactions(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	executor, err := NewExecutor(ExecutorConfig{
		Validators:    []ed25519.PublicKey{pub},
		BlockInterval: 1 * time.Second,
		TxRate:        100,
	})
	require.NoError(t, err)
	defer executor.Cleanup()

	// Create Accumulate transaction generator
	gen, err := NewAccumulateTransactionGenerator(priv)
	require.NoError(t, err)

	// Process an Accumulate transaction
	env, err := gen.GenerateSelfSendTokens(1)
	require.NoError(t, err)
	data, err := env.MarshalBinary()
	require.NoError(t, err)
	err = executor.ProcessTransaction(data)
	require.NoError(t, err)

	// Process a legacy transaction
	legacyTx := NewDataTx(pub, []byte("legacy data"), 1)
	legacyTx.Sign(priv)
	err = executor.ProcessTransaction(legacyTx.Marshal())
	require.NoError(t, err)

	// Verify counts
	assert.Equal(t, uint64(2), executor.GetProcessedCount())
	assert.Equal(t, uint64(1), executor.GetAccumulateTxCount())
	assert.Equal(t, uint64(1), executor.GetLegacyTxCount())
}
