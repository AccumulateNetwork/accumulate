// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package lxr

import (
	"crypto/sha256"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	// Set the test environment flag to use smaller hash tables
	IsTestEnvironment = true
}

func TestHasher_CalculatePow(t *testing.T) {
	// Create a hasher with very small parameters for testing
	hasher := NewCustomHasher(1, 12, 1) // Use very small values for testing
	
	// Create a test block hash
	blockHash := sha256.Sum256([]byte("test block"))
	
	// Create a test nonce
	nonce := []byte("test nonce")
	
	// Calculate the proof of work
	computedHash, difficulty := hasher.CalculatePow(blockHash[:], nonce)
	
	// Verify that the computed hash is not zero
	isZero := true
	for _, b := range computedHash {
		if b != 0 {
			isZero = false
			break
		}
	}
	if isZero {
		t.Error("Computed hash is all zeros")
	}
	
	// Verify that the difficulty is not zero
	if difficulty == 0 {
		t.Error("Difficulty is zero")
	}
	
	// Calculate the proof of work again with the same inputs
	computedHash2, difficulty2 := hasher.CalculatePow(blockHash[:], nonce)
	
	// Verify that the results are deterministic
	for i := 0; i < 32; i++ {
		if computedHash[i] != computedHash2[i] {
			t.Errorf("Computed hash is not deterministic: %x != %x", computedHash, computedHash2)
			break
		}
	}
	if difficulty != difficulty2 {
		t.Errorf("Difficulty is not deterministic: %d != %d", difficulty, difficulty2)
	}
}

func TestHasher_VerifySignature(t *testing.T) {
	// Create a hasher with very small parameters for testing
	hasher := NewCustomHasher(1, 12, 1) // Use very small values for testing
	
	// Create a test block hash
	blockHash := sha256.Sum256([]byte("test block"))
	
	// Create a test nonce
	nonce := []byte("test nonce")
	
	// Calculate the proof of work
	computedHash, difficulty := hasher.CalculatePow(blockHash[:], nonce)
	
	// Create a valid signature
	validSig := &protocol.LxrMiningSignature{
		Nonce:        nonce,
		ComputedHash: computedHash,
		BlockHash:    blockHash,
		// Other fields not relevant for verification
	}
	
	// Create an invalid signature with wrong computed hash
	invalidSig := &protocol.LxrMiningSignature{
		Nonce:        nonce,
		ComputedHash: [32]byte{}, // Zero hash
		BlockHash:    blockHash,
		// Other fields not relevant for verification
	}
	
	// Verify the valid signature
	if !hasher.VerifySignature(validSig, 0) {
		t.Error("Valid signature verification failed")
	}
	
	// Verify the invalid signature
	if hasher.VerifySignature(invalidSig, 0) {
		t.Error("Invalid signature verification passed")
	}
	
	// Verify with a minimum difficulty requirement
	if !hasher.VerifySignature(validSig, difficulty-1) {
		t.Error("Valid signature verification failed with lower difficulty requirement")
	}
	
	if hasher.VerifySignature(validSig, difficulty+1) {
		t.Error("Valid signature verification passed with higher difficulty requirement")
	}
}

func TestHasher_CheckDifficulty(t *testing.T) {
	// Create a hasher with very small parameters for testing
	hasher := NewCustomHasher(1, 12, 1) // Use very small values for testing
	
	// Create a test block hash
	blockHash := sha256.Sum256([]byte("test block"))
	
	// Create a test nonce
	nonce := []byte("test nonce")
	
	// Calculate the proof of work
	_, difficulty := hasher.CalculatePow(blockHash[:], nonce)
	
	// Check with a lower difficulty requirement
	meets, actual := hasher.CheckDifficulty(blockHash[:], nonce, difficulty-1)
	if !meets {
		t.Error("CheckDifficulty failed with lower difficulty requirement")
	}
	if actual != difficulty {
		t.Errorf("CheckDifficulty returned wrong difficulty: %d != %d", actual, difficulty)
	}
	
	// Check with a higher difficulty requirement
	meets, actual = hasher.CheckDifficulty(blockHash[:], nonce, difficulty+1)
	if meets {
		t.Error("CheckDifficulty passed with higher difficulty requirement")
	}
	if actual != difficulty {
		t.Errorf("CheckDifficulty returned wrong difficulty: %d != %d", actual, difficulty)
	}
}
