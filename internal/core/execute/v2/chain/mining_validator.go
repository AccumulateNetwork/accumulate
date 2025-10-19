package chain

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/big"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MiningValidator manages mining submission validation and consensus
type MiningValidator struct {
	mutex sync.RWMutex
	
	// Priority Queue Management
	priorityQueue   *MiningPriorityQueue
	topNSize        uint64
	
	// Current Epoch State
	currentEpoch    uint64
	baselineTarget  []byte  // 32-byte difficulty target
	dnAnchorHash    []byte  // Directory Network anchor hash
	submissionWindow [2]uint64  // [start_block, end_block]
	
	// Validation State
	validSubmissions map[string]*MiningSubmission // submission_hash -> submission
	totalSubmissions uint64
	epochStartTime   time.Time
	
	// Consensus Tracking
	transactionBodyVotes map[string]uint64  // transaction_hash -> vote_count
	majorityThreshold    uint64             // Required votes for consensus
	consensusReached     map[string]bool    // transaction_hash -> consensus_status
	
	// Configuration
	config *MiningValidatorConfig
}

// MiningValidatorConfig contains configuration for the mining validator
type MiningValidatorConfig struct {
	TopNSize                uint64        `json:"topNSize"`
	MajorityThreshold       uint64        `json:"majorityThreshold"`
	MaxSubmissionsPerMiner  uint64        `json:"maxSubmissionsPerMiner"`
	SubmissionTimeoutBlocks uint64        `json:"submissionTimeoutBlocks"`
	RequireTransactionBody  bool          `json:"requireTransactionBody"`
	EnableConsensusTracking bool          `json:"enableConsensusTracking"`
}

// DefaultMiningValidatorConfig returns default configuration
func DefaultMiningValidatorConfig() *MiningValidatorConfig {
	return &MiningValidatorConfig{
		TopNSize:                10,
		MajorityThreshold:       6,  // 60% majority
		MaxSubmissionsPerMiner:  1,  // One submission per miner per epoch
		SubmissionTimeoutBlocks: 100,
		RequireTransactionBody:  false,
		EnableConsensusTracking: true,
	}
}

// NewMiningValidator creates a new mining validator instance
func NewMiningValidator(config *MiningValidatorConfig) *MiningValidator {
	if config == nil {
		config = DefaultMiningValidatorConfig()
	}
	
	return &MiningValidator{
		priorityQueue:        NewMiningPriorityQueue(config.TopNSize),
		topNSize:             config.TopNSize,
		validSubmissions:     make(map[string]*MiningSubmission),
		transactionBodyVotes: make(map[string]uint64),
		consensusReached:     make(map[string]bool),
		config:               config,
	}
}

// InitializeEpoch sets up a new mining epoch
func (mv *MiningValidator) InitializeEpoch(epochNumber uint64, baselineTarget []byte, dnAnchorHash []byte, submissionWindow [2]uint64) error {
	mv.mutex.Lock()
	defer mv.mutex.Unlock()
	
	// Validate parameters
	if len(baselineTarget) != 32 {
		return errors.BadRequest.WithFormat("baseline target must be 32 bytes, got %d", len(baselineTarget))
	}
	if len(dnAnchorHash) != 32 {
		return errors.BadRequest.WithFormat("DN anchor hash must be 32 bytes, got %d", len(dnAnchorHash))
	}
	if submissionWindow[1] <= submissionWindow[0] {
		return errors.BadRequest.WithFormat("invalid submission window: end block %d <= start block %d", submissionWindow[1], submissionWindow[0])
	}
	
	// Reset state for new epoch
	mv.currentEpoch = epochNumber
	mv.baselineTarget = bytes.Clone(baselineTarget)
	mv.dnAnchorHash = bytes.Clone(dnAnchorHash)
	mv.submissionWindow = submissionWindow
	mv.epochStartTime = time.Now()
	
	// Clear previous epoch data
	mv.priorityQueue.Clear()
	mv.validSubmissions = make(map[string]*MiningSubmission)
	mv.transactionBodyVotes = make(map[string]uint64)
	mv.consensusReached = make(map[string]bool)
	mv.totalSubmissions = 0
	
	return nil
}

