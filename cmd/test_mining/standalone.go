// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"sync"
)

// MiningSubmission represents a mining submission
type MiningSubmission struct {
	Signer     string
	Difficulty uint64
}

// RewardDistributor distributes rewards to miners
type RewardDistributor interface {
	DistributeRewards(submissions []*MiningSubmission, totalReward uint64) error
}

// DefaultRewardDistributor implements a simple equal distribution strategy
type DefaultRewardDistributor struct {
	transferFunc func(signer string, amount uint64) error
	mu           sync.Mutex
}

// NewDefaultRewardDistributor creates a new DefaultRewardDistributor
func NewDefaultRewardDistributor(transferFunc func(signer string, amount uint64) error) *DefaultRewardDistributor {
	return &DefaultRewardDistributor{
		transferFunc: transferFunc,
	}
}

// SetTransferFunc sets the transfer function
func (d *DefaultRewardDistributor) SetTransferFunc(transferFunc func(signer string, amount uint64) error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.transferFunc = transferFunc
}

// DistributeRewards distributes rewards equally among submissions
func (d *DefaultRewardDistributor) DistributeRewards(submissions []*MiningSubmission, totalReward uint64) error {
	if len(submissions) == 0 {
		return fmt.Errorf("no submissions to distribute rewards to")
	}

	d.mu.Lock()
	transferFunc := d.transferFunc
	d.mu.Unlock()

	if transferFunc == nil {
		return fmt.Errorf("transfer function not set")
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

		err := transferFunc(submission.Signer, reward)
		if err != nil {
			return fmt.Errorf("failed to transfer reward: %w", err)
		}
	}

	return nil
}

// TransactionForwarder forwards mined transactions
type TransactionForwarder interface {
	ForwardTransaction(blockHash string, submissions []*MiningSubmission) error
}

// DefaultTransactionForwarder implements a simple transaction forwarding strategy
type DefaultTransactionForwarder struct {
	forwardFunc func(blockHash string, submissions []*MiningSubmission) error
	mu          sync.Mutex
}

// NewDefaultTransactionForwarder creates a new DefaultTransactionForwarder
func NewDefaultTransactionForwarder(forwardFunc func(blockHash string, submissions []*MiningSubmission) error) *DefaultTransactionForwarder {
	return &DefaultTransactionForwarder{
		forwardFunc: forwardFunc,
	}
}

// SetForwardFunc sets the forward function
func (f *DefaultTransactionForwarder) SetForwardFunc(forwardFunc func(blockHash string, submissions []*MiningSubmission) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forwardFunc = forwardFunc
}

// ForwardTransaction forwards the mined transaction
func (f *DefaultTransactionForwarder) ForwardTransaction(blockHash string, submissions []*MiningSubmission) error {
	f.mu.Lock()
	forwardFunc := f.forwardFunc
	f.mu.Unlock()

	if forwardFunc == nil {
		return nil
	}

	return forwardFunc(blockHash, submissions)
}

// MiningValidator validates mining submissions and manages the mining process
type MiningValidator struct {
	windowActive        bool
	currentHash         string
	difficulty          uint64
	submissions         []*MiningSubmission
	mu                  sync.Mutex
	rewardDistributor   RewardDistributor
	transactionForwarder TransactionForwarder
}

// NewMiningValidator creates a new MiningValidator
func NewMiningValidator() *MiningValidator {
	return &MiningValidator{
		windowActive:        false,
		submissions:         make([]*MiningSubmission, 0),
		rewardDistributor:   NewDefaultRewardDistributor(nil),
		transactionForwarder: NewDefaultTransactionForwarder(nil),
	}
}

// StartNewWindow starts a new mining window
func (v *MiningValidator) StartNewWindow(blockHash string, difficulty uint64) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.windowActive = true
	v.currentHash = blockHash
	v.difficulty = difficulty
	v.submissions = make([]*MiningSubmission, 0)
}

// SubmitSignature submits a signature for mining
func (v *MiningValidator) SubmitSignature(signer string, difficulty uint64) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check if the window is active
	if !v.windowActive {
		return false, fmt.Errorf("mining window is not active")
	}

	// Check if the difficulty is sufficient
	if difficulty < v.difficulty {
		return false, nil
	}

	// Create a mining submission
	submission := &MiningSubmission{
		Signer:     signer,
		Difficulty: difficulty,
	}

	// Add the submission to the list of submissions
	v.submissions = append(v.submissions, submission)

	return true, nil
}

// GetTopSubmissions returns the top submissions
func (v *MiningValidator) GetTopSubmissions() []*MiningSubmission {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.submissions
}

// CloseWindow closes the current mining window and returns the top submissions
func (v *MiningValidator) CloseWindow() []*MiningSubmission {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check if the window is active
	if !v.windowActive {
		return nil
	}

	// Get the top submissions
	topSubmissions := v.submissions

	// Reset the submissions
	v.submissions = make([]*MiningSubmission, 0)

	// Mark the window as inactive
	v.windowActive = false

	return topSubmissions
}

