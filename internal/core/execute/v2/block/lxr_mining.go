// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining"
	"gitlab.com/accumulatenetwork/accumulate/internal/mining/lxr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// verifyLxrMiningSignature verifies an LxrMiningSignature
func verifyLxrMiningSignature(batch *database.Batch, transaction *protocol.Transaction, signature *protocol.LxrMiningSignature, md sigExecMetadata) error {
	// Get the signer URL
	signerUrl := signature.Signer
	if signerUrl == nil {
		return protocol.ErrInvalidSignature.With("missing signer URL")
	}

	// Get the key page
	var keyPage *protocol.KeyPage
	err := batch.Account(signerUrl).Main().GetAs(&keyPage)
	if err != nil {
		return protocol.ErrInvalidSignature.WithFormat("failed to load key page: %v", err)
	}

	// Check if mining is enabled
	if !keyPage.MiningEnabled {
		return protocol.ErrInvalidSignature.With("mining is not enabled for this key page")
	}

	// Check if the signer version matches
	if signature.SignerVersion != keyPage.Version {
		return protocol.ErrInvalidSignature.WithFormat("signer version mismatch: got %d, want %d", signature.SignerVersion, keyPage.Version)
	}

	// Create a hasher
	hasher := lxr.NewHasher()

	// Verify the signature
	if !hasher.VerifySignature(signature, keyPage.MiningDifficulty) {
		return protocol.ErrInvalidSignature.With("invalid LXR mining signature")
	}

	return nil
}

// LxrMiningValidator is a validator for LxrMiningSignatures
type LxrMiningValidator struct {
	logger   logging.Logger
	hasher   *lxr.Hasher
	registry *mining.Validator
	batch    *database.Batch // Current batch for database operations
}

// NewLxrMiningValidator creates a new LxrMiningValidator
func NewLxrMiningValidator(logger logging.Logger) *LxrMiningValidator {
	// Create a reward distributor that transfers rewards to miners
	rewardDistributor := mining.NewDefaultRewardDistributor(func(signer *url.URL, amount uint64) error {
		return transferReward(nil, signer, amount) // Will be updated with batch when available
	})

	// Create a transaction forwarder that forwards mined transactions
	txForwarder := mining.NewDefaultTransactionForwarder(func(blockHash [32]byte, submissions []*mining.MiningSubmission) error {
		return forwardMinedTransaction(nil, blockHash, submissions) // Will be updated with batch when available
	})

	return &LxrMiningValidator{
		logger:   logger,
		hasher:   lxr.NewHasher(),
		registry: mining.NewCustomValidator(3600, 10, rewardDistributor, txForwarder), // 1 hour window, 10 submissions
	}
}

// StartNewWindow starts a new mining window
func (v *LxrMiningValidator) StartNewWindow(blockHash [32]byte, minDifficulty uint64) {
	v.registry.StartNewWindow(blockHash, minDifficulty)
	v.logger.Info("Started new mining window", "blockHash", blockHash, "minDifficulty", minDifficulty)
}

// SubmitSignature submits an LxrMiningSignature for validation
func (v *LxrMiningValidator) SubmitSignature(signature *protocol.LxrMiningSignature) (bool, error) {
	accepted, err := v.registry.SubmitSignature(signature)
	if err != nil {
		v.logger.Info("Mining signature rejected", "error", err)
		return false, err
	}
	if accepted {
		v.logger.Info("Mining signature accepted", "signer", signature.Signer)
	} else {
		v.logger.Info("Mining signature not accepted (low difficulty)", "signer", signature.Signer)
	}
	return accepted, nil
}

// GetTopSubmissions returns the top N submissions for the current window
func (v *LxrMiningValidator) GetTopSubmissions() []*mining.MiningSubmission {
	return v.registry.GetTopSubmissions()
}

// CloseWindow closes the current mining window and returns the top submissions
func (v *LxrMiningValidator) CloseWindow() []*mining.MiningSubmission {
	submissions := v.registry.CloseWindow()
	v.logger.Info("Closed mining window", "submissions", len(submissions))
	return submissions
}

// CloseWindowAndDistributeRewards closes the current mining window, distributes rewards, and returns the top submissions
func (v *LxrMiningValidator) CloseWindowAndDistributeRewards(totalReward uint64) ([]*mining.MiningSubmission, error) {
	// Update the reward distributor and transaction forwarder with the current batch
	v.updateCallbacks()
	
	// Close the window, distribute rewards, and return the top submissions
	submissions, err := v.registry.CloseWindowAndDistributeRewards(totalReward)
	if err != nil {
		v.logger.Error("Failed to distribute rewards", "error", err)
		return submissions, err
	}
	
	v.logger.Info("Closed mining window and distributed rewards", "submissions", len(submissions), "totalReward", totalReward)
	return submissions, nil
}

