// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestMiningE2E_ThirdPartyAppWorkflow(t *testing.T) {
	// Simulate a third-party app that enables mining for users
	appName := "mining-app"
	app := AccountUrl(appName)
	appKey := acctesting.GenerateKey(app)

	// User accounts
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	bob := AccountUrl("bob")
	bobKey := acctesting.GenerateKey(bob)

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	t.Log("=== Phase 1: Setup App and Users ===")
	
	// Create mining app identity
	MakeIdentity(t, sim.DatabaseFor(app), app, appKey[32:])

	// Create user identities
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])

	// Add credits for operations
	UpdateAccount(t, sim.DatabaseFor(app), app.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	t.Log("=== Phase 2: App Registers Users for Mining ===")

	// App enables mining for Alice (premium tier - easier difficulty)
	premiumDifficulty := make([]byte, 32)
	copy(premiumDifficulty[:4], []byte{0x00, 0x00, 0x0F, 0xFF}) // Easier target
	premiumExpiry := uint64(time.Now().Unix() + 30*24*3600) // 30 days

	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.Keys[0].MiningDifficulty = premiumDifficulty
		page.Keys[0].MiningExpiry = premiumExpiry
	})

	// App enables mining for Bob (standard tier - harder difficulty)
	standardDifficulty := make([]byte, 32)
	copy(standardDifficulty[:4], []byte{0x00, 0x00, 0x00, 0xFF}) // Harder target
	standardExpiry := uint64(time.Now().Unix() + 7*24*3600) // 7 days

	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(page *KeyPage) {
		page.Keys[0].MiningDifficulty = standardDifficulty
		page.Keys[0].MiningExpiry = standardExpiry
	})

	t.Log("=== Phase 3: Verify Mining Configurations ===")

	// Verify Alice's premium configuration
	alicePage := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.Equal(t, premiumDifficulty, alicePage.Keys[0].MiningDifficulty, "Alice should have premium difficulty")
	require.Equal(t, premiumExpiry, alicePage.Keys[0].MiningExpiry, "Alice should have premium expiry")

	// Verify Bob's standard configuration
	bobPage := GetAccount[*KeyPage](t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"))
	require.Equal(t, standardDifficulty, bobPage.Keys[0].MiningDifficulty, "Bob should have standard difficulty")
	require.Equal(t, standardExpiry, bobPage.Keys[0].MiningExpiry, "Bob should have standard expiry")

	t.Log("=== Phase 4: Simulate Mining Activity ===")

	// In a real implementation, this would involve:
	// 1. Users generating proof-of-work against their difficulty targets
	// 2. Submitting mining transactions with the proofs
	// 3. Network validating proofs against stored difficulties
	// 4. Rewarding successful miners
	//
	// For now, we verify that the mining configurations are properly stored
	// and would be available for future mining transaction validation

	// Simulate time passage
	for i := 0; i < 10; i++ {
		sim.Step()
	}

	// Verify configurations persist across blocks
	alicePage = GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	bobPage = GetAccount[*KeyPage](t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"))

	require.Equal(t, premiumDifficulty, alicePage.Keys[0].MiningDifficulty, "Alice's configuration should persist")
	require.Equal(t, standardDifficulty, bobPage.Keys[0].MiningDifficulty, "Bob's configuration should persist")

	t.Log("=== Phase 5: App Updates Mining Policies ===")

	// App decides to disable mining for Bob (perhaps subscription expired)
	UpdateAccount(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), func(page *KeyPage) {
		page.Keys[0].MiningDifficulty = nil // Disable mining
		page.Keys[0].MiningExpiry = 0 // Reset expiry
	})

	// Verify Bob's mining is disabled
	bobPage = GetAccount[*KeyPage](t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"))
	require.Nil(t, bobPage.Keys[0].MiningDifficulty, "Bob's mining should be disabled")
	require.Equal(t, uint64(0), bobPage.Keys[0].MiningExpiry, "Bob's expiry should be reset")

	// Verify Alice's mining is still enabled
	alicePage = GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.Equal(t, premiumDifficulty, alicePage.Keys[0].MiningDifficulty, "Alice's mining should remain enabled")

	t.Log("=== Test Complete: Mining E2E workflow successful ===")
}

func TestMiningE2E_ExpiryHandling(t *testing.T) {
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	// Initialize simulator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Create identity
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.CreditBalance = 1e9
	})

	t.Log("=== Phase 1: Set Short-Term Mining Permission ===")

	// Set mining with short expiry (simulating near-future expiry)
	difficulty := make([]byte, 32)
	copy(difficulty, []byte("short-term-mining"))
	// Set expiry to a "past" timestamp to simulate expiry
	expiredTime := uint64(1000000) // This represents an expired timestamp

	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.Keys[0].MiningDifficulty = difficulty
		page.Keys[0].MiningExpiry = expiredTime
	})

	// Verify mining permission was set
	page := GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.Equal(t, difficulty, page.Keys[0].MiningDifficulty)
	require.Equal(t, expiredTime, page.Keys[0].MiningExpiry)

	t.Log("=== Phase 2: Extend Mining Permission ===")

	// Extend mining permission to future
	futureExpiry := uint64(9999999999) // Far future timestamp

	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.Keys[0].MiningExpiry = futureExpiry
	})

	// Verify expiry was extended, difficulty unchanged
	page = GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.Equal(t, difficulty, page.Keys[0].MiningDifficulty, "Difficulty should be unchanged")
	require.Equal(t, futureExpiry, page.Keys[0].MiningExpiry, "Expiry should be extended")

	t.Log("=== Phase 3: Remove Expiry (Permanent Mining) ===")

	// Set expiry to 0 for permanent mining
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(page *KeyPage) {
		page.Keys[0].MiningExpiry = 0
	})

	// Verify permanent mining (no expiry)
	page = GetAccount[*KeyPage](t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"))
	require.Equal(t, difficulty, page.Keys[0].MiningDifficulty, "Difficulty should be unchanged")
	require.Equal(t, uint64(0), page.Keys[0].MiningExpiry, "Should have no expiry (permanent)")

	t.Log("=== Test Complete: Expiry handling workflow successful ===")
}

