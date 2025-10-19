package chain

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// EpochManager manages mining epoch lifecycle and difficulty adjustments
type EpochManager struct {
	mutex sync.RWMutex
	
	// Current State
	currentEpoch        *MiningEpoch
	epochHistory        []*MiningEpoch
	issuanceAccounts    map[string]*url.URL  // token_url -> issuance_account
	
	// Difficulty Adjustment Configuration
	targetSubmissionRate float64    // Target submissions per block
	difficultyWindow     uint64     // Blocks to look back for adjustment
	maxDifficultyChange  float64    // Maximum change per adjustment (e.g., 2.0 = 200%)
	minDifficultyTarget  *big.Int   // Minimum difficulty (easiest)
	maxDifficultyTarget  *big.Int   // Maximum difficulty (hardest)
	
	// DN Integration
	dnAnchorProvider    DNAnchorProvider
	
	// Configuration
	epochDurationBlocks uint64      // Blocks per epoch
	submissionWindow    uint64      // Blocks for submissions within epoch
	maxEpochHistory     uint64      // Maximum epochs to keep in history
	
	// Mining Validator Integration
	validator           *MiningValidator
	
	// Statistics
	totalEpochsCreated  uint64
	lastDifficultyAdjustment time.Time
}

// MiningEpoch represents a complete mining epoch with all associated data
type MiningEpoch struct {
	// Epoch Identity
	EpochNumber     uint64    `json:"epochNumber"`
	StartBlock      uint64    `json:"startBlock"`
	EndBlock        uint64    `json:"endBlock"`
	CreatedAt       time.Time `json:"createdAt"`
	FinalizedAt     *time.Time `json:"finalizedAt,omitempty"`
	
	// Mining Parameters
	BaselineTarget  []byte    `json:"baselineTarget"`    // 32-byte difficulty target
	DNAnchorHash    []byte    `json:"dnAnchorHash"`      // Directory Network anchor
	SubmissionWindow [2]uint64 `json:"submissionWindow"` // [start_block, end_block]
	
	// Submission Tracking
	TotalSubmissions uint64                `json:"totalSubmissions"`
	ValidSubmissions uint64                `json:"validSubmissions"`
	TopNWinners     []*MiningSubmission    `json:"topNWinners,omitempty"`
	
	// Performance Metrics
	AverageHashTime  time.Duration         `json:"averageHashTime"`
	HashesPerSecond  float64              `json:"hashesPerSecond"`
	MinerCount       uint64               `json:"minerCount"`
	CompetitionRatio float64              `json:"competitionRatio"` // valid_submissions / top_n_size
	
	// Reward Distribution
	RewardPerWinner      *big.Int          `json:"rewardPerWinner,omitempty"`
	TotalRewardsIssued   *big.Int          `json:"totalRewardsIssued,omitempty"`
	RewardDistribution   []*RewardPayout   `json:"rewardDistribution,omitempty"`
	
	// Status and Metrics
	Status              EpochStatus       `json:"status"`
	SubmissionDeadline  time.Time        `json:"submissionDeadline"`
	FinalizeDeadline    time.Time        `json:"finalizeDeadline"`
}

// EpochStatus represents the current state of a mining epoch
type EpochStatus int

const (
	EpochStatusPending EpochStatus = iota  // Created but not yet active
	EpochStatusActive                      // Currently accepting submissions
	EpochStatusSubmissionsClosed           // No longer accepting submissions
	EpochStatusFinalizing                  // Processing final results
	EpochStatusCompleted                   // Finalized with rewards distributed
	EpochStatusExpired                     // Expired without completion
)

func (s EpochStatus) String() string {
	switch s {
	case EpochStatusPending:
		return "pending"
	case EpochStatusActive:
		return "active"
	case EpochStatusSubmissionsClosed:
		return "submissions_closed"
	case EpochStatusFinalizing:
		return "finalizing"
	case EpochStatusCompleted:
		return "completed"
	case EpochStatusExpired:
		return "expired"
	default:
		return "unknown"
	}
}

