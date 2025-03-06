// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package mining

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func init() {
	// Set the test environment flag for LXR
	lxr.IsTestEnvironment = true
}

func TestDefaultRewardDistributor_DistributeRewards(t *testing.T) {
	// Track rewards distributed
	rewardsDistributed := make(map[string]uint64)
	
	// Create a transfer function that records the rewards
	transferFunc := func(signer *url.URL, amount uint64) error {
		rewardsDistributed[signer.String()] = amount
		return nil
	}
	
	// Create a reward distributor
	distributor := NewDefaultRewardDistributor(transferFunc)
	
	// Create some test submissions
	submissions := []*MiningSubmission{
		{Difficulty: 100, Signer: "acc://signer1"},
		{Difficulty: 200, Signer: "acc://signer2"},
		{Difficulty: 150, Signer: "acc://signer3"},
	}
	
	// Distribute rewards
	err := distributor.DistributeRewards(submissions, 900)
	if err != nil {
		t.Fatalf("Error distributing rewards: %v", err)
	}
	
	// Check that each signer received the correct amount (equal distribution)
	expectedReward := uint64(300) // 900 / 3 = 300
	for _, s := range submissions {
		reward, ok := rewardsDistributed[s.Signer]
		if !ok {
			t.Errorf("Signer %s did not receive any reward", s.Signer)
		}
		if reward != expectedReward {
			t.Errorf("Signer %s received incorrect reward: expected %d, got %d", s.Signer, expectedReward, reward)
		}
	}
}

func TestWeightedRewardDistributor_DistributeRewards(t *testing.T) {
	// Track rewards distributed
	rewardsDistributed := make(map[string]uint64)
	
	// Create a transfer function that records the rewards
	transferFunc := func(signer *url.URL, amount uint64) error {
		rewardsDistributed[signer.String()] = amount
		return nil
	}
	
	// Create a reward distributor
	distributor := NewWeightedRewardDistributor(transferFunc)
	
	// Create some test submissions
	submissions := []*MiningSubmission{
		{Difficulty: 100, Signer: "acc://signer1"},
		{Difficulty: 200, Signer: "acc://signer2"},
		{Difficulty: 300, Signer: "acc://signer3"},
	}
	
	// Total difficulty: 100 + 200 + 300 = 600
	
	// Distribute rewards
	err := distributor.DistributeRewards(submissions, 600)
	if err != nil {
		t.Fatalf("Error distributing rewards: %v", err)
	}
	
	// Check that each signer received the correct amount (weighted distribution)
	expectedRewards := map[string]uint64{
		"acc://signer1": 100, // (100/600) * 600 = 100
		"acc://signer2": 200, // (200/600) * 600 = 200
		"acc://signer3": 300, // (300/600) * 600 = 300
	}
	
	for signer, expectedReward := range expectedRewards {
		reward, ok := rewardsDistributed[signer]
		if !ok {
			t.Errorf("Signer %s did not receive any reward", signer)
		}
		if reward != expectedReward {
			t.Errorf("Signer %s received incorrect reward: expected %d, got %d", signer, expectedReward, reward)
		}
	}
}

func TestValidator_CloseWindowAndDistributeRewards(t *testing.T) {
	// Track rewards distributed
	rewardsDistributed := make(map[string]uint64)
	
	// Create a transfer function that records the rewards
	transferFunc := func(signer *url.URL, amount uint64) error {
		rewardsDistributed[signer.String()] = amount
		return nil
	}
	
	// Create a reward distributor
	distributor := NewDefaultRewardDistributor(transferFunc)
	
	// Create a transaction forwarder
	forwarder := NewDefaultTransactionForwarder(nil)
	
	// Create a validator with custom components
	validator := NewCustomValidator(60, 10, distributor, forwarder)
	
	// Create a test block hash
	blockHash := [32]byte{1, 2, 3, 4}
	
	// Start a new window
	validator.StartNewWindow(blockHash, 0)
	
	// Create some test submissions
	submissions := []*MiningSubmission{
		{Difficulty: 100, Signer: "acc://signer1"},
		{Difficulty: 200, Signer: "acc://signer2"},
		{Difficulty: 150, Signer: "acc://signer3"},
	}
	
	// Add the submissions to the queue
	for _, s := range submissions {
		validator.currentWindow.Queue.AddSubmission(s)
	}
	
	// Close the window and distribute rewards
	result, err := validator.CloseWindowAndDistributeRewards(900)
	if err != nil {
		t.Fatalf("Error closing window and distributing rewards: %v", err)
	}
	
	// Check that the correct number of submissions was returned
	if len(result) != 3 {
		t.Errorf("Expected 3 submissions, got %d", len(result))
	}
	
	// Check that each signer received the correct amount (equal distribution)
	expectedReward := uint64(300) // 900 / 3 = 300
	for _, s := range submissions {
		reward, ok := rewardsDistributed[s.Signer]
		if !ok {
			t.Errorf("Signer %s did not receive any reward", s.Signer)
		}
		if reward != expectedReward {
			t.Errorf("Signer %s received incorrect reward: expected %d, got %d", s.Signer, expectedReward, reward)
		}
	}
	
	// Check that the window is closed
	if validator.IsWindowActive() {
		t.Error("Window should be closed after distributing rewards")
	}
}