func TestMiningE2E_MultiTierMiningApp(t *testing.T) {
	// Simulate a sophisticated mining app with multiple tiers
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.Genesis(GenesisTime),
	)

	// Define mining tiers
	type MiningTier struct {
		Name       string
		Difficulty []byte
		Duration   uint64 // Days
	}

	tiers := []MiningTier{
		{
			Name:       "Free",
			Difficulty: func() []byte { d := make([]byte, 32); copy(d[:4], []byte{0x00, 0x00, 0x00, 0x0F}); return d }(), // Hardest
			Duration:   3, // 3 days
		},
		{
			Name:       "Premium",
			Difficulty: func() []byte { d := make([]byte, 32); copy(d[:4], []byte{0x00, 0x00, 0x0F, 0xFF}); return d }(), // Medium
			Duration:   30, // 30 days
		},
		{
			Name:       "Enterprise",
			Difficulty: func() []byte { d := make([]byte, 32); copy(d[:4], []byte{0x00, 0x0F, 0xFF, 0xFF}); return d }(), // Easiest
			Duration:   0, // No expiry
		},
	}

	// Create users for each tier
	users := make(map[string]*url.URL)
	userKeys := make(map[string][]byte)
	
	for _, tier := range tiers {
		userName := fmt.Sprintf("user-%s", tier.Name)
		userUrl := AccountUrl(userName)
		userKey := acctesting.GenerateKey(userUrl)

		users[tier.Name] = userUrl
		userKeys[tier.Name] = userKey

		// Create user identity
		MakeIdentity(t, sim.DatabaseFor(userUrl), userUrl, userKey[32:])

		// Add credits
		UpdateAccount(t, sim.DatabaseFor(userUrl), userUrl.JoinPath("book", "1"), func(page *KeyPage) {
			page.CreditBalance = 1e9
		})
	}

	t.Log("=== Applying Mining Tier Configurations ===")

	// Apply tier configurations
	for _, tier := range tiers {
		userUrl := users[tier.Name]

		var expiry *uint64
		if tier.Duration > 0 {
			expiryTime := uint64(time.Now().Unix() + int64(tier.Duration*24*3600))
			expiry = &expiryTime
		} else {
			// Enterprise tier - no expiry
			expiryTime := uint64(0)
			expiry = &expiryTime
		}

		UpdateAccount(t, sim.DatabaseFor(userUrl), userUrl.JoinPath("book", "1"), func(page *KeyPage) {
			page.Keys[0].MiningDifficulty = tier.Difficulty
			if expiry != nil {
				page.Keys[0].MiningExpiry = *expiry
			} else {
				page.Keys[0].MiningExpiry = 0
			}
		})

		t.Logf("Applied %s tier to %s", tier.Name, userUrl)
	}

	t.Log("=== Verifying Tier Configurations ===")

	// Verify each tier has correct configuration
	for _, tier := range tiers {
		userUrl := users[tier.Name]
		page := GetAccount[*KeyPage](t, sim.DatabaseFor(userUrl), userUrl.JoinPath("book", "1"))
		
		require.Equal(t, tier.Difficulty, page.Keys[0].MiningDifficulty, 
			"User %s should have %s tier difficulty", userUrl, tier.Name)

		if tier.Duration == 0 {
			require.Equal(t, uint64(0), page.Keys[0].MiningExpiry,
				"Enterprise user should have no expiry")
		} else {
			require.NotEqual(t, uint64(0), page.Keys[0].MiningExpiry,
				"User %s should have expiry set", userUrl)
		}

		t.Logf("✓ %s tier verified for %s", tier.Name, userUrl)
	}

	t.Log("=== Test Complete: Multi-tier mining app workflow successful ===")
}