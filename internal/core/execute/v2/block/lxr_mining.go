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
	// Validate batch
	if batch == nil {
		return fmt.Errorf("%w: database batch is nil", protocol.ErrInvalidSignature)
	}

	// Validate transaction
	if transaction == nil {
		return fmt.Errorf("%w: transaction is nil", protocol.ErrInvalidSignature)
	}

	// Validate signature
	if signature == nil {
		return fmt.Errorf("%w: no mining signature", protocol.ErrInvalidSignature)
	}

	// Get the signer URL
	signerUrl := signature.Signer
	if signerUrl == nil {
		return fmt.Errorf("%w: signature has no signer", protocol.ErrInvalidSignature)
	}

	// Validate block hash
	if signature.BlockHash == [32]byte{} {
		return fmt.Errorf("%w: signature has no block hash", protocol.ErrInvalidSignature)
	}

	// Validate nonce
	if len(signature.Nonce) == 0 {
		return fmt.Errorf("%w: signature has no nonce", protocol.ErrInvalidSignature)
	}

	// Validate computed hash
	if signature.ComputedHash == [32]byte{} {
		return fmt.Errorf("%w: signature has no computed hash", protocol.ErrInvalidSignature)
	}

	// Get the key page
	var keyPage *protocol.KeyPage
	err := batch.Account(signerUrl).Main().GetAs(&keyPage)
	if err != nil {
		logging.L.Error("Failed to load key page for LXR mining signature", 
			"error", err, 
			"signer", signerUrl)
		return fmt.Errorf("%w: failed to load key page: %v", protocol.ErrInvalidSignature, err)
	}

	// Check if the account is a key page
	if keyPage == nil {
		logging.L.Error("Account is not a key page", "signer", signerUrl)
		return fmt.Errorf("%w: account %v is not a key page", protocol.ErrInvalidSignature, signerUrl)
	}

	// Check if mining is enabled
	if !keyPage.MiningEnabled {
		logging.L.Debug("Mining is not enabled for key page", "signer", signerUrl)
		return fmt.Errorf("%w: mining is not enabled for this key page", protocol.ErrInvalidSignature)
	}

	// Check if the signer version matches
	if signature.SignerVersion != keyPage.Version {
		logging.L.Debug("Signer version mismatch", 
			"signer", signerUrl, 
			"got", signature.SignerVersion, 
			"want", keyPage.Version)
		return fmt.Errorf("%w: signer version mismatch: got %d, want %d", protocol.ErrInvalidSignature, signature.SignerVersion, keyPage.Version)
	}

	// Create a hasher
	hasher := lxr.NewHasher()

	// Verify the signature
	if !hasher.VerifySignature(signature, keyPage.MiningDifficulty) {
		// Calculate the actual difficulty to provide more detailed error message
		_, actualDifficulty := hasher.CalculatePow(signature.GetBlockHash(), signature.GetNonce())
		logging.L.Debug("Invalid LXR mining signature", 
			"signer", signerUrl, 
			"required", keyPage.MiningDifficulty, 
			"actual", actualDifficulty)
		
		if actualDifficulty < keyPage.MiningDifficulty {
			return fmt.Errorf("%w: insufficient mining difficulty: got %d, required %d", protocol.ErrInvalidSignature, actualDifficulty, keyPage.MiningDifficulty)
		}
		return fmt.Errorf("%w: invalid LXR mining signature: hash verification failed", protocol.ErrInvalidSignature)
	}

	// Log successful verification
	logging.L.Debug("Successfully verified LXR mining signature", 
		"signer", signerUrl, 
		"difficulty", keyPage.MiningDifficulty)

	return nil
}

// LxrMiningValidator handles validation of LXR mining signatures
type LxrMiningValidator struct {
	hasher   *lxr.Hasher
	logger   logging.Logger
	registry *mining.Validator
	batch    *database.Batch // Database batch for transaction storage
}

// NewLxrMiningValidator creates a new LxrMiningValidator
func NewLxrMiningValidator(logger logging.Logger) *LxrMiningValidator {
	// Create a reward distributor that transfers rewards to miners
	rewardDistributor := mining.NewDefaultRewardDistributor(func(signer *url.URL, amount uint64) error {
		return transferReward(nil, signer, amount) // Will be updated with batch when available
	})

	// Create a transaction forwarder that forwards mined transactions
	txForwarder := mining.NewDefaultTransactionForwarder(func(blockHash [32]byte, submissions []*mining.MiningSubmission) error {
		// Initially, this is a placeholder. We'll update it after creating the validator
		return nil
	})

	validator := &LxrMiningValidator{
		hasher:   lxr.NewHasher(),
		logger:   logger,
		registry: mining.NewCustomValidator(3600, 10, rewardDistributor, txForwarder), // 1 hour window, 10 submissions
	}

	// Update the forwarder to use this validator instance
	txForwarder.SetForwardFunc(func(blockHash [32]byte, submissions []*mining.MiningSubmission) error {
		return validator.ForwardTransaction(blockHash, submissions)
	})

	return validator
}

