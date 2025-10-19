package e2e

import (
	"crypto/sha256"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestMiningValidator_CoreFunctionality(t *testing.T) {
	t.Run("InitializeEpoch", func(t *testing.T) {
		validator := chain.NewMiningValidator(chain.DefaultMiningValidatorConfig())
		
		// Test epoch initialization
		epochNumber := uint64(1)
		baselineTarget := make([]byte, 32)
		baselineTarget[31] = 0xFF // Easy target for testing
		dnAnchorHash := sha256.Sum256([]byte("test-dn-anchor"))
		submissionWindow := [2]uint64{100, 200}
		
		err := validator.InitializeEpoch(epochNumber, baselineTarget, dnAnchorHash[:], submissionWindow)
		require.NoError(t, err)
		
		// Verify epoch statistics
		stats := validator.GetEpochStatistics()
		require.Equal(t, epochNumber, stats.EpochNumber)
		require.Equal(t, baselineTarget, stats.BaselineTarget)
		require.Equal(t, dnAnchorHash[:], stats.DNAnchorHash)
		require.Equal(t, submissionWindow, stats.SubmissionWindow)
	})
	
	t.Run("ValidateAndSubmit_ValidSubmission", func(t *testing.T) {
		validator := chain.NewMiningValidator(chain.DefaultMiningValidatorConfig())
		
		// Initialize epoch
		epochNumber := uint64(1)
		baselineTarget := make([]byte, 32)
		for i := range baselineTarget {
			baselineTarget[i] = 0xFF // Very easy target
		}
		dnAnchorHash := sha256.Sum256([]byte("test-dn-anchor"))
		submissionWindow := [2]uint64{100, 200}
		
		err := validator.InitializeEpoch(epochNumber, baselineTarget, dnAnchorHash[:], submissionWindow)
		require.NoError(t, err)
		
		// Create valid mining transaction
		minerADI, _ := url.Parse("acc://alice.acme")
		miningTx := &protocol.MiningTransaction{
			BoundNonce:      createValidBoundNonce(minerADI),
			TransactionData: []byte("test-transaction-data"),
			BlockHash:       dnAnchorHash[:],
			BaselineTarget:  baselineTarget,
			MinerADI:        minerADI,
			Timestamp:       uint64(time.Now().Unix()),
			EpochNumber:     epochNumber,
		}
		
		// Submit to validator
		result, err := validator.ValidateAndSubmit(miningTx)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.True(t, result.IsAccepted)
		require.NotEmpty(t, result.SubmissionHash)
		require.NotEmpty(t, result.ComputedHash)
	})
	
	t.Run("ValidateAndSubmit_InvalidEpoch", func(t *testing.T) {
		validator := chain.NewMiningValidator(chain.DefaultMiningValidatorConfig())
		
		// Initialize epoch 1
		epochNumber := uint64(1)
		baselineTarget := make([]byte, 32)
		baselineTarget[31] = 0xFF
		dnAnchorHash := sha256.Sum256([]byte("test-dn-anchor"))
		submissionWindow := [2]uint64{100, 200}
		
		err := validator.InitializeEpoch(epochNumber, baselineTarget, dnAnchorHash[:], submissionWindow)
		require.NoError(t, err)
		
		// Create mining transaction for wrong epoch
		minerADI, _ := url.Parse("acc://alice.acme")
		miningTx := &protocol.MiningTransaction{
			BoundNonce:      createValidBoundNonce(minerADI),
			TransactionData: []byte("test-transaction-data"),
			BlockHash:       dnAnchorHash[:],
			BaselineTarget:  baselineTarget,
			MinerADI:        minerADI,
			Timestamp:       uint64(time.Now().Unix()),
			EpochNumber:     epochNumber + 1, // Wrong epoch
		}
		
		// Submit to validator
		result, err := validator.ValidateAndSubmit(miningTx)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.IsAccepted)
		require.Contains(t, result.ErrorMessage, "invalid epoch number")
	})
}

