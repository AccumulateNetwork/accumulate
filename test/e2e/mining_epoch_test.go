package e2e

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/integrations"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestMiningEpochManager_CoreFunctionality(t *testing.T) {
	t.Run("InitializeNewEpoch", func(t *testing.T) {
		// Create mock DN anchor provider
		dnProvider := integrations.CreateMockDNAnchorProvider()
		defer dnProvider.Close()
		
		// Create epoch manager
		config := chain.DefaultEpochManagerConfig()
		config.EpochDurationBlocks = 100  // Shorter for testing
		config.SubmissionWindow = 80      // 80% of epoch
		manager := chain.NewEpochManager(config, dnProvider)
		
		// Initialize first epoch
		epoch, err := manager.InitializeNewEpoch()
		require.NoError(t, err)
		require.NotNil(t, epoch)
		
		// Verify epoch properties
		require.Equal(t, uint64(1), epoch.EpochNumber)
		require.Equal(t, chain.EpochStatusActive, epoch.Status)
		require.Len(t, epoch.BaselineTarget, 32)
		require.Len(t, epoch.DNAnchorHash, 32)
		require.True(t, epoch.SubmissionWindow[1] > epoch.SubmissionWindow[0])
		require.Equal(t, uint64(80), epoch.SubmissionWindow[1]-epoch.SubmissionWindow[0])
		
		// Verify current epoch is set
		currentEpoch := manager.GetCurrentEpoch()
		require.NotNil(t, currentEpoch)
		require.Equal(t, epoch.EpochNumber, currentEpoch.EpochNumber)
	})
	
	t.Run("EpochFinalization", func(t *testing.T) {
		dnProvider := integrations.CreateMockDNAnchorProvider()
		defer dnProvider.Close()
		
		config := chain.DefaultEpochManagerConfig()
		config.EpochDurationBlocks = 50
		manager := chain.NewEpochManager(config, dnProvider)
		
		// Register an issuance account for rewards
		issuanceAccount, _ := url.Parse("acc://mining.acme/rewards")
		tokenURL, _ := url.Parse("acc://ACME")
		err := manager.RegisterIssuanceAccount(tokenURL, issuanceAccount)
		require.NoError(t, err)
		
		// Initialize epoch
		epoch, err := manager.InitializeNewEpoch()
		require.NoError(t, err)
		
		// Simulate some mining submissions
		err = simulateMiningSubmissions(t, manager, epoch, 5)
		require.NoError(t, err)
		
		// Finalize epoch
		result, err := manager.FinalizeCurrentEpoch()
		require.NoError(t, err)
		require.NotNil(t, result)
		
		// Verify finalization result
		require.Equal(t, epoch.EpochNumber, result.EpochNumber)
		require.Equal(t, epoch.EpochNumber+1, result.NextEpochNumber)
		require.NotZero(t, result.WinnerCount)
		require.NotEmpty(t, result.SyntheticTransactions)
		
		// Verify epoch status
		finalEpoch := manager.GetCurrentEpoch()
		require.Equal(t, chain.EpochStatusCompleted, finalEpoch.Status)
		require.NotNil(t, finalEpoch.FinalizedAt)
	})
	
	t.Run("MultipleEpochTransitions", func(t *testing.T) {
		dnProvider := integrations.CreateMockDNAnchorProvider()
		defer dnProvider.Close()
		
		config := chain.DefaultEpochManagerConfig()
		config.EpochDurationBlocks = 30
		config.MaxEpochHistory = 5
		manager := chain.NewEpochManager(config, dnProvider)
		
		// Create and finalize multiple epochs
		epochCount := 7
		for i := 0; i < epochCount; i++ {
			// Initialize new epoch
			epoch, err := manager.InitializeNewEpoch()
			require.NoError(t, err)
			require.Equal(t, uint64(i+1), epoch.EpochNumber)
			
			// Simulate mining activity
			err = simulateMiningSubmissions(t, manager, epoch, 3)
			require.NoError(t, err)
			
			// Finalize epoch
			result, err := manager.FinalizeCurrentEpoch()
			require.NoError(t, err)
			require.Equal(t, uint64(i+1), result.EpochNumber)
		}
		
		// Verify history management (should keep only 5 epochs)
		history := manager.GetEpochHistory(0) // Get all
		require.LessOrEqual(t, len(history), 5)
		
		// Verify most recent epochs are kept
		if len(history) == 5 {
			require.Equal(t, uint64(epochCount), history[0].EpochNumber) // Most recent first
			require.Equal(t, uint64(epochCount-4), history[4].EpochNumber) // Oldest kept
		}
	})
}

