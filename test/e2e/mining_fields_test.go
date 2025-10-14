// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestMiningFields_BasicPersistence(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Create identity using helper function
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	// Verify initial state - no mining fields
	page := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.Len(t, page.Keys, 1)
	require.Nil(t, page.Keys[0].MiningDifficulty)
	require.Equal(t, uint64(0), page.Keys[0].MiningExpiry)

	// Update key with mining fields directly using UpdateAccount
	miningDifficulty := make([]byte, 32)
	copy(miningDifficulty, []byte("test-mining-difficulty-target"))
	miningExpiry := uint64(1000000)

	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.Keys[0].MiningDifficulty = miningDifficulty
		page.Keys[0].MiningExpiry = miningExpiry
	})

	// Verify mining fields are persisted
	page = GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.Len(t, page.Keys, 1)
	require.Equal(t, miningDifficulty, page.Keys[0].MiningDifficulty)
	require.Equal(t, miningExpiry, page.Keys[0].MiningExpiry)
}

func TestMiningFields_UpdateOperations(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Create identity with initial mining configuration
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		// Add initial mining configuration
		initialDifficulty := make([]byte, 32)
		copy(initialDifficulty, []byte("initial-difficulty-target"))
		page.Keys[0].MiningDifficulty = initialDifficulty
		page.Keys[0].MiningExpiry = 500000
	})

	// Test updating mining difficulty
	newDifficulty := make([]byte, 32)
	copy(newDifficulty, []byte("updated-difficulty-target"))

	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.Keys[0].MiningDifficulty = newDifficulty
	})

	// Verify difficulty was updated, expiry unchanged
	page := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.Equal(t, newDifficulty, page.Keys[0].MiningDifficulty)
	require.Equal(t, uint64(500000), page.Keys[0].MiningExpiry) // Should remain unchanged

	// Test updating mining expiry
	newExpiry := uint64(750000)
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.Keys[0].MiningExpiry = newExpiry
	})

	// Verify expiry was updated, difficulty unchanged
	page = GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.Equal(t, newDifficulty, page.Keys[0].MiningDifficulty) // Should remain unchanged
	require.Equal(t, newExpiry, page.Keys[0].MiningExpiry)
}

func TestMiningFields_DisableMining(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Create identity with mining enabled
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
		// Enable mining initially
		difficulty := make([]byte, 32)
		copy(difficulty, []byte("enabled-mining-target"))
		page.Keys[0].MiningDifficulty = difficulty
		page.Keys[0].MiningExpiry = 1000000
	})

	// Verify mining is initially enabled
	page := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.NotNil(t, page.Keys[0].MiningDifficulty)
	require.NotEqual(t, uint64(0), page.Keys[0].MiningExpiry)

	// Disable mining by setting difficulty to nil and expiry to 0
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.Keys[0].MiningDifficulty = nil
		page.Keys[0].MiningExpiry = 0
	})

	// Verify mining is disabled
	page = GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.Nil(t, page.Keys[0].MiningDifficulty)
	require.Equal(t, uint64(0), page.Keys[0].MiningExpiry)
}

func TestMiningFields_MultipleKeys(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey1 := acctesting.GenerateKey(alice, "key1")
	aliceKey2 := acctesting.GenerateKey(alice, "key2")
	aliceKeyHash1 := sha256.Sum256(aliceKey1[32:])
	aliceKeyHash2 := sha256.Sum256(aliceKey2[32:])

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Create identity
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey1[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	// Add second key with different mining configuration
	difficulty1 := make([]byte, 32)
	copy(difficulty1, []byte("key1-mining-difficulty"))
	difficulty2 := make([]byte, 32)
	copy(difficulty2, []byte("key2-mining-difficulty"))

	// Add key2 with mining configuration
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		keyHash2 := sha256.Sum256(aliceKey2[32:])
		keySpec2 := &KeySpec{
			PublicKeyHash:    keyHash2[:],
			MiningDifficulty: difficulty2,
			MiningExpiry:     0, // No expiry for key2
		}
		page.AddKeySpec(keySpec2)
	})

	// Update key1 with mining configuration
	expiry1 := uint64(500000)
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.Keys[0].MiningDifficulty = difficulty1
		page.Keys[0].MiningExpiry = expiry1
	})

	// Verify both keys have different mining configurations
	page := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.Len(t, page.Keys, 2)

	// Find keys by hash and verify configurations
	var key1Spec, key2Spec *KeySpec
	for _, key := range page.Keys {
		if bytes.Equal(key.PublicKeyHash, aliceKeyHash1[:]) {
			key1Spec = key
		} else if bytes.Equal(key.PublicKeyHash, aliceKeyHash2[:]) {
			key2Spec = key
		}
	}

	require.NotNil(t, key1Spec, "Key1 not found")
	require.NotNil(t, key2Spec, "Key2 not found")

	// Verify key1 configuration
	require.Equal(t, difficulty1, key1Spec.MiningDifficulty)
	require.Equal(t, expiry1, key1Spec.MiningExpiry)

	// Verify key2 configuration
	require.Equal(t, difficulty2, key2Spec.MiningDifficulty)
	require.Equal(t, uint64(0), key2Spec.MiningExpiry) // No expiry
}

func TestMiningFields_NetworkConsistency(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize multi-partition simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Create identity on BVN0
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	// Add mining configuration
	miningDifficulty := make([]byte, 32)
	copy(miningDifficulty, []byte("network-test-difficulty"))
	miningExpiry := uint64(999999)

	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.Keys[0].MiningDifficulty = miningDifficulty
		page.Keys[0].MiningExpiry = miningExpiry
	})

	// Execute several blocks to ensure state propagation
	for i := 0; i < 5; i++ {
		sim.Step()
	}

	// Verify consistency across all partitions that can access the account
	// Test on the primary BVN that owns alice's account
	alicePage := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.Len(t, alicePage.Keys, 1, "Primary BVN: wrong number of keys")
	require.Equal(t, miningDifficulty, alicePage.Keys[0].MiningDifficulty, "Primary BVN: wrong mining difficulty")
	require.Equal(t, miningExpiry, alicePage.Keys[0].MiningExpiry, "Primary BVN: wrong mining expiry")

	// Note: Directory network doesn't store individual key pages, only directory entries
	// This test verifies the mining fields persist correctly on the primary BVN
	t.Log("Mining field network consistency test completed successfully")
}