func TestMiningPriorityQueue_Operations(t *testing.T) {
	t.Run("InsertAndRetrieve", func(t *testing.T) {
		pq := chain.NewMiningPriorityQueue(3) // Top-3 queue
		
		// Create submissions with different hash qualities
		submissions := []*chain.MiningSubmission{
			createTestSubmission("acc://alice.acme", []byte{0x11, 0x11}),   // Good hash
			createTestSubmission("acc://bob.acme", []byte{0x22, 0x22}),     // Medium hash
			createTestSubmission("acc://charlie.acme", []byte{0x33, 0x33}), // Worse hash
			createTestSubmission("acc://david.acme", []byte{0x00, 0x01}),   // Best hash
			createTestSubmission("acc://eve.acme", []byte{0x44, 0x44}),     // Worst hash
		}
		
		// Insert all submissions
		for _, sub := range submissions {
			accepted := pq.InsertOrReplace(sub)
			if sub.MinerADI.String() == "acc://eve.acme" {
				require.False(t, accepted) // Should be rejected (worst hash, queue full)
			} else {
				require.True(t, accepted)
			}
		}
		
		// Verify queue size
		require.Equal(t, uint64(3), pq.Size())
		require.True(t, pq.IsFull())
		
		// Get top submissions
		topSubmissions := pq.GetTopN()
		require.Len(t, topSubmissions, 3)
		
		// Verify ordering (best hash first)
		require.Equal(t, "acc://david.acme", topSubmissions[0].MinerADI.String())
		require.Equal(t, uint64(1), topSubmissions[0].Rank)
		require.Equal(t, "acc://alice.acme", topSubmissions[1].MinerADI.String())
		require.Equal(t, uint64(2), topSubmissions[1].Rank)
		require.Equal(t, "acc://bob.acme", topSubmissions[2].MinerADI.String())
		require.Equal(t, uint64(3), topSubmissions[2].Rank)
	})
	
	t.Run("Statistics", func(t *testing.T) {
		pq := chain.NewMiningPriorityQueue(2)
		
		// Add valid submissions
		validSub1 := createTestSubmission("acc://alice.acme", []byte{0x11, 0x11})
		validSub2 := createTestSubmission("acc://bob.acme", []byte{0x22, 0x22})
		
		// Add invalid submission
		invalidSub := createTestSubmission("acc://charlie.acme", []byte{0x33, 0x33})
		invalidSub.IsValid = false
		
		pq.InsertOrReplace(validSub1)
		pq.InsertOrReplace(validSub2)
		pq.InsertOrReplace(invalidSub)
		
		stats := pq.GetStatistics()
		require.Equal(t, uint64(3), stats.TotalSubmitted)
		require.Equal(t, uint64(2), stats.TotalValid)
		require.Equal(t, uint64(2), stats.CurrentSize)
		require.True(t, stats.IsFull)
		require.NotNil(t, stats.BestHash)
		require.NotNil(t, stats.WorstHash)
	})
}

