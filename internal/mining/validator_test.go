// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package mining

import (
	"crypto/sha256"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func init() {
	// Set the test environment flag to use smaller hash tables
	lxr.IsTestEnvironment = true
}

func TestValidator_StartNewWindow(t *testing.T) {
	// Create a validator
	validator := NewValidator(60, 10) // 60 second window, 10 submissions
	
	// Create a test block hash
	blockHash := sha256.Sum256([]byte("test block"))
	
	// Start a new window
	validator.StartNewWindow(blockHash, 100)
	
	// Check that the window was created
	window := validator.GetCurrentWindow()
	if window == nil {
		t.Fatal("Window was not created")
	}
	
	// Check the window properties
	if window.BlockHash != blockHash {
		t.Errorf("Window has wrong block hash: %x != %x", window.BlockHash, blockHash)
	}
	
	if window.MinDifficulty != 100 {
		t.Errorf("Window has wrong minimum difficulty: %d != %d", window.MinDifficulty, 100)
	}
	
	// Check that the window is active
	if !validator.IsWindowActive() {
		t.Error("Window should be active")
	}
}

func TestValidator_SubmitSignature(t *testing.T) {
	// Create a validator
	validator := NewValidator(60, 10) // 60 second window, 10 submissions
	
	// Create a test block hash
	blockHash := sha256.Sum256([]byte("test block"))
	
	// Start a new window
	validator.StartNewWindow(blockHash, 0) // Use 0 difficulty for testing
	
	// Create a hasher for testing
	hasher := lxr.NewCustomHasher(1, 12, 1) // Use very small values for testing
	
	// Create a test nonce
	nonce := []byte("test nonce")
	
	// Calculate the proof of work
	computedHash, _ := hasher.CalculatePow(blockHash[:], nonce)
	
	// Create a valid signature
	signerURL := url.MustParse("acc://test/signer")
	validSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     blockHash,
		Signer:        signerURL,
		SignerVersion: 1,
		Timestamp:     uint64(time.Now().Unix()),
	}
	
	// Submit the signature
	accepted, err := validator.SubmitSignature(validSig)
	if err != nil {
		t.Fatalf("Error submitting signature: %v", err)
	}
	if !accepted {
		t.Error("Valid signature was not accepted")
	}
	
	// Check that the submission was added to the queue
	submissions := validator.GetTopSubmissions()
	if len(submissions) != 1 {
		t.Errorf("Expected 1 submission, got %d", len(submissions))
	}
	
	// Create an invalid signature with wrong block hash
	wrongBlockHash := sha256.Sum256([]byte("wrong block"))
	invalidSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     wrongBlockHash,
		Signer:        signerURL,
		SignerVersion: 1,
		Timestamp:     uint64(time.Now().Unix()),
	}
	
	// Submit the invalid signature
	accepted, err = validator.SubmitSignature(invalidSig)
	if err == nil {
		t.Error("Expected error submitting invalid signature")
	}
	if accepted {
		t.Error("Invalid signature was accepted")
	}
}

func TestValidator_CloseWindow(t *testing.T) {
	// Create a validator
	validator := NewValidator(60, 10) // 60 second window, 10 submissions
	
	// Create a test block hash
	blockHash := sha256.Sum256([]byte("test block"))
	
	// Start a new window
	validator.StartNewWindow(blockHash, 0) // Use 0 difficulty for testing
	
	// Create a hasher for testing
	hasher := lxr.NewCustomHasher(1, 12, 1) // Use very small values for testing
	
	// Create a test nonce
	nonce := []byte("test nonce")
	
	// Calculate the proof of work
	computedHash, _ := hasher.CalculatePow(blockHash[:], nonce)
	
	// Create a valid signature
	signerURL := url.MustParse("acc://test/signer")
	validSig := &protocol.LxrMiningSignature{
		Nonce:         nonce,
		ComputedHash:  computedHash,
		BlockHash:     blockHash,
		Signer:        signerURL,
		SignerVersion: 1,
		Timestamp:     uint64(time.Now().Unix()),
	}
	
	// Submit the signature
	validator.SubmitSignature(validSig)
	
	// Close the window
	submissions := validator.CloseWindow()
	
	// Check that the submissions were returned
	if len(submissions) != 1 {
		t.Errorf("Expected 1 submission, got %d", len(submissions))
	}
	
	// Check that the window is no longer active
	if validator.IsWindowActive() {
		t.Error("Window should not be active after closing")
	}
	
	// Check that the current window is nil
	if validator.GetCurrentWindow() != nil {
		t.Error("Current window should be nil after closing")
	}
}

