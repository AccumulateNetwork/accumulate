// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package test provides standalone tests for LXR mining functionality
package test

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestLxrMiningCore tests the core LXR mining functionality
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

	// Verify the signature
	result := hasher.VerifySignature(validSig, minDifficulty)
	require.True(t, result, "Valid signature should be verified successfully")

	// Try with an invalid signature (wrong block hash)
	invalidBlockHash := [32]byte{}
	rand.Read(invalidBlockHash[:])

	invalidSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     invalidBlockHash, // Different block hash
		Signer:        protocol.AccountUrl("miner", "book0", "1"),
		SignerVersion: 1,
		Timestamp:     uint64(time.Now().Unix()),
	}

	// Verify the invalid signature
	result = hasher.VerifySignature(invalidSig, minDifficulty)
	require.False(t, result, "Invalid signature should fail verification")

	// Try with an invalid signature (wrong computed hash)
	invalidComputedHash := [32]byte{}
	rand.Read(invalidComputedHash[:])

	invalidSig2 := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  invalidComputedHash, // Different computed hash
		BlockHash:     blockHash,
		Signer:        protocol.AccountUrl("miner", "book0", "1"),
		SignerVersion: 1,
		Timestamp:     uint64(time.Now().Unix()),
	}

	// Verify the invalid signature
	result = hasher.VerifySignature(invalidSig2, minDifficulty)
	require.False(t, result, "Invalid signature with wrong computed hash should fail verification")

	// Try with a higher difficulty requirement
	highDifficulty := difficulty + 1000
	result = hasher.VerifySignature(validSig, highDifficulty)
	require.False(t, result, "Signature should fail verification with higher difficulty requirement")
}