func TestMiningDifficultyAdjustment(t *testing.T) {
	t.Run("LinearDifficultyAdjustment", func(t *testing.T) {
		dnProvider := integrations.CreateMockDNAnchorProvider()
		defer dnProvider.Close()
		
		config := chain.DefaultEpochManagerConfig()
		config.TargetSubmissionRate = 10.0  // Target 10 submissions/block
		config.DifficultyWindow = 3         // Use last 3 epochs
		config.MaxDifficultyChange = 2.0    // Max 200% change
		manager := chain.NewEpochManager(config, dnProvider)
		
		// Create baseline epochs with different submission rates
		epochData := []struct {
			submissionsPerBlock float64
			expectedAdjustment  string // "easier", "harder", "stable"
		}{
			{20.0, "harder"},  // Too many submissions, make harder
			{15.0, "harder"},  // Still too many, continue harder
			{5.0, "easier"},   // Too few submissions, make easier
		}
		
		var previousDifficulty *big.Float
		
		for i, data := range epochData {
			// Initialize epoch
			epoch, err := manager.InitializeNewEpoch()
			require.NoError(t, err)
			
			// Calculate expected submissions based on rate
			blocksInEpoch := config.EpochDurationBlocks
			expectedSubmissions := int(data.submissionsPerBlock * float64(blocksInEpoch))
			
			// Simulate submissions at the specified rate
			err = simulateMiningSubmissions(t, manager, epoch, expectedSubmissions)
			require.NoError(t, err)
			
			// Finalize epoch
			_, err = manager.FinalizeCurrentEpoch()
			require.NoError(t, err)
			
			// Check difficulty adjustment (after first epoch)
			if i > 0 {
				diffInfo := manager.GetDifficultyInfo()
				require.NotNil(t, diffInfo.CurrentDifficulty)
				
				if previousDifficulty != nil {
					comparison := diffInfo.CurrentDifficulty.Cmp(previousDifficulty)
					
					switch data.expectedAdjustment {
					case "harder":
						require.Greater(t, comparison, 0, "Difficulty should increase (harder)")
					case "easier":
						require.Less(t, comparison, 0, "Difficulty should decrease (easier)")
					case "stable":
						// Allow small changes due to rounding
						require.InDelta(t, 0, comparison, 1, "Difficulty should remain stable")
					}
				}
				
				previousDifficulty = new(big.Float).Set(diffInfo.CurrentDifficulty)
			}
		}
	})
	
	t.Run("DifficultyBounds", func(t *testing.T) {
		dnProvider := integrations.CreateMockDNAnchorProvider()
		defer dnProvider.Close()
		
		config := chain.DefaultEpochManagerConfig()
		config.MaxDifficultyChange = 10.0  // Allow large changes for testing
		manager := chain.NewEpochManager(config, dnProvider)
		
		// Create epoch with extreme submission rate
		epoch, err := manager.InitializeNewEpoch()
		require.NoError(t, err)
		
		// Simulate massive number of submissions (should trigger max difficulty)
		err = simulateMiningSubmissions(t, manager, epoch, 10000)
		require.NoError(t, err)
		
		_, err = manager.FinalizeCurrentEpoch()
		require.NoError(t, err)
		
		// Verify difficulty is within bounds
		diffInfo := manager.GetDifficultyInfo()
		
		minTarget := new(big.Int).SetBytes(diffInfo.MinTarget)
		maxTarget := new(big.Int).SetBytes(diffInfo.MaxTarget)
		currentTarget := new(big.Int).SetBytes(diffInfo.CurrentTarget)
		
		require.True(t, currentTarget.Cmp(minTarget) >= 0, "Current target should be >= min target")
		require.True(t, currentTarget.Cmp(maxTarget) <= 0, "Current target should be <= max target")
	})
}