func TestMiningRewardDistribution(t *testing.T) {
	t.Run("EqualDistribution", func(t *testing.T) {
		baseReward := big.NewInt(1000)
		distributor := chain.NewMiningRewardDistributor(baseReward, chain.EqualDistribution)
		
		// Create test winners
		winners := []*chain.MiningSubmission{
			createTestSubmission("acc://alice.acme", []byte{0x11, 0x11}),
			createTestSubmission("acc://bob.acme", []byte{0x22, 0x22}),
			createTestSubmission("acc://charlie.acme", []byte{0x33, 0x33}),
		}
		
		// Set ranks
		for i, winner := range winners {
			winner.Rank = uint64(i + 1)
		}
		
		// Calculate rewards
		payouts, err := distributor.CalculateRewards(winners, nil)
		require.NoError(t, err)
		require.Len(t, payouts, 3)
		
		// Verify equal distribution
		for _, payout := range payouts {
			require.Equal(t, baseReward, payout.Amount)
			require.Equal(t, "mining", payout.RewardType)
		}
	})
	
	t.Run("ProportionalDistribution", func(t *testing.T) {
		baseReward := big.NewInt(1000)
		distributor := chain.NewMiningRewardDistributor(baseReward, chain.ProportionalByHashQuality)
		
		// Create test winners with different hash qualities
		winners := []*chain.MiningSubmission{
			createTestSubmission("acc://alice.acme", []byte{0x10, 0x00}), // Best hash
			createTestSubmission("acc://bob.acme", []byte{0x20, 0x00}),   // Medium hash
			createTestSubmission("acc://charlie.acme", []byte{0x30, 0x00}), // Worst hash
		}
		
		// Set ranks
		for i, winner := range winners {
			winner.Rank = uint64(i + 1)
		}
		
		// Calculate rewards
		payouts, err := distributor.CalculateRewards(winners, nil)
		require.NoError(t, err)
		require.Len(t, payouts, 3)
		
		// Verify proportional distribution
		// Better hashes should get higher rewards
		aliceReward := findPayoutForMiner(payouts, "acc://alice.acme").Amount
		bobReward := findPayoutForMiner(payouts, "acc://bob.acme").Amount
		charlieReward := findPayoutForMiner(payouts, "acc://charlie.acme").Amount
		
		require.True(t, aliceReward.Cmp(bobReward) > 0, "Alice (better hash) should get more reward than Bob")
		require.True(t, bobReward.Cmp(charlieReward) > 0, "Bob should get more reward than Charlie")
	})
	
	t.Run("TieredDistribution", func(t *testing.T) {
		baseReward := big.NewInt(1000)
		distributor := chain.NewMiningRewardDistributor(baseReward, chain.TieredByRanking)
		
		// Create test winners
		winners := []*chain.MiningSubmission{
			createTestSubmission("acc://alice.acme", []byte{0x11, 0x11}),   // Rank 1
			createTestSubmission("acc://bob.acme", []byte{0x22, 0x22}),     // Rank 2
			createTestSubmission("acc://charlie.acme", []byte{0x33, 0x33}), // Rank 3
			createTestSubmission("acc://david.acme", []byte{0x44, 0x44}),   // Rank 4
		}
		
		// Set ranks
		for i, winner := range winners {
			winner.Rank = uint64(i + 1)
		}
		
		// Calculate rewards
		payouts, err := distributor.CalculateRewards(winners, nil)
		require.NoError(t, err)
		require.Len(t, payouts, 4)
		
		// Verify tiered distribution
		aliceReward := findPayoutForMiner(payouts, "acc://alice.acme").Amount
		bobReward := findPayoutForMiner(payouts, "acc://bob.acme").Amount
		charlieReward := findPayoutForMiner(payouts, "acc://charlie.acme").Amount
		davidReward := findPayoutForMiner(payouts, "acc://david.acme").Amount
		
		// Rank 1 should get 2x base reward (2000)
		expectedAliceReward := big.NewInt(2000)
		require.Equal(t, expectedAliceReward, aliceReward)
		
		// Rank 2 should get 1.5x base reward (1500)
		expectedBobReward := big.NewInt(1500)
		require.Equal(t, expectedBobReward, bobReward)
		
		// Rank 3 should get 1.2x base reward (1200)
		expectedCharlieReward := big.NewInt(1200)
		require.Equal(t, expectedCharlieReward, charlieReward)
		
		// Rank 4+ should get 1x base reward (1000)
		require.Equal(t, baseReward, davidReward)
	})
}

