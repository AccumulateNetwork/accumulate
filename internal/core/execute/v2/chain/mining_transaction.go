// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/big"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type MiningTransaction struct{}

var (
	// Global mining validator instance
	globalMiningValidator *MiningValidator
	miningValidatorMutex  sync.Once
)

// GetMiningValidator returns the global mining validator instance
func GetMiningValidator() *MiningValidator {
	miningValidatorMutex.Do(func() {
		config := DefaultMiningValidatorConfig()
		globalMiningValidator = NewMiningValidator(config)
	})
	return globalMiningValidator
}

func (MiningTransaction) Type() protocol.TransactionType { return protocol.TransactionTypeMining }

func (x MiningTransaction) Validate(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	_, err := x.check(st, tx)
	return nil, err
}

func (MiningTransaction) check(st *StateManager, tx *Delivery) (*protocol.MiningTransaction, error) {
	body, ok := tx.Transaction.Body.(*protocol.MiningTransaction)
	if !ok {
		return nil, fmt.Errorf("invalid payload: want %T, got %T", new(protocol.MiningTransaction), tx.Transaction.Body)
	}

	// Validate required fields
	if len(body.BoundNonce) == 0 {
		return nil, errors.BadRequest.WithFormat("bound nonce is required")
	}
	if len(body.TransactionData) == 0 {
		return nil, errors.BadRequest.WithFormat("transaction data is required")
	}
	if len(body.BlockHash) == 0 {
		return nil, errors.BadRequest.WithFormat("block hash is required")
	}
	if len(body.BaselineTarget) == 0 {
		return nil, errors.BadRequest.WithFormat("baseline target is required")
	}
	if len(body.BaselineTarget) != 32 {
		return nil, errors.BadRequest.WithFormat("baseline target must be 32 bytes")
	}
	if body.MinerADI == nil {
		return nil, errors.BadRequest.WithFormat("miner ADI is required")
	}
	if body.Timestamp == 0 {
		return nil, errors.BadRequest.WithFormat("timestamp is required")
	}
	if body.EpochNumber == 0 {
		return nil, errors.BadRequest.WithFormat("epoch number is required")
	}

	// Validate timestamp is within reasonable bounds (not too far in past/future)
	// This prevents timestamp manipulation attacks while allowing for network latency
	const maxTimestampSkew = 300 // 5 minutes in seconds
	currentTime := uint64(st.BlockTimestamp().Unix())
	if body.Timestamp > currentTime+maxTimestampSkew {
		return nil, errors.BadRequest.WithFormat("timestamp too far in future: %d > %d", body.Timestamp, currentTime+maxTimestampSkew)
	}
	if body.Timestamp < currentTime-maxTimestampSkew {
		return nil, errors.BadRequest.WithFormat("timestamp too far in past: %d < %d", body.Timestamp, currentTime-maxTimestampSkew)
	}

	// Validate bound nonce format (should be nonce + SHA256(miner_ADI))
	if len(body.BoundNonce) < 32 {
		return nil, errors.BadRequest.WithFormat("bound nonce must be at least 32 bytes (nonce + SHA256(miner_ADI))")
	}

	return body, nil
}

func (x MiningTransaction) Execute(st *StateManager, tx *Delivery) (protocol.TransactionResult, error) {
	body, err := x.check(st, tx)
	if err != nil {
		return nil, err
	}

	// Get the global mining validator instance
	validator := GetMiningValidator()
	
	// Submit to mining validator for validation and priority queue management
	validationResult, err := validator.ValidateAndSubmit(body)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("mining validation failed: %w", err)
	}
	
	// Create transaction result with mining validation details
	result := &MiningTransactionResult{
		SubmissionHash: validationResult.SubmissionHash,
		IsAccepted:     validationResult.IsAccepted,
		ComputedHash:   validationResult.ComputedHash,
		CurrentRank:    validationResult.CurrentRank,
		Message:        validationResult.Message,
		ErrorMessage:   validationResult.ErrorMessage,
		EpochNumber:    validationResult.EpochNumber,
		MinerADI:       validationResult.MinerADI,
	}
	
	// If the submission was not accepted, still return success but with details
	// This allows miners to see why their submission wasn't accepted
	if !validationResult.IsAccepted && validationResult.ErrorMessage != "" {
		result.Message = fmt.Sprintf("Submission rejected: %s", validationResult.ErrorMessage)
	}
	
	return result, nil
}