// DNAnchorProvider interface for Directory Network integration
type DNAnchorProvider interface {
	GetCurrentAnchor() ([]byte, error)
	GetAnchorAtBlock(blockHeight uint64) ([]byte, error)
	GetCurrentBlockHeight() (uint64, error)
	SubscribeToAnchors() (<-chan AnchorUpdate, error)
}

// AnchorUpdate represents a Directory Network anchor change
type AnchorUpdate struct {
	BlockHeight uint64
	AnchorHash  []byte
	Timestamp   time.Time
}

// EpochManagerConfig contains configuration for the epoch manager
type EpochManagerConfig struct {
	// Epoch Duration
	EpochDurationBlocks uint64        `json:"epochDurationBlocks"`  // Default: 1000 blocks
	SubmissionWindow    uint64        `json:"submissionWindow"`     // Default: 900 blocks
	
	// Difficulty Adjustment
	TargetSubmissionRate float64      `json:"targetSubmissionRate"` // Default: 10.0 submissions/block
	DifficultyWindow     uint64       `json:"difficultyWindow"`     // Default: 10 epochs
	MaxDifficultyChange  float64      `json:"maxDifficultyChange"`  // Default: 2.0 (200%)
	
	// History Management
	MaxEpochHistory      uint64       `json:"maxEpochHistory"`      // Default: 100 epochs
	
	// Integration
	ValidatorConfig      *MiningValidatorConfig `json:"validatorConfig,omitempty"`
}

// DefaultEpochManagerConfig returns default configuration
func DefaultEpochManagerConfig() *EpochManagerConfig {
	return &EpochManagerConfig{
		EpochDurationBlocks:  1000,   // ~16.7 minutes at 1 block/second
		SubmissionWindow:     900,    // 90% of epoch for submissions
		TargetSubmissionRate: 10.0,   // 10 submissions per block target
		DifficultyWindow:     10,     // Look back 10 epochs for adjustment
		MaxDifficultyChange:  2.0,    // Maximum 200% difficulty change
		MaxEpochHistory:      100,    // Keep 100 epochs in memory
		ValidatorConfig:      DefaultMiningValidatorConfig(),
	}
}

