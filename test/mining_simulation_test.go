// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() {
	acctesting.EnableDebugFeatures()
}

// TestLxrMiningSimulation tests the full workflow of LXR mining, including reward distribution
// and transaction forwarding using a direct simulation approach.
func TestLxrMiningSimulation(t *testing.T) {
	// Initialize miners
	miner1 := url.MustParse("acc://miner1")
	miner2 := url.MustParse("acc://miner2")
	miner3 := url.MustParse("acc://miner3")

	// Create a block hash to mine against
	blockHash := [32]byte{}
	copy(blockHash[:], acctesting.GenerateKey("blockhash"))

	// Create a hasher for mining
	lxr.IsTestEnvironment = true // Use small hash table for testing
	hasher := lxr.NewHasher()

	// Track rewards distributed
	rewardsDistributed := make(map[string]uint64)

	// Create a reward distributor
	rewardDistributor := mining.NewDefaultRewardDistributor(func(signer *url.URL, amount uint64) error {
		// Record the reward distribution
		rewardsDistributed[signer.String()] += amount
		return nil
	})

	// Track transactions forwarded
	txsForwarded := 0

	// Create a transaction forwarder
	forwarder := mining.NewDefaultTransactionForwarder(func(bh [32]byte, submissions []*mining.MiningSubmission) error {
		// Record the transaction forwarding
		txsForwarded += len(submissions)
		return nil
	})

	// Create a validator with 60 second window duration and queue capacity of 10
	validator := mining.NewValidator(60, 10)
	
	// Start a new mining window
	validator.StartNewWindow(blockHash, 100) // 100 is the minimum difficulty

	// Set the reward distributor and transaction forwarder
	validator.SetRewardDistributor(rewardDistributor)
	validator.SetTransactionForwarder(forwarder)

	// Simulate mining process
	miners := []*url.URL{miner1, miner2, miner3}
	for i, miner := range miners {
		// Create a unique nonce for each miner
		nonce := []byte("test nonce " + miner.String())
		
		// Calculate the proof of work
		computedHash, _ := hasher.CalculatePow(blockHash[:], nonce)
		
		// Create a valid mining signature
		validSig := &protocol.LxrMiningSignature{
			Nonce:         nonce,
			ComputedHash:  computedHash,
			BlockHash:     blockHash,
			Signer:        miner,
			SignerVersion: uint64(i + 1),
			Timestamp:     uint64(time.Now().Unix()),
		}
		
		// Submit the signature
		accepted, err := validator.SubmitSignature(validSig)
		require.NoError(t, err, "Failed to submit signature for %s", miner)
		require.True(t, accepted, "Signature for %s should be accepted", miner)
	}

	// Close the mining window and distribute rewards
	submissions, err := validator.CloseWindowAndDistributeRewards(1000) // 1000 tokens as reward
	require.NoError(t, err, "Failed to close window and distribute rewards")

	// Verify the number of submissions
	require.Equal(t, len(miners), len(submissions), "Expected %d submissions, got %d", len(miners), len(submissions))

	// Verify rewards were distributed to miners
	for _, miner := range miners {
		reward := rewardsDistributed[miner.String()]
		require.Greater(t, reward, uint64(0), "Miner %s should have received rewards", miner)
		t.Logf("Miner %s received %d tokens", miner, reward)
	}

	// Verify that transactions were forwarded
	require.Equal(t, len(miners), txsForwarded, "Expected %d transactions forwarded, got %d", len(miners), txsForwarded)

	// Log the total rewards distributed
	totalRewards := uint64(0)
	for _, reward := range rewardsDistributed {
		totalRewards += reward
	}
	require.Equal(t, uint64(1000), totalRewards, "Total rewards should be 1000, got %d", totalRewards)
}