func TestMiningValidator_Integration(t *testing.T) {
	t.Run("EndToEndEpochProcessing", func(t *testing.T) {
		config := chain.DefaultMiningValidatorConfig()
		config.TopNSize = 3
		validator := chain.NewMiningValidator(config)
		
		// Initialize epoch
		epochNumber := uint64(1)
		baselineTarget := make([]byte, 32)
		for i := range baselineTarget {
			baselineTarget[i] = 0xFF // Very easy target
		}
		dnAnchorHash := sha256.Sum256([]byte("test-dn-anchor"))
		submissionWindow := [2]uint64{100, 200}
		
		err := validator.InitializeEpoch(epochNumber, baselineTarget, dnAnchorHash[:], submissionWindow)
		require.NoError(t, err)
		
		// Submit multiple mining transactions
		miners := []string{"alice", "bob", "charlie", "david", "eve"}
		for i, minerName := range miners {
			minerADI, _ := url.Parse("acc://" + minerName + ".acme")
			
			// Create different hash qualities by varying transaction data
			transactionData := []byte("transaction-data-" + minerName)
			
			miningTx := &protocol.MiningTransaction{
				BoundNonce:      createValidBoundNonce(minerADI),
				TransactionData: transactionData,
				BlockHash:       dnAnchorHash[:],
				BaselineTarget:  baselineTarget,
				MinerADI:        minerADI,
				Timestamp:       uint64(time.Now().Unix()) + uint64(i),
				EpochNumber:     epochNumber,
			}
			
			result, err := validator.ValidateAndSubmit(miningTx)
			require.NoError(t, err)
			require.NotNil(t, result)
			
			// First 3 should be accepted, others might not be (depending on hash quality)
			if i < 3 {
				require.True(t, result.IsAccepted, "Submission %d should be accepted", i)
			}
		}
		
		// Get epoch statistics
		stats := validator.GetEpochStatistics()
		require.Equal(t, uint64(5), stats.TotalSubmissions)
		require.LessOrEqual(t, stats.ValidSubmissions, uint64(5))
		require.LessOrEqual(t, stats.CurrentTopN, uint64(3))
		
		// Get top winners
		winners := validator.GetTopNWinners()
		require.LessOrEqual(t, len(winners), 3)
		
		// Process epoch rewards
		issuanceAccount, _ := url.Parse("acc://mining.acme/issuance")
		rewardDistributor := chain.NewMiningRewardDistributor(big.NewInt(1000), chain.EqualDistribution)
		
		rewardResult, err := validator.ProcessEpochRewards(issuanceAccount, rewardDistributor)
		require.NoError(t, err)
		require.NotNil(t, rewardResult)
		require.Equal(t, epochNumber, rewardResult.EpochNumber)
		require.Equal(t, uint64(len(winners)), rewardResult.WinnerCount)
		require.NotNil(t, rewardResult.SyntheticTransactions)
		require.Len(t, rewardResult.SyntheticTransactions, len(winners))
	})
}

// Helper functions

func createValidBoundNonce(minerADI *url.URL) []byte {
	nonce := []byte("test-nonce-12345")
	adiHash := sha256.Sum256([]byte(minerADI.String()))
	return append(nonce, adiHash[:]...)
}

func createTestSubmission(minerADIStr string, hashBytes []byte) *chain.MiningSubmission {
	minerADI, _ := url.Parse(minerADIStr)
	
	// Pad hash to 32 bytes
	computedHash := make([]byte, 32)
	copy(computedHash, hashBytes)
	
	return &chain.MiningSubmission{
		MinerADI:        minerADI,
		SubmissionHash:  sha256.Sum256([]byte(minerADIStr + "submission"))[:],
		BoundNonce:      createValidBoundNonce(minerADI),
		TransactionData: []byte("test-data-" + minerADIStr),
		ComputedHash:    computedHash,
		IsValid:         true,
		EpochNumber:     1,
		Timestamp:       uint64(time.Now().Unix()),
	}
}

func findPayoutForMiner(payouts []*chain.RewardPayout, minerADIStr string) *chain.RewardPayout {
	for _, payout := range payouts {
		if payout.MinerADI.String() == minerADIStr {
			return payout
		}
	}
	return nil
}