// CloseWindowAndDistributeRewards closes the current mining window, distributes rewards, and returns the top submissions
func (v *MiningValidator) CloseWindowAndDistributeRewards(totalReward uint64) ([]*MiningSubmission, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check if the window is active
	if !v.windowActive {
		return nil, nil
	}

	// Get the top submissions
	topSubmissions := v.submissions

	// Distribute rewards
	if v.rewardDistributor != nil {
		err := v.rewardDistributor.DistributeRewards(topSubmissions, totalReward)
		if err != nil {
			return nil, fmt.Errorf("failed to distribute rewards: %w", err)
		}
	}

	// Forward transactions
	if v.transactionForwarder != nil {
		err := v.transactionForwarder.ForwardTransaction(v.currentHash, topSubmissions)
		if err != nil {
			return nil, fmt.Errorf("failed to forward transactions: %w", err)
		}
	}

	// Reset the submissions
	v.submissions = make([]*MiningSubmission, 0)

	// Mark the window as inactive
	v.windowActive = false

	return topSubmissions, nil
}

// SetRewardDistributor sets the reward distributor
func (v *MiningValidator) SetRewardDistributor(distributor RewardDistributor) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.rewardDistributor = distributor
}

// SetTransactionForwarder sets the transaction forwarder
func (v *MiningValidator) SetTransactionForwarder(forwarder TransactionForwarder) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.transactionForwarder = forwarder
}

// IsWindowActive returns true if the mining window is active
func (v *MiningValidator) IsWindowActive() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.windowActive
}

func main() {
	fmt.Println("Testing LXR Mining Integration")
	fmt.Println("=============================")

	// Create a validator
	validator := NewMiningValidator()

	// Create a reward distributor
	rewardsDistributed := make(map[string]uint64)
	rewardMu := sync.Mutex{}

	transferFunc := func(signer string, amount uint64) error {
		rewardMu.Lock()
		defer rewardMu.Unlock()
		rewardsDistributed[signer] += amount
		return nil
	}

	distributor := NewDefaultRewardDistributor(transferFunc)
	validator.SetRewardDistributor(distributor)

	// Create a transaction forwarder
	txsForwarded := 0
	txMu := sync.Mutex{}

	forwardFunc := func(blockHash string, submissions []*MiningSubmission) error {
		txMu.Lock()
		defer txMu.Unlock()
		txsForwarded += len(submissions)
		return nil
	}

	forwarder := NewDefaultTransactionForwarder(forwardFunc)
	validator.SetTransactionForwarder(forwarder)

	// Start a new mining window
	blockHash := "test-block-hash"
	validator.StartNewWindow(blockHash, 100)

	// Submit some signatures
	miners := []string{"acc://test/miner1", "acc://test/miner2", "acc://test/miner3"}
	difficulties := []uint64{150, 200, 250}

	for i, miner := range miners {
		accepted, err := validator.SubmitSignature(miner, difficulties[i])
		if err != nil {
			fmt.Printf("Error submitting signature: %v\n", err)
			return
		}
		if !accepted {
			fmt.Printf("Signature not accepted for miner %s\n", miner)
			return
		}
	}

	// Get the top submissions
	submissions := validator.GetTopSubmissions()
	fmt.Printf("Got %d submissions\n", len(submissions))

	// Close the window and distribute rewards
	totalReward := uint64(900) // 900 tokens to distribute
	closedSubmissions, err := validator.CloseWindowAndDistributeRewards(totalReward)
	if err != nil {
		fmt.Printf("Error closing window and distributing rewards: %v\n", err)
		return
	}
	fmt.Printf("Closed window with %d submissions\n", len(closedSubmissions))

	// Check that rewards were distributed correctly
	rewardMu.Lock()
	fmt.Println("\nReward distribution:")
	var totalDistributed uint64
	for miner, amount := range rewardsDistributed {
		fmt.Printf("  %s: %d\n", miner, amount)
		totalDistributed += amount
	}
	rewardMu.Unlock()

	if totalDistributed != totalReward {
		fmt.Printf("Error: Total distributed rewards %d does not match expected %d\n", totalDistributed, totalReward)
		return
	}
	fmt.Printf("Total distributed rewards: %d\n", totalDistributed)

	// Check that transactions were forwarded
	txMu.Lock()
	if txsForwarded != len(miners) {
		fmt.Printf("Error: Forwarded %d transactions, expected %d\n", txsForwarded, len(miners))
		return
	}
	fmt.Printf("Successfully forwarded %d transactions\n", txsForwarded)
	txMu.Unlock()

	fmt.Println("\nAll tests passed successfully!")
}