// ValidateAndSubmit validates a mining transaction and adds it to the priority queue if valid
func (mv *MiningValidator) ValidateAndSubmit(miningTx *protocol.MiningTransaction) (*MiningValidationResult, error) {
	mv.mutex.Lock()
	defer mv.mutex.Unlock()
	
	mv.totalSubmissions++
	
	// Create submission record
	submission := &MiningSubmission{
		MinerADI:        miningTx.MinerADI,
		BoundNonce:      bytes.Clone(miningTx.BoundNonce),
		TransactionData: bytes.Clone(miningTx.TransactionData),
		BlockHash:       bytes.Clone(miningTx.BlockHash),
		Timestamp:       miningTx.Timestamp,
		EpochNumber:     miningTx.EpochNumber,
		IsValid:         false,
	}
	
	// Generate submission hash for tracking
	submissionHash := mv.generateSubmissionHash(miningTx)
	submission.SubmissionHash = submissionHash
	
	// Validate submission
	result := &MiningValidationResult{
		SubmissionHash: submissionHash,
		MinerADI:       miningTx.MinerADI,
		EpochNumber:    miningTx.EpochNumber,
		IsAccepted:     false,
		Timestamp:      time.Now(),
	}
	
	// Check epoch validity
	if miningTx.EpochNumber != mv.currentEpoch {
		result.ErrorMessage = fmt.Sprintf("invalid epoch number: got %d, expected %d", miningTx.EpochNumber, mv.currentEpoch)
		return result, nil
	}
	
	// Check if submission window is open
	// Note: In real implementation, this would check current block height
	// For now, we'll assume submissions are always within window
	
	// Check for duplicate submissions from same miner
	if mv.config.MaxSubmissionsPerMiner > 0 {
		existingCount := mv.countSubmissionsFromMiner(miningTx.MinerADI)
		if existingCount >= mv.config.MaxSubmissionsPerMiner {
			result.ErrorMessage = fmt.Sprintf("miner %s has already submitted %d times (limit: %d)", 
				miningTx.MinerADI, existingCount, mv.config.MaxSubmissionsPerMiner)
			return result, nil
		}
	}
	
	// Validate bound nonce
	if err := mv.validateBoundNonce(miningTx); err != nil {
		result.ErrorMessage = fmt.Sprintf("bound nonce validation failed: %v", err)
		return result, nil
	}
	
	// Validate block hash
	if err := mv.validateBlockHash(miningTx); err != nil {
		result.ErrorMessage = fmt.Sprintf("block hash validation failed: %v", err)
		return result, nil
	}
	
	// Compute and validate proof-of-work
	computedHash, err := mv.validateProofOfWork(miningTx)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("proof-of-work validation failed: %v", err)
		return result, nil
	}
	
	submission.ComputedHash = computedHash
	submission.IsValid = true
	
	// Handle transaction body consensus if enabled
	if mv.config.EnableConsensusTracking && len(miningTx.TransactionBody) > 0 {
		mv.updateTransactionBodyConsensus(miningTx.TransactionBody)
		submission.TransactionBodyHash = sha256.Sum256(miningTx.TransactionBody)[:]
	}
	
	// Store valid submission
	mv.validSubmissions[string(submissionHash)] = submission
	
	// Try to add to priority queue
	accepted := mv.priorityQueue.InsertOrReplace(submission)
	
	result.IsAccepted = accepted
	result.ComputedHash = computedHash
	result.CurrentRank = submission.Rank
	
	if accepted {
		result.Message = "Mining submission accepted into top-N queue"
	} else {
		result.Message = fmt.Sprintf("Mining submission valid but not in top-%d (queue full with better submissions)", mv.topNSize)
	}
	
	return result, nil
}

// GetTopNWinners returns the current top-N mining submissions
func (mv *MiningValidator) GetTopNWinners() []*MiningSubmission {
	mv.mutex.RLock()
	defer mv.mutex.RUnlock()
	
	return mv.priorityQueue.GetTopN()
}

// GetEpochStatistics returns statistics for the current epoch
func (mv *MiningValidator) GetEpochStatistics() *EpochStatistics {
	mv.mutex.RLock()
	defer mv.mutex.RUnlock()
	
	queueStats := mv.priorityQueue.GetStatistics()
	
	stats := &EpochStatistics{
		EpochNumber:      mv.currentEpoch,
		TotalSubmissions: mv.totalSubmissions,
		ValidSubmissions: queueStats.TotalValid,
		TopNSize:         mv.topNSize,
		CurrentTopN:      queueStats.CurrentSize,
		EpochStartTime:   mv.epochStartTime,
		EpochDuration:    time.Since(mv.epochStartTime),
		BaselineTarget:   bytes.Clone(mv.baselineTarget),
		DNAnchorHash:     bytes.Clone(mv.dnAnchorHash),
		SubmissionWindow: mv.submissionWindow,
	}
	
	if queueStats.BestHash != nil {
		stats.BestHash = bytes.Clone(queueStats.BestHash)
	}
	if queueStats.WorstHash != nil {
		stats.WorstHash = bytes.Clone(queueStats.WorstHash)
	}
	
	// Add consensus statistics
	if mv.config.EnableConsensusTracking {
		stats.TransactionBodyVotes = make(map[string]uint64)
		for hash, votes := range mv.transactionBodyVotes {
			stats.TransactionBodyVotes[hash] = votes
		}
		
		stats.ConsensusReached = make(map[string]bool)
		for hash, reached := range mv.consensusReached {
			stats.ConsensusReached[hash] = reached
		}
	}
	
	return stats
}

// Helper methods

func (mv *MiningValidator) generateSubmissionHash(miningTx *protocol.MiningTransaction) []byte {
	// Create unique hash for this submission
	h := sha256.New()
	h.Write(miningTx.BoundNonce)
	h.Write(miningTx.TransactionData)
	h.Write(miningTx.BlockHash)
	h.Write(miningTx.MinerADI.Bytes())
	return h.Sum(nil)
}

