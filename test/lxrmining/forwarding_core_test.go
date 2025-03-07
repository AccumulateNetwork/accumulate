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
	"gitlab.com/accumulatenetwork/accumulate/internal/mining"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestForwardingCore tests the core transaction forwarding functionality without database dependencies
func TestForwardingCore(t *testing.T) {
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
		Signer:          protocol.AccountUrl("miner", "book0", "1"),
		SignerVersion:   1,
		Timestamp:       uint64(time.Now().Unix()),
		TransactionData: txData,
	}

	// Track forwarded transactions
	var forwardedHash [32]byte
	var forwardedSubmissions []*mining.MiningSubmission
	
	// Create a transaction forwarder
	forwarder := mining.NewDefaultTransactionForwarder(func(bh [32]byte, subs []*mining.MiningSubmission) error {
		forwardedHash = bh
		forwardedSubmissions = subs
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

	// Verify the transaction data can be unmarshalled correctly
	parsedTx := new(protocol.Transaction)
	err = parsedTx.UnmarshalBinary(forwardedSubmissions[0].Signature.TransactionData)
	require.NoError(t, err)
	require.Equal(t, tx.Header.Principal.String(), parsedTx.Header.Principal.String())
}

// TestInvalidForwarding tests handling of invalid transactions during forwarding
func TestInvalidForwarding(t *testing.T) {
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
		Signer:          protocol.AccountUrl("miner", "book0", "1"),
		SignerVersion:   1,
		Timestamp:       uint64(time.Now().Unix()),
		TransactionData: []byte("invalid transaction data"), // Invalid data
	}

	// Track forwarding errors
	var forwardingCalled bool
	var forwardingError error
	
	// Create a transaction forwarder that tracks errors
	forwarder := mining.NewDefaultTransactionForwarder(func(bh [32]byte, subs []*mining.MiningSubmission) error {
		forwardingCalled = true
		
		// Try to unmarshal the transaction data
		if len(subs) > 0 && subs[0].Signature != nil && len(subs[0].Signature.TransactionData) > 0 {
			tx := new(protocol.Transaction)
			err := tx.UnmarshalBinary(subs[0].Signature.TransactionData)
			if err != nil {
				forwardingError = err
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

	// Verify the forwarder was called
	require.True(t, forwardingCalled)
	
	// Verify an error occurred when trying to unmarshal the transaction data
	require.NotNil(t, forwardingError)
	require.Contains(t, forwardingError.Error(), "unmarshal")
}
