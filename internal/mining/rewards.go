// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package mining

import (
	"errors"
	"sync"
	
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Error definitions
var (
	ErrNotImplemented = errors.New("not implemented")
)

// RewardDistributor defines the interface for distributing rewards to miners
type RewardDistributor interface {
	// DistributeRewards distributes rewards to the top miners
	DistributeRewards(submissions []*MiningSubmission, totalReward uint64) error
}

// DefaultRewardDistributor implements the RewardDistributor interface
// with a simple equal distribution strategy
type DefaultRewardDistributor struct {
	// TransferReward is a function that transfers rewards to a miner
	TransferReward func(signer *url.URL, amount uint64) error
	mu            sync.Mutex
}

// NewDefaultRewardDistributor creates a new DefaultRewardDistributor
func NewDefaultRewardDistributor(transferFunc func(signer *url.URL, amount uint64) error) *DefaultRewardDistributor {
	return &DefaultRewardDistributor{
		TransferReward: transferFunc,
	}
}

// SetTransferFunc sets the transfer function
func (d *DefaultRewardDistributor) SetTransferFunc(transferFunc func(signer *url.URL, amount uint64) error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.TransferReward = transferFunc
}

// DistributeRewards distributes rewards equally among the top miners
func (d *DefaultRewardDistributor) DistributeRewards(submissions []*MiningSubmission, totalReward uint64) error {
	if len(submissions) == 0 {
		return nil
	}

	// Check if the transfer function is set
	if d.TransferReward == nil {
		return errors.New("transfer reward function not set")
	}

	// Calculate reward per miner (equal distribution)
	rewardPerMiner := totalReward / uint64(len(submissions))
	
	// Distribute rewards to each miner
	for _, submission := range submissions {
		// Skip if the submission doesn't have a valid signer
		if submission.Signer == "" {
			continue
		}
		
		// Parse the signer URL
		signerURL, err := url.Parse(submission.Signer)
		if err != nil {
			// Log error but continue with other miners
			continue
		}
		
		// Transfer the reward
		if err := d.TransferReward(signerURL, rewardPerMiner); err != nil {
			// Log error but continue with other miners
			continue
		}
	}
	
	return nil
}

// WeightedRewardDistributor implements a weighted distribution strategy
// where rewards are distributed based on the difficulty of the submission
type WeightedRewardDistributor struct {
	// TransferReward is a function that transfers rewards to a miner
	TransferReward func(signer *url.URL, amount uint64) error
}

// NewWeightedRewardDistributor creates a new WeightedRewardDistributor
func NewWeightedRewardDistributor(transferFunc func(signer *url.URL, amount uint64) error) *WeightedRewardDistributor {
	return &WeightedRewardDistributor{
		TransferReward: transferFunc,
	}
}

// DistributeRewards distributes rewards based on the difficulty of each submission
func (w *WeightedRewardDistributor) DistributeRewards(submissions []*MiningSubmission, totalReward uint64) error {
	if len(submissions) == 0 {
		return nil
	}
	
	// Calculate total difficulty
	var totalDifficulty uint64
	for _, submission := range submissions {
		totalDifficulty += submission.Difficulty
	}
	
	// Distribute rewards proportionally to difficulty
	for _, submission := range submissions {
		// Skip if the submission doesn't have a valid signer
		if submission.Signer == "" {
			continue
		}
		
		// Parse the signer URL
		signerURL, err := url.Parse(submission.Signer)
		if err != nil {
			// Log error but continue with other miners
			continue
		}
		
		// Calculate weighted reward
		weightedReward := (submission.Difficulty * totalReward) / totalDifficulty
		
		// Transfer the reward
		if err := w.TransferReward(signerURL, weightedReward); err != nil {
			// Log error but continue with other miners
			continue
		}
	}
	
	return nil
}

// TransactionForwarder defines the interface for forwarding mined transactions
type TransactionForwarder interface {
	// ForwardTransaction forwards a mined transaction
	ForwardTransaction(blockHash [32]byte, submissions []*MiningSubmission) error
}

// DefaultTransactionForwarder implements a simple transaction forwarding strategy
type DefaultTransactionForwarder struct {
	// ForwardFunc is a function that forwards a transaction
	ForwardFunc func(blockHash [32]byte, submissions []*MiningSubmission) error
	mu          sync.Mutex
}

// NewDefaultTransactionForwarder creates a new DefaultTransactionForwarder
func NewDefaultTransactionForwarder(forwardFunc func(blockHash [32]byte, submissions []*MiningSubmission) error) *DefaultTransactionForwarder {
	return &DefaultTransactionForwarder{
		ForwardFunc: forwardFunc,
	}
}

// SetForwardFunc sets the forward function
func (f *DefaultTransactionForwarder) SetForwardFunc(forwardFunc func(blockHash [32]byte, submissions []*MiningSubmission) error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ForwardFunc = forwardFunc
}

// ForwardTransaction forwards the mined transaction
func (f *DefaultTransactionForwarder) ForwardTransaction(blockHash [32]byte, submissions []*MiningSubmission) error {
	f.mu.Lock()
	forwardFunc := f.ForwardFunc
	f.mu.Unlock()
	
	if forwardFunc == nil {
		return nil
	}
	
	return forwardFunc(blockHash, submissions)
}