// StartNewWindow starts a new mining window
func (v *LxrMiningValidator) StartNewWindow(batch *database.Batch, blockHash [32]byte, windowDifficulty uint64) {
	v.batch = batch
	v.registry.StartNewWindow(blockHash, windowDifficulty)
	v.logger.Info("Started new mining window", "blockHash", blockHash, "windowDifficulty", windowDifficulty)

	// Update the transaction forwarder to use the current batch
	if tf, ok := v.registry.TransactionForwarder().(*mining.DefaultTransactionForwarder); ok {
		// Update the forward function with the current batch
		tf.SetForwardFunc(func(blockHash [32]byte, submissions []*mining.MiningSubmission) error {
			return v.ForwardTransaction(blockHash, submissions)
		})
	}
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

// CloseWindow closes the current mining window
func (v *LxrMiningValidator) CloseWindow(batch *database.Batch) ([]*mining.MiningSubmission, error) {
	// Get the submissions
	submissions := v.registry.CloseWindow()
	
	// Forward the mined transactions
	if err := v.ForwardTransaction(v.registry.GetCurrentWindow().BlockHash, submissions); err != nil {
		return submissions, fmt.Errorf("failed to forward mined transactions: %w", err)
	}
	
	return submissions, nil
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
			return v.ForwardTransaction(blockHash, submissions)
		})
	}
}

// IsWindowActive returns true if there is an active mining window
func (v *LxrMiningValidator) IsWindowActive() bool {
	return v.registry.IsWindowActive()
}

// ForwardTransaction forwards a mining transaction to the mempool
func (v *LxrMiningValidator) ForwardTransaction(blockHash [32]byte, submissions []*mining.MiningSubmission) error {
	// Skip if no submissions
	if len(submissions) == 0 {
		return nil
	}
	
	// In a real implementation, we would get access to the database
	// For now, log that we would process these transactions
	v.logger.Info("Would forward mining transactions", "count", len(submissions))
	
	var errs []error
	
	// Process each submission, starting with the highest difficulty
	for _, submission := range submissions {
		// Create a LxrMiningSignature from the submission
		signature := &protocol.LxrMiningSignature{
			Nonce:         submission.Nonce,
			ComputedHash:  submission.ComputedHash,
			BlockHash:     submission.BlockHash,
			SignerVersion: submission.SignerVersion,
			Timestamp:     submission.Timestamp,
		}
		
		// Parse the signer URL
		signerUrl, err := url.Parse(submission.Signer)
		if err != nil {
			v.logger.Debug("Skipping submission with invalid signer URL", 
				"signer", submission.Signer,
				"difficulty", submission.Difficulty,
				"error", err)
			continue
		}
		signature.Signer = signerUrl
		
		// Skip if we can't resolve the signer URL
		if signature.Signer == nil {
			v.logger.Debug("Skipping submission with no signer URL", 
				"difficulty", submission.Difficulty)
			continue
		}
		
		// Create a transaction ID for the synthetic transaction
		// Convert the hash slice to a [32]byte array
		var hashBytes [32]byte
		copy(hashBytes[:], signature.Hash())
		txID := signature.Signer.WithTxID(hashBytes)
		
		v.logger.Info("Created synthetic transaction for mining submission", 
			"txID", txID,
			"signer", signature.Signer, 
			"difficulty", submission.Difficulty)
		
		// In a real implementation, we would add the transaction to the database
		// We would use something like:
		//
		// batch := database.NewBatch("mining", kvstore, true, v.logger)
		// defer batch.Discard()
		//
		// txBody := &protocol.SyntheticCreateIdentity{}
		// txBody.Cause = txID
		// txBody.Initiator = signature.Signer
		//
		// tx := &protocol.Transaction{
		//     Header: protocol.TransactionHeader{
		//         Principal: signature.Signer,
		//     },
		//     Body: txBody,
		// }
		//
		// err = batch.Account(tx.Header.Principal).Transaction(txID.Hash()).Main().Put(tx)
		// if err != nil {
		//     errs = append(errs, fmt.Errorf("failed to add synthetic transaction: %w", err))
		//     continue
		// }
		//
		// err = batch.Commit()
		// if err != nil {
		//     errs = append(errs, fmt.Errorf("failed to commit batch: %w", err))
		// }
	}
	
	// Return errors if any
	if len(errs) > 0 {
		return fmt.Errorf("failed to forward %d/%d mining transactions: %v", len(errs), len(submissions), errs)
	}
	
	return nil
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
		return fmt.Errorf("%w: failed to load account: %v", protocol.ErrInvalidSignature, err)
	}
	
	// Check if the account is a lite token account
	lta, ok := account.(*protocol.LiteTokenAccount)
	if !ok {
		// Try to get the lite identity
		lid, ok := account.(*protocol.LiteIdentity)
		if !ok {
			return fmt.Errorf("%w: account %v is not a lite token account or lite identity", protocol.ErrInvalidSignature, signer)
		}
		
		// Get the ACME token account
		acmeUrl := url.JoinPath(lid.Url(), "acme")
		account, err = batch.Account(acmeUrl).Main().Get()
		if err != nil {
			return fmt.Errorf("%w: failed to load ACME token account: %v", protocol.ErrInvalidSignature, err)
		}
		
		// Check if the account is a lite token account
		lta, ok = account.(*protocol.LiteTokenAccount)
		if !ok {
			return fmt.Errorf("%w: account %v is not a lite token account", protocol.ErrInvalidSignature, acmeUrl)
		}
	}
	
	// Update the balance
	lta.Balance += amount
	
	// Save the account
	err = batch.Account(lta.Url()).Main().Put(lta)
	if err != nil {
		return fmt.Errorf("%w: failed to save account: %v", protocol.ErrInvalidSignature, err)
	}
	
	return nil
}

// GetCurrentWindow returns the current mining window
func (v *LxrMiningValidator) GetCurrentWindow() *mining.MiningWindow {
	return v.registry.GetCurrentWindow()
}
