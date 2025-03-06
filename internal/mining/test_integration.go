// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package mining

import (
	"fmt"
	"sync"
)

// TestRewardDistribution tests the reward distribution functionality
func TestRewardDistribution() error {
	// Create a reward distributor
	rewardsDistributed := make(map[string]uint64)
	mu := sync.Mutex{}

	// Create a transfer function that records the rewards
	transferFunc := func(signer string, amount uint64) error {
		mu.Lock()
		defer mu.Unlock()
		rewardsDistributed[signer] += amount
		return nil
	}

	// Create a simple reward distributor
	distributor := &SimpleRewardDistributor{transferFunc: transferFunc}

	// Create some test submissions
	submissions := []*SimpleMiningSubmission{
		{Signer: "acc://test/miner1", Difficulty: 100},
		{Signer: "acc://test/miner2", Difficulty: 200},
		{Signer: "acc://test/miner3", Difficulty: 300},
	}

	// Distribute rewards
	totalReward := uint64(600)
	err := distributor.DistributeRewards(submissions, totalReward)
	if err != nil {
		return fmt.Errorf("failed to distribute rewards: %w", err)
	}

	// Check that all rewards were distributed
	mu.Lock()
	defer mu.Unlock()
	
	var totalDistributed uint64
	for _, amount := range rewardsDistributed {
		totalDistributed += amount
	}
	
	if totalDistributed != totalReward {
		return fmt.Errorf("total distributed rewards %d does not match expected %d", totalDistributed, totalReward)
	}

	// Print the distribution
	fmt.Println("Reward distribution:")
	for signer, amount := range rewardsDistributed {
		fmt.Printf("  %s: %d\n", signer, amount)
	}

	return nil
}

// TestTransactionForwarding tests the transaction forwarding functionality
func TestTransactionForwarding() error {
	// Create a transaction forwarder
	txsForwarded := 0
	mu := sync.Mutex{}

	// Create a forward function that counts the forwarded transactions
	forwardFunc := func(blockHash string, submissions []*SimpleMiningSubmission) error {
		mu.Lock()
		defer mu.Unlock()
		txsForwarded += len(submissions)
		return nil
	}

	// Create a transaction forwarder
	forwarder := &SimpleTransactionForwarder{forwardFunc: forwardFunc}

	// Create some test submissions
	submissions := []*SimpleMiningSubmission{
		{Signer: "acc://test/miner1", Difficulty: 100},
		{Signer: "acc://test/miner2", Difficulty: 200},
		{Signer: "acc://test/miner3", Difficulty: 300},
	}

	// Forward transactions
	blockHash := "test-block-hash"
	err := forwarder.ForwardTransaction(blockHash, submissions)
	if err != nil {
		return fmt.Errorf("failed to forward transactions: %w", err)
	}

	// Check that all transactions were forwarded
	mu.Lock()
	defer mu.Unlock()
	
	if txsForwarded != len(submissions) {
		return fmt.Errorf("forwarded %d transactions, expected %d", txsForwarded, len(submissions))
	}

	fmt.Printf("Successfully forwarded %d transactions\n", txsForwarded)
	return nil
}

// SimpleMiningSubmission is a simplified version of MiningSubmission for testing
type SimpleMiningSubmission struct {
	Signer     string
	Difficulty uint64
}

// SimpleRewardDistributor is a simplified reward distributor for testing
type SimpleRewardDistributor struct {
	transferFunc func(signer string, amount uint64) error
	mu          sync.Mutex
}

// DistributeRewards distributes rewards equally among submissions
func (d *SimpleRewardDistributor) DistributeRewards(submissions []*SimpleMiningSubmission, totalReward uint64) error {
	if len(submissions) == 0 {
		return fmt.Errorf("no submissions to distribute rewards to")
	}

	// Calculate reward per submission
	rewardPerSubmission := totalReward / uint64(len(submissions))
	remaining := totalReward - (rewardPerSubmission * uint64(len(submissions)))

	// Distribute rewards
	for i, submission := range submissions {
		reward := rewardPerSubmission
		if i == 0 {
			reward += remaining // Add any remaining tokens to the first submission
		}

		d.mu.Lock()
		transferFunc := d.transferFunc
		d.mu.Unlock()

		if transferFunc != nil {
			err := transferFunc(submission.Signer, reward)
			if err != nil {
				return fmt.Errorf("failed to transfer reward: %w", err)
			}
		}
	}

	return nil
}

// SimpleTransactionForwarder is a simplified transaction forwarder for testing
type SimpleTransactionForwarder struct {
	forwardFunc func(blockHash string, submissions []*SimpleMiningSubmission) error
	mu          sync.Mutex
}

// ForwardTransaction forwards transactions
func (f *SimpleTransactionForwarder) ForwardTransaction(blockHash string, submissions []*SimpleMiningSubmission) error {
	f.mu.Lock()
	forwardFunc := f.forwardFunc
	f.mu.Unlock()

	if forwardFunc != nil {
		return forwardFunc(blockHash, submissions)
	}

	return nil
}

// RunIntegrationTests runs all integration tests
func RunIntegrationTests() {
	fmt.Println("Running integration tests...")

	// Test reward distribution
	fmt.Println("\nTesting reward distribution:")
	err := TestRewardDistribution()
	if err != nil {
		fmt.Printf("Reward distribution test failed: %v\n", err)
	} else {
		fmt.Println("Reward distribution test passed!")
	}

	// Test transaction forwarding
	fmt.Println("\nTesting transaction forwarding:")
	err = TestTransactionForwarding()
	if err != nil {
		fmt.Printf("Transaction forwarding test failed: %v\n", err)
	} else {
		fmt.Println("Transaction forwarding test passed!")
	}
}
