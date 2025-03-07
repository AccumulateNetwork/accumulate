// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package lxrmining

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestTransactionForwarding tests the transaction forwarding functionality
func TestTransactionForwarding(t *testing.T) {
	// Create a memory database
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create a key page with mining enabled
	keyPageUrl := protocol.AccountUrl("miner", "book0", "1")
	keyPage := &protocol.KeyPage{
		Url:              keyPageUrl,
		Version:          1,
		MiningEnabled:    true,
		MiningDifficulty: 100, // Low difficulty for testing
		Keys:             []*protocol.KeySpec{{PublicKeyHash: make([]byte, 32)}},
	}

	// Save the key page to the database
	err := batch.Account(keyPage.Url).Main().Put(keyPage)
	require.NoError(t, err)

	// Create a transaction
	tx := new(protocol.Transaction)
	tx.Header.Principal = protocol.AccountUrl("miner")
	tx.Body = new(protocol.WriteData)
	txData, err := tx.MarshalBinary()
	require.NoError(t, err)

	// Create a block hash to mine against
	blockHash := [32]byte{}
	rand.Read(blockHash[:])

	// Enable test environment for LXR hasher
	lxr.IsTestEnvironment = true

	// Create a hasher for mining
	hasher := lxr.NewHasher()

	// Create a nonce
	nonce := []byte("test nonce for mining")

	// Calculate the proof of work
	computedHash, difficulty := hasher.CalculatePow(blockHash[:], nonce)
	require.GreaterOrEqual(t, difficulty, uint64(100), "Mining difficulty should be at least 100")

	// Create a mining signature
	sig := &protocol.LxrMiningSignature{
		Nonce:           nonce,
		ComputedHash:    computedHash,
		BlockHash:       blockHash,
		Signer:          keyPageUrl,
		SignerVersion:   keyPage.Version,
		Timestamp:       uint64(time.Now().Unix()),
		TransactionData: txData,
	}

	// Create a mining submission
	submission := &mining.MiningSubmission{
		Signature:  sig,
		Difficulty: difficulty,
		Timestamp:  uint64(time.Now().Unix()),
	}

	// Create a transaction forwarder
	var forwardedTx *protocol.SyntheticTransaction
	var forwardedHash [32]byte
	var forwardedSubmissions []*mining.MiningSubmission
	
	forwarder := mining.NewDefaultTransactionForwarder(func(bh [32]byte, subs []*mining.MiningSubmission) error {
		forwardedHash = bh
		forwardedSubmissions = subs
		
		// Parse the transaction data from the first submission
		if len(subs) > 0 && subs[0].Signature != nil && len(subs[0].Signature.TransactionData) > 0 {
			tx := new(protocol.Transaction)
			err := tx.UnmarshalBinary(subs[0].Signature.TransactionData)
			if err != nil {
				return err
			}
			
			// Create a synthetic transaction
			forwardedTx = &protocol.SyntheticTransaction{
				Cause: &protocol.TransactionReference{
					SourceNetwork: protocol.Directory,
					Hash:          subs[0].Signature.Hash(),
				},
				Transaction: tx,
			}
			
			// Add the synthetic transaction to the batch
			err = batch.Transaction(tx.GetHash()).Synthetic().Put(forwardedTx)
			if err != nil {
				return err
			}
			
			// Also add an entry to the pending synthetic transaction index
			err = batch.Transaction(tx.GetHash()).PendingSynthetic().Put(forwardedTx)
			if err != nil {
				return err
			}
		}
		
		return nil
	})

	// Create a validator
	validator := mining.NewCustomValidator(3600, 10, nil, forwarder)

	// Start a new mining window
	validator.StartNewWindow(blockHash, 100)

	// Submit the signature
	accepted, err := validator.SubmitSignature(sig)
	require.NoError(t, err)
	require.True(t, accepted)

	// Close the window and check if the transaction was forwarded
	submissions := validator.CloseWindow()
	require.Len(t, submissions, 1)
	require.Equal(t, sig, submissions[0].Signature)

	// Check if the transaction forwarder was called with the correct parameters
	require.Equal(t, blockHash, forwardedHash)
	require.Len(t, forwardedSubmissions, 1)
	require.Equal(t, sig, forwardedSubmissions[0].Signature)

	// Check if the synthetic transaction was created and stored correctly
	require.NotNil(t, forwardedTx)
	require.Equal(t, tx.Header.Principal, forwardedTx.Transaction.Header.Principal)

	// Verify the synthetic transaction was stored in the database
	var storedTx *protocol.SyntheticTransaction
	err = batch.Transaction(tx.GetHash()).Synthetic().GetAs(&storedTx)
	require.NoError(t, err)
	require.NotNil(t, storedTx)
	require.Equal(t, forwardedTx.Transaction.Header.Principal, storedTx.Transaction.Header.Principal)

	// Verify the synthetic transaction was added to the pending index
	var pendingTx *protocol.SyntheticTransaction
	err = batch.Transaction(tx.GetHash()).PendingSynthetic().GetAs(&pendingTx)
	require.NoError(t, err)
	require.NotNil(t, pendingTx)
	require.Equal(t, forwardedTx.Transaction.Header.Principal, pendingTx.Transaction.Header.Principal)
}

