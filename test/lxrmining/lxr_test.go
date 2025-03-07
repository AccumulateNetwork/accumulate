// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package lxrmining provides standalone tests for LXR mining functionality
package lxrmining

import (
	"crypto/rand"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestLxrMiningCore tests the core LXR mining functionality without database dependencies
func TestLxrMiningCore(t *testing.T) {
	// Enable test environment for LXR hasher
	lxr.IsTestEnvironment = true

	// Create a hasher for mining
	hasher := lxr.NewHasher()

	// Create a block hash to mine against
	blockHash := [32]byte{}
	rand.Read(blockHash[:])

	// Create a nonce
	nonce := []byte("test nonce for mining")

	// Calculate the proof of work
	computedHash, difficulty := hasher.CalculatePow(blockHash[:], nonce)
	
	// Set a minimum difficulty for testing
	minDifficulty := uint64(100)
	
	// Verify the difficulty meets the minimum requirement
	require.GreaterOrEqual(t, difficulty, minDifficulty, "Mining difficulty should be at least %d", minDifficulty)

	// Create a valid mining signature
	validSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     blockHash,
		Signer:        protocol.AccountUrl("miner", "book0", "1"),
		SignerVersion: 1,
		Timestamp:     uint64(time.Now().Unix()),
	}

	// Verify the signature with minimum difficulty
	result := hasher.VerifySignature(validSig, minDifficulty)
	require.True(t, result, "Valid signature should be verified successfully with min difficulty")

	// Verify the signature with exact difficulty
	result = hasher.VerifySignature(validSig, difficulty)
	require.True(t, result, "Valid signature should be verified successfully with exact difficulty")

	// Verify the signature with higher difficulty (should fail)
	result = hasher.VerifySignature(validSig, difficulty+1)
	require.False(t, result, "Signature should fail verification with higher difficulty")

	// Test with invalid block hash
	invalidBlockHash := [32]byte{}
	rand.Read(invalidBlockHash[:])
	
	invalidBlockHashSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     invalidBlockHash, // Different block hash
		Signer:        protocol.AccountUrl("miner", "book0", "1"),
		SignerVersion: 1,
		Timestamp:     uint64(time.Now().Unix()),
	}
	
	// Verify the signature with invalid block hash
	result = hasher.VerifySignature(invalidBlockHashSig, minDifficulty)
	require.False(t, result, "Signature with invalid block hash should fail verification")

	// Test with invalid computed hash
	invalidComputedHash := [32]byte{}
	rand.Read(invalidComputedHash[:])
	
	invalidComputedHashSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  invalidComputedHash, // Different computed hash
		BlockHash:     blockHash,
		Signer:        protocol.AccountUrl("miner", "book0", "1"),
		SignerVersion: 1,
		Timestamp:     uint64(time.Now().Unix()),
	}
	
	// Verify the signature with invalid computed hash
	result = hasher.VerifySignature(invalidComputedHashSig, minDifficulty)
	require.False(t, result, "Signature with invalid computed hash should fail verification")

	// Test with different nonce (should fail)
	differentNonce := []byte("different nonce for mining")
	
	differentNonceSig := &protocol.LxrMiningSignature{
		Nonce:         differentNonce, // Different nonce
		ComputedHash:  computedHash,
		BlockHash:     blockHash,
		Signer:        protocol.AccountUrl("miner", "book0", "1"),
		SignerVersion: 1,
		Timestamp:     uint64(time.Now().Unix()),
	}
	
	// Verify the signature with different nonce
	result = hasher.VerifySignature(differentNonceSig, minDifficulty)
	require.False(t, result, "Signature with different nonce should fail verification")

	// Test with very high difficulty requirement
	highDifficulty := uint64(math.MaxUint64 - 1)
	result = hasher.VerifySignature(validSig, highDifficulty)
	require.False(t, result, "Signature should fail verification with very high difficulty requirement")
}