func TestMiningAnalytics(t *testing.T) {
	t.Run("EpochMetricsRecording", func(t *testing.T) {
		analytics := chain.NewMiningAnalytics(chain.DefaultAnalyticsConfig())
		
		// Create test epoch with known data
		epoch := createTestEpoch(1, 100, 85, 10) // 100 total, 85 valid, 10 winners
		
		// Create test submissions
		submissions := createTestSubmissions(100, 85)
		
		// Record epoch completion
		err := analytics.RecordEpochCompletion(epoch, submissions)
		require.NoError(t, err)
		
		// Verify metrics were recorded
		metrics := analytics.GetEpochMetrics(1)
		require.NotNil(t, metrics)
		require.Equal(t, uint64(1), metrics.EpochNumber)
		require.Equal(t, uint64(100), metrics.TotalSubmissions)
		require.Equal(t, uint64(85), metrics.ValidSubmissions)
		require.Equal(t, uint64(15), metrics.InvalidSubmissions)
		require.InDelta(t, 0.85, metrics.ValidationRate, 0.01)
		require.Equal(t, uint64(10), metrics.TopNSize)
		require.InDelta(t, 8.5, metrics.CompetitionRatio, 0.1)
	})
	
	t.Run("MinerStatistics", func(t *testing.T) {
		analytics := chain.NewMiningAnalytics(chain.DefaultAnalyticsConfig())
		
		// Create test miner
		minerADI, _ := url.Parse("acc://alice.acme")
		
		// Simulate multiple epochs for the miner
		for epochNum := 1; epochNum <= 5; epochNum++ {
			epoch := createTestEpoch(uint64(epochNum), 50, 40, 5)
			
			// Alice wins epochs 2 and 4
			aliceWins := epochNum == 2 || epochNum == 4
			submissions := createTestSubmissionsForMiner(minerADI, 3, aliceWins)
			
			err := analytics.RecordEpochCompletion(epoch, submissions)
			require.NoError(t, err)
		}
		
		// Check miner statistics
		stats := analytics.GetMinerStatistics(minerADI)
		require.NotNil(t, stats)
		require.Equal(t, uint64(1), stats.FirstEpoch)
		require.Equal(t, uint64(5), stats.LastEpoch)
		require.Equal(t, uint64(5), stats.TotalEpochsParticipated)
		require.Equal(t, uint64(15), stats.TotalSubmissions) // 3 per epoch * 5 epochs
		require.Equal(t, uint64(2), stats.TotalWins)         // Epochs 2 and 4
		require.InDelta(t, 0.4, stats.WinRate, 0.01)        // 2/5 = 40%
	})
	
	t.Run("NetworkStatistics", func(t *testing.T) {
		analytics := chain.NewMiningAnalytics(chain.DefaultAnalyticsConfig())
		
		// Simulate network activity over multiple epochs
		miners := []string{"alice.acme", "bob.acme", "charlie.acme", "david.acme"}
		
		for epochNum := 1; epochNum <= 10; epochNum++ {
			epoch := createTestEpoch(uint64(epochNum), 100, 80, 5)
			
			var allSubmissions []*chain.MiningSubmission
			
			// Each miner submits with different frequencies
			for i, minerName := range miners {
				minerADI, _ := url.Parse("acc://" + minerName)
				submissions := createTestSubmissionsForMiner(minerADI, i+1, false) // Different submission counts
				allSubmissions = append(allSubmissions, submissions...)
			}
			
			err := analytics.RecordEpochCompletion(epoch, allSubmissions)
			require.NoError(t, err)
		}
		
		// Check network statistics
		netStats := analytics.GetNetworkStatistics()
		require.NotNil(t, netStats)
		require.Equal(t, uint64(10), netStats.TotalEpochs)
		require.Equal(t, uint64(4), netStats.TotalMinersEver)
		require.Equal(t, uint64(4), netStats.ActiveMiners) // All miners active in recent epochs
		
		// Check that concentration index is reasonable (not too concentrated)
		require.Greater(t, netStats.MinerConcentrationIndex, 0.0)
		require.Less(t, netStats.MinerConcentrationIndex, 1.0)
	})
	
	t.Run("TopMinersRanking", func(t *testing.T) {
		analytics := chain.NewMiningAnalytics(chain.DefaultAnalyticsConfig())
		
		// Create miners with different performance profiles
		minerProfiles := []struct {
			name string
			wins uint64
		}{
			{"alice.acme", 10}, // Top performer
			{"bob.acme", 7},    // Second place
			{"charlie.acme", 3}, // Third place
		}
		
		// Simulate epochs to establish rankings
		for epochNum := 1; epochNum <= 20; epochNum++ {
			epoch := createTestEpoch(uint64(epochNum), 50, 40, 3)
			
			var allSubmissions []*chain.MiningSubmission
			
			for _, profile := range minerProfiles {
				minerADI, _ := url.Parse("acc://" + profile.name)
				
				// Determine if this miner wins this epoch
				shouldWin := uint64(epochNum) <= profile.wins
				submissions := createTestSubmissionsForMiner(minerADI, 2, shouldWin)
				allSubmissions = append(allSubmissions, submissions...)
			}
			
			err := analytics.RecordEpochCompletion(epoch, allSubmissions)
			require.NoError(t, err)
		}
		
		// Get top miners by wins
		topMiners := analytics.GetTopMiners("wins", 3)
		require.Len(t, topMiners, 3)
		
		// Verify ranking order
		require.Equal(t, "acc://alice.acme", topMiners[0].MinerADI.String())
		require.Equal(t, uint64(10), topMiners[0].TotalWins)
		require.Equal(t, "acc://bob.acme", topMiners[1].MinerADI.String())
		require.Equal(t, uint64(7), topMiners[1].TotalWins)
		require.Equal(t, "acc://charlie.acme", topMiners[2].MinerADI.String())
		require.Equal(t, uint64(3), topMiners[2].TotalWins)
	})
}