// NewEpochManager creates a new mining epoch manager
func NewEpochManager(config *EpochManagerConfig, dnProvider DNAnchorProvider) *EpochManager {
	if config == nil {
		config = DefaultEpochManagerConfig()
	}
	
	// Initialize difficulty bounds (Bitcoin-compatible 256-bit)
	minTarget := big.NewInt(1)
	maxTarget := new(big.Int)
	maxTarget.SetString("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 16)
	
	em := &EpochManager{
		epochHistory:         make([]*MiningEpoch, 0, config.MaxEpochHistory),
		issuanceAccounts:     make(map[string]*url.URL),
		targetSubmissionRate: config.TargetSubmissionRate,
		difficultyWindow:     config.DifficultyWindow,
		maxDifficultyChange:  config.MaxDifficultyChange,
		minDifficultyTarget:  minTarget,
		maxDifficultyTarget:  maxTarget,
		dnAnchorProvider:     dnProvider,
		epochDurationBlocks:  config.EpochDurationBlocks,
		submissionWindow:     config.SubmissionWindow,
		maxEpochHistory:      config.MaxEpochHistory,
		validator:            NewMiningValidator(config.ValidatorConfig),
	}
	
	return em
}

// InitializeNewEpoch creates and initializes a new mining epoch
func (em *EpochManager) InitializeNewEpoch() (*MiningEpoch, error) {
	em.mutex.Lock()
	defer em.mutex.Unlock()
	
	// Get current block height and DN anchor
	currentBlock, err := em.dnAnchorProvider.GetCurrentBlockHeight()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	
	dnAnchorHash, err := em.dnAnchorProvider.GetCurrentAnchor()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	
	if len(dnAnchorHash) != 32 {
		return nil, errors.BadRequest.WithFormat("DN anchor hash must be 32 bytes, got %d", len(dnAnchorHash))
	}
	
	// Calculate epoch number
	epochNumber := em.totalEpochsCreated + 1
	if em.currentEpoch != nil {
		epochNumber = em.currentEpoch.EpochNumber + 1
	}
	
	// Calculate epoch blocks
	startBlock := currentBlock
	endBlock := startBlock + em.epochDurationBlocks
	submissionEndBlock := startBlock + em.submissionWindow
	
	// Calculate new baseline difficulty target
	newBaselineTarget, err := em.calculateNewBaseline()
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	
	// Create new epoch
	newEpoch := &MiningEpoch{
		EpochNumber:      epochNumber,
		StartBlock:       startBlock,
		EndBlock:         endBlock,
		CreatedAt:        time.Now(),
		BaselineTarget:   newBaselineTarget,
		DNAnchorHash:     dnAnchorHash,
		SubmissionWindow: [2]uint64{startBlock, submissionEndBlock},
		Status:           EpochStatusPending,
		SubmissionDeadline: time.Now().Add(time.Duration(em.submissionWindow) * time.Second), // Assuming 1 second/block
		FinalizeDeadline:   time.Now().Add(time.Duration(em.epochDurationBlocks) * time.Second),
	}
	
	// Finalize previous epoch if exists
	if em.currentEpoch != nil && em.currentEpoch.Status != EpochStatusCompleted {
		if err := em.finalizeEpoch(em.currentEpoch); err != nil {
			return nil, errors.Wrap(errors.StatusUnknownError, err)
		}
	}
	
	// Initialize mining validator for new epoch
	err = em.validator.InitializeEpoch(
		epochNumber,
		newBaselineTarget,
		dnAnchorHash,
		newEpoch.SubmissionWindow,
	)
	if err != nil {
		return nil, errors.Wrap(errors.StatusUnknownError, err)
	}
	
	// Set as current epoch
	em.currentEpoch = newEpoch
	em.totalEpochsCreated++
	
	// Add to history
	em.addEpochToHistory(newEpoch)
	
	// Start the epoch
	newEpoch.Status = EpochStatusActive
	
	return newEpoch, nil
}

// FinalizeCurrentEpoch completes the current epoch and processes rewards
func (em *EpochManager) FinalizeCurrentEpoch() (*EpochFinalizeResult, error) {
	em.mutex.Lock()
	defer em.mutex.Unlock()
	
	if em.currentEpoch == nil {
		return nil, errors.BadRequest.WithFormat("no current epoch to finalize")
	}
	
	if em.currentEpoch.Status == EpochStatusCompleted {
		return nil, errors.BadRequest.WithFormat("epoch %d is already completed", em.currentEpoch.EpochNumber)
	}
	
	return em.finalizeEpoch(em.currentEpoch)
}

// GetCurrentEpoch returns the current active epoch
func (em *EpochManager) GetCurrentEpoch() *MiningEpoch {
	em.mutex.RLock()
	defer em.mutex.RUnlock()
	
	if em.currentEpoch == nil {
		return nil
	}
	
	// Return a copy to prevent external modification
	epochCopy := *em.currentEpoch
	return &epochCopy
}

// GetEpochHistory returns the historical epochs
func (em *EpochManager) GetEpochHistory(limit uint64) []*MiningEpoch {
	em.mutex.RLock()
	defer em.mutex.RUnlock()
	
	// Determine how many epochs to return
	historyLen := uint64(len(em.epochHistory))
	if limit > 0 && limit < historyLen {
		historyLen = limit
	}
	
	// Return most recent epochs first
	result := make([]*MiningEpoch, historyLen)
	for i := uint64(0); i < historyLen; i++ {
		epochIndex := uint64(len(em.epochHistory)) - 1 - i
		epochCopy := *em.epochHistory[epochIndex]
		result[i] = &epochCopy
	}
	
	return result
}

// RegisterIssuanceAccount registers a mining token issuance account
func (em *EpochManager) RegisterIssuanceAccount(tokenURL *url.URL, issuanceAccount *url.URL) error {
	em.mutex.Lock()
	defer em.mutex.Unlock()
	
	em.issuanceAccounts[tokenURL.String()] = issuanceAccount
	return nil
}

// GetEpochStatistics returns comprehensive statistics for the current epoch
func (em *EpochManager) GetEpochStatistics() *EpochStatistics {
	em.mutex.RLock()
	defer em.mutex.RUnlock()
	
	if em.currentEpoch == nil {
		return nil
	}
	
	// Get validator statistics
	validatorStats := em.validator.GetEpochStatistics()
	
	// Enhance with epoch manager data
	stats := validatorStats
	stats.EpochDuration = time.Since(em.currentEpoch.CreatedAt)
	
	// Add difficulty information
	if len(em.currentEpoch.BaselineTarget) > 0 {
		stats.BaselineTarget = em.currentEpoch.BaselineTarget
	}
	
	return stats
}

// Helper methods

func (em *EpochManager) addEpochToHistory(epoch *MiningEpoch) {
	em.epochHistory = append(em.epochHistory, epoch)
	
	// Trim history if it exceeds maximum
	if uint64(len(em.epochHistory)) > em.maxEpochHistory {
		// Remove oldest epoch
		em.epochHistory = em.epochHistory[1:]
	}
}

// EpochFinalizeResult contains the results of epoch finalization
type EpochFinalizeResult struct {
	EpochNumber          uint64                      `json:"epochNumber"`
	WinnerCount          uint64                      `json:"winnerCount"`
	TotalRewardsIssued   *big.Int                    `json:"totalRewardsIssued"`
	SyntheticTransactions []*SyntheticMiningTransaction `json:"syntheticTransactions"`
	FinalizedAt          time.Time                   `json:"finalizedAt"`
	NextEpochNumber      uint64                      `json:"nextEpochNumber"`
}

// finalizeEpoch processes the completion of a mining epoch (requires mutex)
func (em *EpochManager) finalizeEpoch(epoch *MiningEpoch) (*EpochFinalizeResult, error) {
	// Mark epoch as finalizing
	epoch.Status = EpochStatusFinalizing
	
	// Get final winners from validator
	winners := em.validator.GetTopNWinners()
	epoch.TopNWinners = winners
	
	// Update epoch statistics
	epochStats := em.validator.GetEpochStatistics()
	epoch.TotalSubmissions = epochStats.TotalSubmissions
	epoch.ValidSubmissions = epochStats.ValidSubmissions
	epoch.MinerCount = uint64(len(winners))
	epoch.CompetitionRatio = float64(epochStats.ValidSubmissions) / float64(epochStats.TopNSize)
	
	// Process rewards if we have winners
	var syntheticTxs []*SyntheticMiningTransaction
	var totalRewards *big.Int
	
	if len(winners) > 0 {
		// Find issuance account for reward distribution
		// For now, use first registered issuance account
		var issuanceAccount *url.URL
		for _, account := range em.issuanceAccounts {
			issuanceAccount = account
			break
		}
		
		if issuanceAccount != nil {
			// Create reward distributor
			baseReward := big.NewInt(1000) // TODO: Make configurable
			distributor := NewMiningRewardDistributor(baseReward, EqualDistribution)
			
			// Process epoch rewards
			rewardResult, err := em.validator.ProcessEpochRewards(issuanceAccount, distributor)
			if err != nil {
				return nil, errors.Wrap(errors.StatusUnknownError, err)
			}
			
			syntheticTxs = rewardResult.SyntheticTransactions
			totalRewards = rewardResult.TotalRewardsIssued
			epoch.TotalRewardsIssued = totalRewards
		}
	}
	
	// Mark epoch as completed
	now := time.Now()
	epoch.Status = EpochStatusCompleted
	epoch.FinalizedAt = &now
	
	// Update last difficulty adjustment time
	em.lastDifficultyAdjustment = now
	
	result := &EpochFinalizeResult{
		EpochNumber:           epoch.EpochNumber,
		WinnerCount:           uint64(len(winners)),
		TotalRewardsIssued:    totalRewards,
		SyntheticTransactions: syntheticTxs,
		FinalizedAt:           now,
		NextEpochNumber:       epoch.EpochNumber + 1,
	}
	
	return result, nil
}