// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package mining

import (
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MiningWindow represents a time window for mining submissions
type MiningWindow struct {
	// StartTime is the start time of the window
	StartTime uint64
	// EndTime is the end time of the window
	EndTime uint64
	// BlockHash is the hash of the block being mined
	BlockHash [32]byte
	// MinDifficulty is the minimum difficulty required for submissions
	MinDifficulty uint64
	// Queue is the priority queue for submissions
	Queue *PriorityQueue
}

// Validator manages the validation of LxrMiningSignatures
type Validator struct {
	mu                sync.RWMutex
	hasher            *lxr.Hasher
	currentWindow     *MiningWindow
	windowDuration    uint64 // in seconds
	queueCapacity     int    // number of top submissions to keep
	rewardDistributor RewardDistributor
	txForwarder       TransactionForwarder
}

// NewValidator creates a new mining validator
func NewValidator(windowDuration uint64, queueCapacity int) *Validator {
	return &Validator{
		hasher:         lxr.NewHasher(),
		windowDuration: windowDuration,
		queueCapacity:  queueCapacity,
		// Use default implementations if not provided
		rewardDistributor: NewDefaultRewardDistributor(nil),
		txForwarder:       NewDefaultTransactionForwarder(nil),
	}
}

// NewCustomValidator creates a new mining validator with custom components
func NewCustomValidator(windowDuration uint64, queueCapacity int, rewardDistributor RewardDistributor, txForwarder TransactionForwarder) *Validator {
	return &Validator{
		hasher:            lxr.NewHasher(),
		windowDuration:    windowDuration,
		queueCapacity:     queueCapacity,
		rewardDistributor: rewardDistributor,
		txForwarder:       txForwarder,
	}
}

// StartNewWindow starts a new mining window
func (v *Validator) StartNewWindow(blockHash [32]byte, minDifficulty uint64) {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := uint64(time.Now().Unix())
	v.currentWindow = &MiningWindow{
		StartTime:     now,
		EndTime:       now + v.windowDuration,
		BlockHash:     blockHash,
		MinDifficulty: minDifficulty,
		Queue:         NewPriorityQueue(v.queueCapacity),
	}
}

// SubmitSignature submits an LxrMiningSignature for validation
// Returns true if the signature was accepted, false otherwise
func (v *Validator) SubmitSignature(sig *protocol.LxrMiningSignature) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Check if there is an active window
	if v.currentWindow == nil {
		return false, protocol.ErrNoActiveWindow
	}

	// Check if the window is still open
	now := uint64(time.Now().Unix())
	if now > v.currentWindow.EndTime {
		return false, protocol.ErrWindowClosed
	}

	// Check if the block hash matches
	for i := 0; i < 32; i++ {
		if sig.BlockHash[i] != v.currentWindow.BlockHash[i] {
			return false, protocol.ErrInvalidBlockHash
		}
	}

	// Verify the signature
	if !v.hasher.VerifySignature(sig, v.currentWindow.MinDifficulty) {
		return false, protocol.ErrInvalidSignature
	}

	// Calculate the difficulty
	_, difficulty := v.hasher.CalculatePow(sig.BlockHash[:], sig.Nonce)

	// Create a submission
	submission := &MiningSubmission{
		Nonce:         sig.Nonce,
		ComputedHash:  sig.ComputedHash,
		BlockHash:     sig.BlockHash,
		Signer:        sig.Signer.String(),
		SignerVersion: sig.SignerVersion,
		Timestamp:     sig.Timestamp,
		Difficulty:    difficulty,
	}

	// Add the submission to the queue
	added := v.currentWindow.Queue.AddSubmission(submission)
	return added, nil
}

// GetTopSubmissions returns the top N submissions for the current window
func (v *Validator) GetTopSubmissions() []*MiningSubmission {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.currentWindow == nil {
		return nil
	}

	return v.currentWindow.Queue.GetSubmissions()
}

// IsWindowActive returns true if there is an active mining window
func (v *Validator) IsWindowActive() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.currentWindow == nil {
		return false
	}

	now := uint64(time.Now().Unix())
	return now <= v.currentWindow.EndTime
}

// GetCurrentWindow returns the current mining window
func (v *Validator) GetCurrentWindow() *MiningWindow {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return v.currentWindow
}

// RewardDistributor returns the reward distributor
func (v *Validator) RewardDistributor() RewardDistributor {
	return v.rewardDistributor
}

// SetRewardDistributor sets the reward distributor
func (v *Validator) SetRewardDistributor(rd RewardDistributor) {
	v.rewardDistributor = rd
}

// TransactionForwarder returns the transaction forwarder
func (v *Validator) TransactionForwarder() TransactionForwarder {
	return v.txForwarder
}

// SetTransactionForwarder sets the transaction forwarder
func (v *Validator) SetTransactionForwarder(tf TransactionForwarder) {
	v.txForwarder = tf
}

// CloseWindow closes the current mining window and returns the top submissions
func (v *Validator) CloseWindow() []*MiningSubmission {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.currentWindow == nil {
		return nil
	}

	submissions := v.currentWindow.Queue.GetSubmissions()
	
	// Store the block hash before clearing the window
	blockHash := v.currentWindow.BlockHash
	
	// Clear the current window
	v.currentWindow = nil
	
	// Forward the transaction if a forwarder is configured
	if v.txForwarder != nil {
		_ = v.txForwarder.ForwardTransaction(blockHash, submissions)
	}
	
	return submissions
}

// DistributeRewards distributes rewards to the top miners
func (v *Validator) DistributeRewards(submissions []*MiningSubmission, totalReward uint64) error {
	if v.rewardDistributor == nil {
		return ErrNotImplemented
	}
	
	return v.rewardDistributor.DistributeRewards(submissions, totalReward)
}

// CloseWindowAndDistributeRewards closes the current mining window, distributes rewards, and returns the top submissions
func (v *Validator) CloseWindowAndDistributeRewards(totalReward uint64) ([]*MiningSubmission, error) {
	// Close the window and get the top submissions
	submissions := v.CloseWindow()
	
	// Distribute rewards if there are submissions
	if len(submissions) > 0 && v.rewardDistributor != nil {
		if err := v.DistributeRewards(submissions, totalReward); err != nil {
			return submissions, err
		}
	}
	
	return submissions, nil
}