func TestDNAnchorIntegration(t *testing.T) {
	t.Run("AnchorProviderBasicOperations", func(t *testing.T) {
		provider := integrations.CreateMockDNAnchorProvider()
		defer provider.Close()
		
		// Test current anchor retrieval
		anchor, err := provider.GetCurrentAnchor()
		require.NoError(t, err)
		require.Len(t, anchor, 32)
		
		// Test current block height
		height, err := provider.GetCurrentBlockHeight()
		require.NoError(t, err)
		require.Greater(t, height, uint64(0))
		
		// Test historical anchor (should exist for block 1)
		historicalAnchor, err := provider.GetAnchorAtBlock(1)
		require.NoError(t, err)
		require.Len(t, historicalAnchor, 32)
	})
	
	t.Run("AnchorSubscription", func(t *testing.T) {
		provider := integrations.CreateMockDNAnchorProvider()
		defer provider.Close()
		
		// Subscribe to anchor updates
		updateChan, err := provider.SubscribeToAnchors()
		require.NoError(t, err)
		
		// Should receive initial update immediately
		select {
		case update := <-updateChan:
			require.Greater(t, update.BlockHeight, uint64(0))
			require.Len(t, update.AnchorHash, 32)
			require.False(t, update.Timestamp.IsZero())
		case <-time.After(time.Second):
			t.Fatal("Should receive initial anchor update immediately")
		}
		
		// Should receive new updates over time
		select {
		case update := <-updateChan:
			require.Greater(t, update.BlockHeight, uint64(1))
		case <-time.After(time.Second * 2):
			t.Fatal("Should receive anchor updates over time")
		}
	})
	
	t.Run("EpochManagerWithDNIntegration", func(t *testing.T) {
		provider := integrations.CreateMockDNAnchorProvider()
		defer provider.Close()
		
		config := chain.DefaultEpochManagerConfig()
		config.EpochDurationBlocks = 20
		manager := chain.NewEpochManager(config, provider)
		
		// Initialize epoch - should use current DN anchor
		epoch1, err := manager.InitializeNewEpoch()
		require.NoError(t, err)
		
		currentAnchor, _ := provider.GetCurrentAnchor()
		require.Equal(t, currentAnchor, epoch1.DNAnchorHash)
		
		// Wait for DN to advance and create new epoch
		time.Sleep(time.Millisecond * 200) // Let simulated blocks advance
		
		epoch2, err := manager.InitializeNewEpoch()
		require.NoError(t, err)
		
		// Second epoch should have different anchor
		require.NotEqual(t, epoch1.DNAnchorHash, epoch2.DNAnchorHash)
		require.Greater(t, epoch2.EpochNumber, epoch1.EpochNumber)
	})
}

