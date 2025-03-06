// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/mining"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	fmt.Println("Starting LXR Mining Simulation")

	// Initialize miners
	miner1 := url.MustParse("acc://miner1")
	miner2 := url.MustParse("acc://miner2")
	miner3 := url.MustParse("acc://miner3")
	miners := []*url.URL{miner1, miner2, miner3}

	// Create a block hash to mine against
	blockHash := [32]byte{1, 2, 3, 4, 5} // Simple example hash

	// Create a hasher for mining
	lxr.IsTestEnvironment = true // Use small hash table for testing
	hasher := lxr.NewHasher()

	// Track rewards distributed
	rewardsDistributed := make(map[string]uint64)

	// Create a reward distributor
	rewardDistributor := mining.NewDefaultRewardDistributor(func(signer *url.URL, amount uint64) error {
		// Record the reward distribution
		rewardsDistributed[signer.String()] += amount
		fmt.Printf("Distributed %d tokens to %s\n", amount, signer)
		return nil
	})

	// Track transactions forwarded
	txsForwarded := 0

	// Create a transaction forwarder
	forwarder := mining.NewDefaultTransactionForwarder(func(bh [32]byte, submissions []*mining.MiningSubmission) error {
		// Record the transaction forwarding
		txsForwarded += len(submissions)
		fmt.Printf("Forwarded %d transactions\n", len(submissions))
		return nil
	})

	// Create a validator with 60 second window duration and queue capacity of 10
	validator := mining.NewValidator(60, 10)

	// Set the reward distributor and transaction forwarder
	validator.SetRewardDistributor(rewardDistributor)
	validator.SetTransactionForwarder(forwarder)

	// Start a new mining window
	validator.StartNewWindow(blockHash, 100) // 100 is the minimum difficulty

	// Simulate mining process
	fmt.Println("\n=== Mining Process ===")
	for i, miner := range miners {
		// Create a unique nonce for each miner
		nonce := []byte(fmt.Sprintf("test nonce %s", miner))
		
		// Calculate the proof of work using the LXR hasher
		computedHash, difficulty := hasher.CalculatePow(blockHash[:], nonce)
		fmt.Printf("Miner %s calculated hash with difficulty: %d\n", miner, difficulty)
		
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
		if err != nil {
			fmt.Printf("Error submitting signature for %s: %v\n", miner, err)
			continue
		}
		fmt.Printf("Miner %s submitted signature with nonce: %s (Accepted: %v)\n", miner, nonce, accepted)
	}

	// Close the mining window and distribute rewards
	fmt.Println("\n=== Closing Mining Window and Distributing Rewards ===")
	submissions, err := validator.CloseWindowAndDistributeRewards(1000) // 1000 tokens as reward
	if err != nil {
		fmt.Printf("Error closing window and distributing rewards: %v\n", err)
		return
	}

	// Print the number of submissions
	fmt.Printf("\nTotal submissions: %d\n", len(submissions))

	// Print rewards distributed to miners
	fmt.Println("\n=== Rewards Distribution ===")
	totalRewards := uint64(0)
	for _, miner := range miners {
		reward := rewardsDistributed[miner.String()]
		fmt.Printf("Miner %s received %d tokens\n", miner, reward)
		totalRewards += reward
	}
	fmt.Printf("Total rewards distributed: %d tokens\n", totalRewards)

	// Print transaction forwarding results
	fmt.Println("\n=== Transaction Forwarding ===")
	fmt.Printf("Total transactions forwarded: %d\n", txsForwarded)

	// Verify the simulation was successful
	fmt.Println("\n=== Simulation Results ===")
	// Due to integer division, we might get 999 instead of 1000 tokens distributed
	// Consider it a success if we're within 1 token of the expected total
	if len(submissions) == len(miners) && txsForwarded == len(miners) && (totalRewards >= 999 && totalRewards <= 1000) {
		fmt.Println("✅ Simulation completed successfully!")
		if totalRewards != 1000 {
			fmt.Printf("  Note: %d tokens distributed instead of 1000 due to integer division\n", totalRewards)
		}
	} else {
		fmt.Println("❌ Simulation had issues:")
		if len(submissions) != len(miners) {
			fmt.Printf("  - Expected %d submissions, got %d\n", len(miners), len(submissions))
		}
		if txsForwarded != len(miners) {
			fmt.Printf("  - Expected %d transactions forwarded, got %d\n", len(miners), txsForwarded)
		}
		if totalRewards < 999 || totalRewards > 1000 {
			fmt.Printf("  - Expected 1000 total rewards, got %d\n", totalRewards)
		}
	}
}