func (mv *MiningValidator) countSubmissionsFromMiner(minerADI *url.URL) uint64 {
	count := uint64(0)
	for _, submission := range mv.validSubmissions {
		if submission.MinerADI.Equal(minerADI) {
			count++
		}
	}
	return count
}

func (mv *MiningValidator) validateBoundNonce(miningTx *protocol.MiningTransaction) error {
	// Extract the expected ADI hash (last 32 bytes of bound nonce)
	if len(miningTx.BoundNonce) < 32 {
		return errors.BadRequest.WithFormat("bound nonce too short: %d bytes", len(miningTx.BoundNonce))
	}
	
	expectedADIHash := miningTx.BoundNonce[len(miningTx.BoundNonce)-32:]
	
	// Compute SHA256(miner_ADI)
	minerADIBytes := []byte(miningTx.MinerADI.String())
	actualADIHash := sha256.Sum256(minerADIBytes)
	
	// Verify the bound nonce ends with the correct ADI hash
	if !bytes.Equal(expectedADIHash, actualADIHash[:]) {
		return errors.BadRequest.WithFormat("bound nonce does not match SHA256(miner_ADI)")
	}
	
	return nil
}

func (mv *MiningValidator) validateBlockHash(miningTx *protocol.MiningTransaction) error {
	// Validate against current DN anchor hash
	if !bytes.Equal(miningTx.BlockHash, mv.dnAnchorHash) {
		return errors.BadRequest.WithFormat("block hash does not match current DN anchor")
	}
	
	return nil
}

func (mv *MiningValidator) validateProofOfWork(miningTx *protocol.MiningTransaction) ([]byte, error) {
	// Prepare hash input: bound_nonce + transaction_data + block_hash
	totalLen := len(miningTx.BoundNonce) + len(miningTx.TransactionData) + len(miningTx.BlockHash)
	hashInput := make([]byte, 0, totalLen)
	hashInput = append(hashInput, miningTx.BoundNonce...)
	hashInput = append(hashInput, miningTx.TransactionData...)
	hashInput = append(hashInput, miningTx.BlockHash...)
	
	// Compute LXRHash (using SHA256 as placeholder)
	// TODO: Replace with actual LXRHash implementation from exp/lxrand
	computedHash := sha256.Sum256(hashInput)
	
	// Convert baseline target to big.Int
	baselineTarget := new(big.Int).SetBytes(mv.baselineTarget)
	
	// Convert hash to big.Int for comparison
	hashValue := new(big.Int).SetBytes(computedHash[:])
	
	// Check if computed_hash < baseline_target
	if hashValue.Cmp(baselineTarget) >= 0 {
		return nil, errors.BadRequest.WithFormat("proof-of-work does not meet baseline difficulty: hash %x >= target %x", 
			computedHash, mv.baselineTarget)
	}
	
	return computedHash[:], nil
}

func (mv *MiningValidator) updateTransactionBodyConsensus(transactionBody []byte) {
	bodyHash := fmt.Sprintf("%x", sha256.Sum256(transactionBody))
	
	mv.transactionBodyVotes[bodyHash]++
	
	// Check if consensus threshold is reached
	if mv.transactionBodyVotes[bodyHash] >= mv.majorityThreshold {
		mv.consensusReached[bodyHash] = true
	}
}

// Data structures for results and statistics

type MiningValidationResult struct {
	SubmissionHash []byte    `json:"submissionHash"`
	MinerADI       *url.URL  `json:"minerADI"`
	EpochNumber    uint64    `json:"epochNumber"`
	IsAccepted     bool      `json:"isAccepted"`
	ComputedHash   []byte    `json:"computedHash,omitempty"`
	CurrentRank    uint64    `json:"currentRank,omitempty"`
	Message        string    `json:"message,omitempty"`
	ErrorMessage   string    `json:"errorMessage,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
}

type EpochStatistics struct {
	EpochNumber           uint64            `json:"epochNumber"`
	TotalSubmissions      uint64            `json:"totalSubmissions"`
	ValidSubmissions      uint64            `json:"validSubmissions"`
	TopNSize              uint64            `json:"topNSize"`
	CurrentTopN           uint64            `json:"currentTopN"`
	EpochStartTime        time.Time         `json:"epochStartTime"`
	EpochDuration         time.Duration     `json:"epochDuration"`
	BaselineTarget        []byte            `json:"baselineTarget"`
	DNAnchorHash          []byte            `json:"dnAnchorHash"`
	SubmissionWindow      [2]uint64         `json:"submissionWindow"`
	BestHash              []byte            `json:"bestHash,omitempty"`
	WorstHash             []byte            `json:"worstHash,omitempty"`
	TransactionBodyVotes  map[string]uint64 `json:"transactionBodyVotes,omitempty"`
	ConsensusReached      map[string]bool   `json:"consensusReached,omitempty"`
}