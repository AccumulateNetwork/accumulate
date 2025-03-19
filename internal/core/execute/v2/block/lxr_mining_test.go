// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/testdata"
)

func TestVerifyLxrMiningSignature(t *testing.T) {
	// Create a test database
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create a test key page with mining enabled
	keyPage := &protocol.KeyPage{
		Url:             testdata.AccountUrl("test", "key-page"),
		Version:         1,
		MiningEnabled:   true,
		MiningDifficulty: 0, // Use 0 difficulty for testing
	}

	// Save the key page to the database
	require.NoError(t, batch.Account(keyPage.Url).Main().Put(keyPage))

	// Create a test transaction
	transaction := &protocol.Transaction{
		Header: &protocol.TransactionHeader{
			Principal: testdata.AccountUrl("test", "account"),
		},
		Body: &protocol.SendTokens{},
	}

	// Create a test block hash
	blockHash := sha256.Sum256([]byte("test block"))

	// Create a test nonce
	nonce := []byte("test nonce")

	// Create a hasher
	hasher := lxr.NewCustomHasher(3, 20, 2) // Use smaller values for testing

	// Calculate the proof of work
	computedHash, _ := hasher.CalculatePow(blockHash[:], nonce)

	// Create a valid signature
	validSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     blockHash,
		Signer:        keyPage.Url,
		SignerVersion: keyPage.Version,
		Timestamp:     uint64(time.Now().Unix()),
	}

	// Verify the signature
	err := verifyLxrMiningSignature(batch, transaction, validSig, sigExecMetadata{})
	require.NoError(t, err)

	// Create an invalid signature with wrong signer version
	invalidSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     blockHash,
		Signer:        keyPage.Url,
		SignerVersion: keyPage.Version + 1, // Wrong version
		Timestamp:     uint64(time.Now().Unix()),
	}

	// Verify the invalid signature
	err = verifyLxrMiningSignature(batch, transaction, invalidSig, sigExecMetadata{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "signer version mismatch")

	// Disable mining and try again with the valid signature
	keyPage.MiningEnabled = false
	require.NoError(t, batch.Account(keyPage.Url).Main().Put(keyPage))

	err = verifyLxrMiningSignature(batch, transaction, validSig, sigExecMetadata{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "mining is not enabled")
}

func TestLxrMiningValidator(t *testing.T) {
	// Create a logger
	logger := logging.NewTestLogger(t, "lxr-mining")

	// Create a validator
	validator := NewLxrMiningValidator(logger)

	// Create a test block hash
	blockHash := sha256.Sum256([]byte("test block"))

	// Start a new mining window
	validator.StartNewWindow(nil, blockHash, 0) // Use 0 difficulty for testing

	// Verify the window is active
	require.True(t, validator.IsWindowActive())

	// Create a test nonce
	nonce := []byte("test nonce")

	// Create a hasher
	hasher := lxr.NewCustomHasher(3, 20, 2) // Use smaller values for testing

	// Calculate the proof of work
	computedHash, _ := hasher.CalculatePow(blockHash[:], nonce)

	// Create a valid signature
	signerURL, _ := url.Parse("acc://test/signer")
	validSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     blockHash,
		Signer:        signerURL,
		SignerVersion: 1,
		Timestamp:     uint64(time.Now().Unix()),
	}

	// Submit the signature
	accepted, err := validator.SubmitSignature(validSig)
	require.NoError(t, err)
	require.True(t, accepted)

	// Get the top submissions
	submissions := validator.GetTopSubmissions()
	require.Len(t, submissions, 1)

	// Close the window
	closedSubmissions := validator.CloseWindow()
	require.Len(t, closedSubmissions, 1)

	// Verify the window is no longer active
	require.False(t, validator.IsWindowActive())
}

func TestLxrMiningValidatorWithRewards(t *testing.T) {
	// Create a test database
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create a logger
	logger := logging.NewTestLogger(t, "lxr-mining-rewards")

	// Track rewards distributed
	rewardsDistributed := make(map[string]uint64)

	// Create a validator with custom reward distributor
	validator := NewLxrMiningValidator(logger)

	// Set the batch for database operations
	validator.SetBatch(batch)

	// Create a test block hash
	blockHash := sha256.Sum256([]byte("test block"))

	// Start a new mining window
	validator.StartNewWindow(nil, blockHash, 0) // Use 0 difficulty for testing

	// Create multiple test signatures
	hasher := lxr.NewCustomHasher(3, 20, 2) // Use smaller values for testing

	// Create multiple miners
	miners := []string{"acc://test/miner1", "acc://test/miner2", "acc://test/miner3"}
	for i, minerURL := range miners {
		// Create test accounts for each miner
		signerURL, _ := url.Parse(minerURL)
		lta := &protocol.LiteTokenAccount{
			Url:     signerURL,
			Balance: 0,
		}
		require.NoError(t, batch.Account(signerURL).Main().Put(lta))

		// Create a test nonce
		nonce := []byte("test nonce " + string(rune('A'+i)))

		// Calculate the proof of work
		computedHash, _ := hasher.CalculatePow(blockHash[:], nonce)

		// Create a valid signature
		validSig := &protocol.LxrMiningSignature{
			Nonce:         nonce,
			ComputedHash:  computedHash,
			BlockHash:     blockHash,
			Signer:        signerURL,
			SignerVersion: 1,
			Timestamp:     uint64(time.Now().Unix()),
		}

		// Submit the signature
		accepted, err := validator.SubmitSignature(validSig)
		require.NoError(t, err)
		require.True(t, accepted)
	}

	// Get the top submissions
	submissions := validator.GetTopSubmissions()
	require.Len(t, submissions, 3)

	// Close the window and distribute rewards
	totalReward := uint64(900) // 900 tokens to distribute
	closedSubmissions, err := validator.CloseWindowAndDistributeRewards(totalReward)
	require.NoError(t, err)
	require.Len(t, closedSubmissions, 3)

	// Verify the rewards were distributed correctly
	expectedReward := uint64(300) // 900 / 3 = 300
	for _, minerURL := range miners {
		signerURL, _ := url.Parse(minerURL)
		account, err := batch.Account(signerURL).Main().Get()
		require.NoError(t, err)

		lta, ok := account.(*protocol.LiteTokenAccount)
		require.True(t, ok)
		require.Equal(t, expectedReward, lta.Balance)
	}

	// Verify the window is no longer active
	require.False(t, validator.IsWindowActive())
}