func TestValidator_RewardDistribution(t *testing.T) {
	// Create a validator
	validator := NewValidator(60, 10) // 60 second window, 10 submissions
	
	// Create a test block hash
	blockHash := sha256.Sum256([]byte("test block"))
	
	// Start a new window
	validator.StartNewWindow(blockHash, 0) // Use 0 difficulty for testing
	
	// Create a hasher for testing
	hasher := lxr.NewCustomHasher(1, 12, 1) // Use very small values for testing
	
	// Track rewards distributed
	rewardsDistributed := make(map[string]uint64)
	
	// Create a reward distributor
	distributor := NewDefaultRewardDistributor(func(signer *url.URL, amount uint64) error {
		rewardsDistributed[signer.String()] += amount
		return nil
	})
	
	// Set the reward distributor
	validator.SetRewardDistributor(distributor)
	
	// Create and submit multiple signatures
	miners := []string{"acc://test/miner1", "acc://test/miner2", "acc://test/miner3"}
	for i, minerAddr := range miners {
		// Create a unique nonce for each miner
		nonce := []byte("test nonce " + minerAddr)
		
		// Calculate the proof of work
		computedHash, _ := hasher.CalculatePow(blockHash[:], nonce)
		
		// Create a valid signature
		signerURL := url.MustParse(minerAddr)
		validSig := &protocol.LxrMiningSignature{
			Nonce:         nonce,
			ComputedHash:  computedHash,
			BlockHash:     blockHash,
			Signer:        signerURL,
			SignerVersion: uint64(i + 1),
			Timestamp:     uint64(time.Now().Unix()),
		}
		
		// Submit the signature
		validator.SubmitSignature(validSig)
	}
	
	// Total reward to distribute
	totalReward := uint64(900) // 900 tokens to distribute
	
	// Close the window and distribute rewards
	submissions, err := validator.CloseWindowAndDistributeRewards(totalReward)
	if err != nil {
		t.Fatalf("Failed to close window and distribute rewards: %v", err)
	}
	
	// Check that the submissions were returned
	if len(submissions) != len(miners) {
		t.Errorf("Expected %d submissions, got %d", len(miners), len(submissions))
	}
	
	// Check that rewards were distributed to all miners
	if len(rewardsDistributed) != len(miners) {
		t.Errorf("Expected rewards distributed to %d miners, got %d", len(miners), len(rewardsDistributed))
	}
	
	// Check that the total rewards distributed matches the expected amount
	var totalDistributed uint64
	for _, amount := range rewardsDistributed {
		totalDistributed += amount
	}
	
	if totalDistributed != totalReward {
		t.Errorf("Expected total distributed rewards %d, got %d", totalReward, totalDistributed)
	}
	
	// Check that the window is no longer active
	if validator.IsWindowActive() {
		t.Error("Window should not be active after closing")
	}
}

func TestValidator_TransactionForwarding(t *testing.T) {
	// Create a validator
	validator := NewValidator(60, 10) // 60 second window, 10 submissions
	
	// Create a test block hash
	blockHash := sha256.Sum256([]byte("test block"))
	
	// Start a new window
	validator.StartNewWindow(blockHash, 0) // Use 0 difficulty for testing
	
	// Create a hasher for testing
	hasher := lxr.NewCustomHasher(1, 12, 1) // Use very small values for testing
	
	// Track forwarded transactions
	txsForwarded := 0
	
	// Create a transaction forwarder
	forwarder := NewDefaultTransactionForwarder(func(blockHash [32]byte, submissions []*MiningSubmission) error {
		txsForwarded += len(submissions)
		return nil
	})
	
	// Set the transaction forwarder
	validator.SetTransactionForwarder(forwarder)
	
	// Create a reward distributor with a dummy transfer function
	rewardDistributor := NewDefaultRewardDistributor(func(signer *url.URL, amount uint64) error {
		return nil // Just a dummy function that does nothing
	})
	
	// Set the reward distributor
	validator.SetRewardDistributor(rewardDistributor)
	
	// Create and submit multiple signatures
	miners := []string{"acc://test/miner1", "acc://test/miner2", "acc://test/miner3"}
	for i, minerAddr := range miners {
		// Create a unique nonce for each miner
		nonce := []byte("test nonce " + minerAddr)
		
		// Calculate the proof of work
		computedHash, _ := hasher.CalculatePow(blockHash[:], nonce)
		
		// Create a valid signature
		signerURL := url.MustParse(minerAddr)
		validSig := &protocol.LxrMiningSignature{
			Nonce:         nonce,
			ComputedHash:  computedHash,
			BlockHash:     blockHash,
			Signer:        signerURL,
			SignerVersion: uint64(i + 1),
			Timestamp:     uint64(time.Now().Unix()),
		}
		
		// Submit the signature
		validator.SubmitSignature(validSig)
	}
	
	// Close the window and forward transactions
	submissions, err := validator.CloseWindowAndDistributeRewards(0) // 0 reward, we're just testing forwarding
	if err != nil {
		t.Fatalf("Failed to close window and forward transactions: %v", err)
	}
	
	// Check that the submissions were returned
	if len(submissions) != len(miners) {
		t.Errorf("Expected %d submissions, got %d", len(miners), len(submissions))
	}
	
	// Check that transactions were forwarded
	if txsForwarded != len(miners) {
		t.Errorf("Expected %d transactions forwarded, got %d", len(miners), txsForwarded)
	}
	
	// Check that the window is no longer active
	if validator.IsWindowActive() {
		t.Error("Window should not be active after closing")
	}
}