// SetBatch sets the current batch for database operations
func (v *LxrMiningValidator) SetBatch(batch *database.Batch) {
	v.batch = batch
	v.updateCallbacks()
}

// updateCallbacks updates the reward distributor and transaction forwarder with the current batch
func (v *LxrMiningValidator) updateCallbacks() {
	// Skip if no batch is available
	if v.batch == nil {
		return
	}
	
	// Get the reward distributor and transaction forwarder
	if rd, ok := v.registry.RewardDistributor().(*mining.DefaultRewardDistributor); ok {
		// Update the transfer function with the current batch
		rd.SetTransferFunc(func(signer *url.URL, amount uint64) error {
			return transferReward(v.batch, signer, amount)
		})
	}
	
	if tf, ok := v.registry.TransactionForwarder().(*mining.DefaultTransactionForwarder); ok {
		// Update the forward function with the current batch
		tf.SetForwardFunc(func(blockHash [32]byte, submissions []*mining.MiningSubmission) error {
			return forwardMinedTransaction(v.batch, blockHash, submissions)
		})
	}
}

// IsWindowActive returns true if there is an active mining window
func (v *LxrMiningValidator) IsWindowActive() bool {
	return v.registry.IsWindowActive()
}

// transferReward transfers a reward to a miner
func transferReward(batch *database.Batch, signer *url.URL, amount uint64) error {
	// Skip if no batch is available
	if batch == nil {
		return nil
	}
	
	// Get the account
	account, err := batch.Account(signer).Main().Get()
	if err != nil {
		return protocol.ErrInvalidSignature.WithFormat("failed to load account: %v", err)
	}
	
	// Check if the account is a lite token account
	lta, ok := account.(*protocol.LiteTokenAccount)
	if !ok {
		// Try to get the lite identity
		lid, ok := account.(*protocol.LiteIdentity)
		if !ok {
			return protocol.ErrInvalidSignature.WithFormat("account %v is not a lite token account or lite identity", signer)
		}
		
		// Get the ACME token account
		acmeUrl := url.JoinPath(lid.Url(), "acme")
		account, err = batch.Account(acmeUrl).Main().Get()
		if err != nil {
			return protocol.ErrInvalidSignature.WithFormat("failed to load ACME token account: %v", err)
		}
		
		// Check if the account is a lite token account
		lta, ok = account.(*protocol.LiteTokenAccount)
		if !ok {
			return protocol.ErrInvalidSignature.WithFormat("account %v is not a lite token account", acmeUrl)
		}
	}
	
	// Update the balance
	lta.Balance += amount
	
	// Save the account
	err = batch.Account(lta.Url()).Main().Put(lta)
	if err != nil {
		return protocol.ErrInvalidSignature.WithFormat("failed to save account: %v", err)
	}
	
	return nil
}

// forwardMinedTransaction forwards a mined transaction
func forwardMinedTransaction(batch *database.Batch, blockHash [32]byte, submissions []*mining.MiningSubmission) error {
	// Skip if no batch is available or no submissions
	if batch == nil || len(submissions) == 0 {
		return nil
	}
	
	// Process each submission, starting with the highest difficulty
	for _, submission := range submissions {
		// Skip if the signature doesn't have transaction data
		if submission.Signature == nil || len(submission.Signature.TransactionData) == 0 {
			continue
		}
		
		// Create a URL for the signer
		signerUrl := submission.Signature.Signer
		if signerUrl == nil {
			continue
		}
		
		// Parse the transaction data
		tx := new(protocol.Transaction)
		err := tx.UnmarshalBinary(submission.Signature.TransactionData)
		if err != nil {
			// Log the error but continue with other submissions
			continue
		}
		
		// Set the synthetic transaction origin
		tx.Header.Principal = signerUrl
		
		// Create a synthetic transaction
		synthTx := &protocol.SyntheticTransaction{
			Cause: &protocol.TransactionReference{
				SourceNetwork: protocol.Directory,
				Hash:          submission.Signature.Hash(),
			},
			Transaction: tx,
		}
		
		// Add the synthetic transaction to the batch
		// In a real implementation, this would involve adding the transaction to the mempool
		// or directly executing it within the current batch
		
		// For now, we'll just store it in the batch metadata
		key := fmt.Sprintf("mined-tx-%x", submission.Signature.Hash())
		batch.Put(key, synthTx)
	}
	
	return nil
}

// GetCurrentWindow returns the current mining window
func (v *LxrMiningValidator) GetCurrentWindow() *mining.MiningWindow {
	return v.registry.GetCurrentWindow()
}