// TestInvalidTransactionForwarding tests handling of invalid transactions during forwarding
func TestInvalidTransactionForwarding(t *testing.T) {
	// Create a memory database
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create a key page with mining enabled
	keyPageUrl := protocol.AccountUrl("miner", "book0", "1")
	keyPage := &protocol.KeyPage{
		Url:              keyPageUrl,
		Version:          1,
		MiningEnabled:    true,
		MiningDifficulty: 100, // Low difficulty for testing
		Keys:             []*protocol.KeySpec{{PublicKeyHash: make([]byte, 32)}},
	}

	// Save the key page to the database
	err := batch.Account(keyPage.Url).Main().Put(keyPage)
	require.NoError(t, err)

	// Create a block hash to mine against
	blockHash := [32]byte{}
	rand.Read(blockHash[:])

	// Enable test environment for LXR hasher
	lxr.IsTestEnvironment = true

	// Create a hasher for mining
	hasher := lxr.NewHasher()

	// Create a nonce
	nonce := []byte("test nonce for mining")

	// Calculate the proof of work
	computedHash, difficulty := hasher.CalculatePow(blockHash[:], nonce)
	require.GreaterOrEqual(t, difficulty, uint64(100), "Mining difficulty should be at least 100")

	// Create a mining signature with invalid transaction data
	invalidSig := &protocol.LxrMiningSignature{
		Nonce:           nonce,
		ComputedHash:    computedHash,
		BlockHash:       blockHash,
		Signer:          keyPageUrl,
		SignerVersion:   keyPage.Version,
		Timestamp:       uint64(time.Now().Unix()),
		TransactionData: []byte("invalid transaction data"), // Invalid data
	}

	// Create a mining submission
	submission := &mining.MiningSubmission{
		Signature:  invalidSig,
		Difficulty: difficulty,
		Timestamp:  uint64(time.Now().Unix()),
	}

	// Create a transaction forwarder that tracks errors
	var forwardError error
	
	forwarder := mining.NewDefaultTransactionForwarder(func(bh [32]byte, subs []*mining.MiningSubmission) error {
		// This should encounter an error when trying to unmarshal the transaction data
		if len(subs) > 0 && subs[0].Signature != nil && len(subs[0].Signature.TransactionData) > 0 {
			tx := new(protocol.Transaction)
			err := tx.UnmarshalBinary(subs[0].Signature.TransactionData)
			if err != nil {
				forwardError = err
				return err
			}
		}
		
		return nil
	})

	// Create a validator
	validator := mining.NewCustomValidator(3600, 10, nil, forwarder)

	// Start a new mining window
	validator.StartNewWindow(blockHash, 100)

	// Submit the signature
	accepted, err := validator.SubmitSignature(invalidSig)
	require.NoError(t, err)
	require.True(t, accepted)

	// Close the window
	submissions := validator.CloseWindow()
	require.Len(t, submissions, 1)
	require.Equal(t, invalidSig, submissions[0].Signature)

	// The forwarder should have encountered an error
	require.NotNil(t, forwardError)
	require.Contains(t, forwardError.Error(), "unmarshal")
}