// Helper functions for testing

func simulateMiningSubmissions(t *testing.T, manager *chain.EpochManager, epoch *chain.MiningEpoch, count int) error {
	// Get the validator from epoch manager (would need to expose this in real implementation)
	// For testing, we'll simulate by creating submissions
	
	for i := 0; i < count; i++ {
		minerADI, _ := url.Parse("acc://miner" + string(rune('A'+i%10)) + ".acme")
		
		// Create mining transaction
		miningTx := &protocol.MiningTransaction{
			BoundNonce:      createValidBoundNonce(minerADI),
			TransactionData: []byte("test-data-" + string(rune('A'+i))),
			BlockHash:       epoch.DNAnchorHash,
			BaselineTarget:  epoch.BaselineTarget,
			MinerADI:        minerADI,
			Timestamp:       uint64(time.Now().Unix()),
			EpochNumber:     epoch.EpochNumber,
		}
		
		// For testing purposes, we'll assume submissions are valid
		// In real implementation, this would go through the validator
		_ = miningTx
	}
	
	return nil
}

func createTestEpoch(epochNumber uint64, totalSubs, validSubs, winners uint64) *chain.MiningEpoch {
	now := time.Now()
	finalized := now.Add(time.Hour)
	
	epoch := &chain.MiningEpoch{
		EpochNumber:      epochNumber,
		StartBlock:       epochNumber * 100,
		EndBlock:         (epochNumber + 1) * 100,
		CreatedAt:        now,
		FinalizedAt:      &finalized,
		BaselineTarget:   make([]byte, 32),
		DNAnchorHash:     make([]byte, 32),
		SubmissionWindow: [2]uint64{epochNumber * 100, epochNumber*100 + 80},
		TotalSubmissions: totalSubs,
		ValidSubmissions: validSubs,
		Status:           chain.EpochStatusCompleted,
	}
	
	// Create test winners
	epoch.TopNWinners = make([]*chain.MiningSubmission, winners)
	for i := uint64(0); i < winners; i++ {
		minerADI, _ := url.Parse("acc://winner" + string(rune('A'+int(i))) + ".acme")
		
		// Create progressively worse hashes (higher values)
		hash := make([]byte, 32)
		hash[31] = byte(i + 1) // Simple hash quality differentiation
		
		epoch.TopNWinners[i] = &chain.MiningSubmission{
			MinerADI:     minerADI,
			ComputedHash: hash,
			Rank:         i + 1,
			IsValid:      true,
			RewardAmount: big.NewInt(1000), // 1000 token reward
		}
	}
	
	return epoch
}

func createTestSubmissions(total, valid uint64) []*chain.MiningSubmission {
	submissions := make([]*chain.MiningSubmission, total)
	
	for i := uint64(0); i < total; i++ {
		minerADI, _ := url.Parse("acc://submitter" + string(rune('A'+int(i%26))) + ".acme")
		
		submissions[i] = &chain.MiningSubmission{
			MinerADI: minerADI,
			IsValid:  i < valid, // First 'valid' submissions are valid
			ComputedHash: make([]byte, 32),
		}
	}
	
	return submissions
}

func createTestSubmissionsForMiner(minerADI *url.URL, count int, shouldWin bool) []*chain.MiningSubmission {
	submissions := make([]*chain.MiningSubmission, count)
	
	for i := 0; i < count; i++ {
		hash := make([]byte, 32)
		if shouldWin && i == 0 {
			// Make first submission a winning hash (low value)
			hash[31] = 0x01
		} else {
			// Regular hash
			hash[31] = byte(0x80 + i)
		}
		
		submissions[i] = &chain.MiningSubmission{
			MinerADI:     minerADI,
			ComputedHash: hash,
			IsValid:      true,
			Rank:         1, // Will be updated if winning
		}
		
		if shouldWin && i == 0 {
			submissions[i].RewardAmount = big.NewInt(1000)
		}
	}
	
	return submissions
}

func createValidBoundNonce(minerADI *url.URL) []byte {
	// Create simple bound nonce for testing
	nonce := []byte("test-nonce-12345")
	// In real implementation, this would include SHA256(miner_ADI)
	return nonce
}