// validateBoundNonce verifies that bound_nonce = nonce + SHA256(miner_ADI)
func (x MiningTransaction) validateBoundNonce(body *protocol.MiningTransaction) error {
	// Extract the expected ADI hash (last 32 bytes of bound nonce)
	if len(body.BoundNonce) < 32 {
		return errors.BadRequest.WithFormat("bound nonce too short")
	}

	expectedADIHash := body.BoundNonce[len(body.BoundNonce)-32:]

	// Compute SHA256(miner_ADI)
	minerADIBytes := []byte(body.MinerADI.String())
	actualADIHash := sha256.Sum256(minerADIBytes)

	// Verify the bound nonce ends with the correct ADI hash
	if !bytes.Equal(expectedADIHash, actualADIHash[:]) {
		return errors.BadRequest.WithFormat("bound nonce does not match SHA256(miner_ADI)")
	}

	return nil
}

// validateProofOfWork computes LXRHash and checks against baseline difficulty
func (x MiningTransaction) validateProofOfWork(body *protocol.MiningTransaction) error {
	// Prepare data for LXRHash: bound_nonce + transaction_data + block_hash
	totalLen := len(body.BoundNonce) + len(body.TransactionData) + len(body.BlockHash)
	hashInput := make([]byte, 0, totalLen)
	hashInput = append(hashInput, body.BoundNonce...)
	hashInput = append(hashInput, body.TransactionData...)
	hashInput = append(hashInput, body.BlockHash...)

	// Compute LXRHash
	// TODO: Replace with actual LXRHash implementation from exp/lxrand
	// For now, use SHA256 as placeholder
	computedHash := sha256.Sum256(hashInput)

	// Convert hash to big.Int for comparison
	hashValue := new(big.Int).SetBytes(computedHash[:])

	// Convert baseline target to big.Int
	baselineTarget := new(big.Int).SetBytes(body.BaselineTarget)

	// Check if computed_hash < baseline_target
	if hashValue.Cmp(baselineTarget) >= 0 {
		return errors.BadRequest.WithFormat("proof-of-work does not meet baseline difficulty: hash %x >= target %x", computedHash, body.BaselineTarget)
	}

	return nil
}

// validateBlockHash verifies the block hash matches current DN anchor
func (x MiningTransaction) validateBlockHash(st *StateManager, body *protocol.MiningTransaction) error {
	// TODO: Implement Directory Network anchor validation
	// This would:
	// 1. Get current DN anchor hash from state
	// 2. Verify it matches body.BlockHash
	// 3. Ensure the epoch number is correct

	// For now, accept any block hash (placeholder implementation)
	if len(body.BlockHash) != 32 {
		return errors.BadRequest.WithFormat("block hash must be 32 bytes")
	}

	return nil
}

// validateTransactionBody implements majority consensus for transaction body agreement
func (x MiningTransaction) validateTransactionBody(body *protocol.MiningTransaction) error {
	// Validate that CandidateTransactionHash matches TransactionBody (if both present)
	if len(body.CandidateTransactionHash) > 0 && len(body.TransactionBody) > 0 {
		computedHash := sha256.Sum256(body.TransactionBody)
		if !bytes.Equal(body.CandidateTransactionHash, computedHash[:]) {
			return errors.BadRequest.WithFormat("candidate transaction hash does not match transaction body")
		}
	}

	// TODO: Implement majority consensus mechanism
	// This would:
	// 1. Track votes for different transaction body hashes
	// 2. Require majority agreement before accepting
	// 3. Handle conflicting submissions

	return nil
}

// MiningTransactionResult represents the result of a mining transaction execution
type MiningTransactionResult struct {
	SubmissionHash []byte   `json:"submissionHash"`
	IsAccepted     bool     `json:"isAccepted"`
	ComputedHash   []byte   `json:"computedHash,omitempty"`
	CurrentRank    uint64   `json:"currentRank,omitempty"`
	Message        string   `json:"message,omitempty"`
	ErrorMessage   string   `json:"errorMessage,omitempty"`
	EpochNumber    uint64   `json:"epochNumber"`
	MinerADI       *url.URL `json:"minerADI"`
}

// Type implements protocol.TransactionResult
func (r *MiningTransactionResult) Type() protocol.TransactionType {
	return protocol.TransactionTypeMining
}
