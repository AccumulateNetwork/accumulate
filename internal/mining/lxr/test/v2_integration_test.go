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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestLxrMiningV2Integration tests the LXR mining signature validation in a v2-like environment
func TestLxrMiningV2Integration(t *testing.T) {
	// Initialize test environment
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	// Create a key page with mining enabled
	keyPage := &protocol.KeyPage{
		Url:              protocol.AccountUrl("miner", "book0", "1"),
		Version:          1,
		MiningEnabled:    true,
		MiningDifficulty: 100, // Low difficulty for testing
	}

	// Add a key to the key page
	keyEntry := &protocol.KeySpec{
		PublicKeyHash: make([]byte, 32),
	}
	keyPage.Keys = append(keyPage.Keys, keyEntry)

	// Save the key page to the database
	err := batch.Account(keyPage.Url).Main().Put(keyPage)
	require.NoError(t, err)

	// Create a transaction to sign
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.AccountUrl("miner")
	txn.Body = new(protocol.WriteData)

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

	// Create a valid mining signature
	validSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     blockHash,
		Signer:        keyPage.Url,
		SignerVersion: keyPage.Version,
		Timestamp:     uint64(time.Now().Unix()),
	}

	// Verify the signature directly
	result := hasher.VerifySignature(validSig, keyPage.MiningDifficulty)
	require.True(t, result, "Valid signature should be verified successfully")

	// Simulate v2 validation logic
	// 1. Get the signer (key page)
	account, err := batch.Account(validSig.Signer).Main().Get()
	require.NoError(t, err, "Failed to get signer account")
	
	signerKeyPage, ok := account.(*protocol.KeyPage)
	require.True(t, ok, "Signer account should be a key page")
	
	// 2. Check if mining is enabled
	require.True(t, signerKeyPage.MiningEnabled, "Mining should be enabled on the key page")
	
	// 3. Check if the signature meets the difficulty requirement
	require.True(t, hasher.VerifySignature(validSig, signerKeyPage.MiningDifficulty), 
		"Signature should meet the difficulty requirement")

	// Test with mining disabled
	disabledKeyPage := &protocol.KeyPage{
		Url:           protocol.AccountUrl("disabled", "book0", "1"),
		Version:       1,
		MiningEnabled: false,
	}
	
	// Add a key to the key page
	disabledKeyPage.Keys = append(disabledKeyPage.Keys, keyEntry)
	
	// Save the key page to the database
	err = batch.Account(disabledKeyPage.Url).Main().Put(disabledKeyPage)
	require.NoError(t, err)
	
	// Create a signature with the disabled key page
	invalidSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     blockHash,
		Signer:        disabledKeyPage.Url,
		SignerVersion: disabledKeyPage.Version,
		Timestamp:     uint64(time.Now().Unix()),
	}
	
	// Simulate v2 validation logic for disabled mining
	account, err = batch.Account(invalidSig.Signer).Main().Get()
	require.NoError(t, err, "Failed to get signer account")
	
	disabledSignerKeyPage, ok := account.(*protocol.KeyPage)
	require.True(t, ok, "Signer account should be a key page")
	
	// Check if mining is enabled (should be false)
	require.False(t, disabledSignerKeyPage.MiningEnabled, "Mining should be disabled on the key page")

	// Test with insufficient difficulty
	highDifficultyKeyPage := &protocol.KeyPage{
		Url:              protocol.AccountUrl("high-diff", "book0", "1"),
		Version:          1,
		MiningEnabled:    true,
		MiningDifficulty: difficulty + 1000, // Much higher than what our signature provides
	}
	
	// Add a key to the key page
	highDifficultyKeyPage.Keys = append(highDifficultyKeyPage.Keys, keyEntry)
	
	// Save the key page to the database
	err = batch.Account(highDifficultyKeyPage.Url).Main().Put(highDifficultyKeyPage)
	require.NoError(t, err)
	
	// Create a signature with the high difficulty key page
	insufficientSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     blockHash,
		Signer:        highDifficultyKeyPage.Url,
		SignerVersion: highDifficultyKeyPage.Version,
		Timestamp:     uint64(time.Now().Unix()),
	}
	
	// Simulate v2 validation logic for insufficient difficulty
	account, err = batch.Account(insufficientSig.Signer).Main().Get()
	require.NoError(t, err, "Failed to get signer account")
	
	highDiffSignerKeyPage, ok := account.(*protocol.KeyPage)
	require.True(t, ok, "Signer account should be a key page")
	
	// Check if mining is enabled
	require.True(t, highDiffSignerKeyPage.MiningEnabled, "Mining should be enabled on the key page")
	
	// Check if the signature meets the difficulty requirement (should be false)
	require.False(t, hasher.VerifySignature(insufficientSig, highDiffSignerKeyPage.MiningDifficulty), 
		"Signature should not meet the high difficulty requirement")

	// Test with invalid block hash
	invalidBlockHash := [32]byte{}
	rand.Read(invalidBlockHash[:])
	
	invalidBlockHashSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     invalidBlockHash, // Different block hash
		Signer:        keyPage.Url,
		SignerVersion: keyPage.Version,
		Timestamp:     uint64(time.Now().Unix()),
	}
	
	// Verify the signature with invalid block hash
	result = hasher.VerifySignature(invalidBlockHashSig, keyPage.MiningDifficulty)
	require.False(t, result, "Signature with invalid block hash should fail verification")

	// Test with invalid computed hash
	invalidComputedHash := [32]byte{}
	rand.Read(invalidComputedHash[:])
	
	invalidComputedHashSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  invalidComputedHash, // Different computed hash
		BlockHash:     blockHash,
		Signer:        keyPage.Url,
		SignerVersion: keyPage.Version,
		Timestamp:     uint64(time.Now().Unix()),
	}
	
	// Verify the signature with invalid computed hash
	result = hasher.VerifySignature(invalidComputedHashSig, keyPage.MiningDifficulty)
	require.False(t, result, "Signature with invalid computed hash should fail verification")
}
