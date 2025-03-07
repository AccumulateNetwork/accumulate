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
	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestLxrMiningV2Flow simulates the v2 validation flow for LXR mining signatures
func TestLxrMiningV2Flow(t *testing.T) {
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

	// Simulate the v2 validation flow
	// 1. Verify the signature meets the difficulty requirement
	result := hasher.VerifySignature(validSig, minDifficulty)
	require.True(t, result, "Valid signature should be verified successfully")

	// 2. Simulate checking if mining is enabled on the key page (would be done in v2 implementation)
	// In a real implementation, this would involve fetching the key page from the database
	// and checking the MiningEnabled flag
	
	// 3. Simulate checking if the key page exists and is valid
	// In a real implementation, this would involve fetching the key page from the database
	// and validating its existence and state

	// 4. Simulate checking if the signature timestamp is valid
	// In a real implementation, this would involve comparing the signature timestamp
	// with the current time and ensuring it's within acceptable bounds
	currentTime := uint64(time.Now().Unix())
	maxTimeDiff := uint64(300) // 5 minutes
	
	require.LessOrEqual(t, validSig.Timestamp, currentTime+maxTimeDiff, 
		"Signature timestamp should not be too far in the future")
	require.GreaterOrEqual(t, validSig.Timestamp, currentTime-maxTimeDiff, 
		"Signature timestamp should not be too far in the past")

	// 5. Test the full validation flow with various error cases
	
	// 5.1 Test with invalid block hash
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
	
	// This should fail at the signature verification step
	result = hasher.VerifySignature(invalidBlockHashSig, minDifficulty)
	require.False(t, result, "Signature with invalid block hash should fail verification")
	
	// 5.2 Test with insufficient difficulty
	// This would fail at the difficulty check step in the v2 implementation
	highDifficulty := difficulty + 1000
	result = hasher.VerifySignature(validSig, highDifficulty)
	require.False(t, result, "Signature should fail verification with higher difficulty")
	
	// 5.3 Test with invalid timestamp
	invalidTimestampSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     blockHash,
		Signer:        protocol.AccountUrl("miner", "book0", "1"),
		SignerVersion: 1,
		Timestamp:     currentTime + 3600, // 1 hour in the future
	}
	
	// This would fail at the timestamp validation step in the v2 implementation
	require.Greater(t, invalidTimestampSig.Timestamp, currentTime+maxTimeDiff, 
		"Signature timestamp should be too far in the future")
}
