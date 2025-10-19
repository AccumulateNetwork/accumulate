package chain

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// SyntheticMiningTransaction represents a synthetic transaction for mining rewards
type SyntheticMiningTransaction struct {
	// Standard synthetic transaction fields
	Source      *url.URL `json:"source"`
	Destination *url.URL `json:"destination"`
	Amount      *big.Int `json:"amount"`
	
	// Mining-specific fields
	EpochNumber     uint64 `json:"epochNumber"`
	MinerADI        *url.URL `json:"minerADI"`
	SubmissionHash  []byte `json:"submissionHash"`
	Rank            uint64 `json:"rank"`
	ComputedHash    []byte `json:"computedHash"`
	
	// Transaction metadata
	TransactionHash []byte    `json:"transactionHash"`
	GeneratedAt     time.Time `json:"generatedAt"`
	RewardType      string    `json:"rewardType"` // "mining", "validation", "bonus"
}

// MiningRewardDistributor handles the distribution of mining rewards
type MiningRewardDistributor struct {
	// Reward Configuration
	baseRewardPerWinner *big.Int
	bonusRewardPool     *big.Int
	
	// Distribution Strategy
	strategy RewardStrategy
	
	// Payout Management
	payoutAccounts map[string]*url.URL  // miner_adi -> token_account
	pendingPayouts []*RewardPayout
	
	// Statistics
	totalRewardsDistributed *big.Int
	totalPayoutsSent        uint64
	epochRewardHistory      map[uint64]*EpochRewardSummary
}

// RewardStrategy defines different reward distribution methods
type RewardStrategy int

const (
	EqualDistribution RewardStrategy = iota
	ProportionalByHashQuality
	TieredByRanking
)

// RewardPayout represents a pending reward payout
type RewardPayout struct {
	MinerADI        *url.URL  `json:"minerADI"`
	TokenAccount    *url.URL  `json:"tokenAccount"`
	Amount          *big.Int  `json:"amount"`
	EpochNumber     uint64    `json:"epochNumber"`
	Rank            uint64    `json:"rank"`
	SubmissionHash  []byte    `json:"submissionHash"`
	RewardType      string    `json:"rewardType"`
	CreatedAt       time.Time `json:"createdAt"`
	PaidAt          *time.Time `json:"paidAt,omitempty"`
	TransactionHash []byte    `json:"transactionHash,omitempty"`
}

// EpochRewardSummary contains summary statistics for epoch rewards
type EpochRewardSummary struct {
	EpochNumber         uint64    `json:"epochNumber"`
	TotalRewardsIssued  *big.Int  `json:"totalRewardsIssued"`
	WinnerCount         uint64    `json:"winnerCount"`
	AverageReward       *big.Int  `json:"averageReward"`
	BonusRewardsIssued  *big.Int  `json:"bonusRewardsIssued"`
	DistributionTime    time.Time `json:"distributionTime"`
}

// NewMiningRewardDistributor creates a new reward distributor
func NewMiningRewardDistributor(baseReward *big.Int, strategy RewardStrategy) *MiningRewardDistributor {
	return &MiningRewardDistributor{
		baseRewardPerWinner:     new(big.Int).Set(baseReward),
		bonusRewardPool:         big.NewInt(0),
		strategy:                strategy,
		payoutAccounts:          make(map[string]*url.URL),
		pendingPayouts:          make([]*RewardPayout, 0),
		totalRewardsDistributed: big.NewInt(0),
		epochRewardHistory:      make(map[uint64]*EpochRewardSummary),
	}
}

// GenerateSyntheticTransactions creates synthetic transactions for mining reward payouts
func (mv *MiningValidator) GenerateSyntheticTransactions(
	winners []*MiningSubmission,
	issuanceAccount *url.URL,
	rewardDistributor *MiningRewardDistributor,
) ([]*SyntheticMiningTransaction, error) {
	
	if len(winners) == 0 {
		return nil, nil
	}
	
	// Calculate rewards for each winner
	payouts, err := rewardDistributor.CalculateRewards(winners, issuanceAccount)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("failed to calculate rewards: %w", err)
	}
	
	// Generate synthetic transactions
	syntheticTxs := make([]*SyntheticMiningTransaction, 0, len(payouts))
	
	for _, payout := range payouts {
		// Get or create mining token account for the miner
		minerTokenAccount, err := mv.getMinerTokenAccount(payout.MinerADI)
		if err != nil {
			// Log error but continue with other payouts
			continue
		}
		
		// Create synthetic transaction
		syntheticTx := &SyntheticMiningTransaction{
			Source:          issuanceAccount,
			Destination:     minerTokenAccount,
			Amount:          new(big.Int).Set(payout.Amount),
			EpochNumber:     payout.EpochNumber,
			MinerADI:        payout.MinerADI,
			SubmissionHash:  payout.SubmissionHash,
			Rank:            payout.Rank,
			RewardType:      payout.RewardType,
			GeneratedAt:     time.Now(),
		}
		
		// Find the corresponding winner to get computed hash
		for _, winner := range winners {
			if winner.MinerADI.Equal(payout.MinerADI) {
				syntheticTx.ComputedHash = winner.ComputedHash
				break
			}
		}
		
		// Generate transaction hash
		syntheticTx.TransactionHash = mv.generateSyntheticTransactionHash(syntheticTx)
		
		syntheticTxs = append(syntheticTxs, syntheticTx)
	}
	
	return syntheticTxs, nil
}

// CalculateRewards determines reward amounts for mining winners
func (rd *MiningRewardDistributor) CalculateRewards(
	winners []*MiningSubmission,
	issuanceAccount *url.URL,
) ([]*RewardPayout, error) {
	
	if len(winners) == 0 {
		return nil, nil
	}
	
	// Note: payouts variable not used due to switch statement
	_ = make([]*RewardPayout, 0, len(winners))
	
	switch rd.strategy {
	case EqualDistribution:
		return rd.calculateEqualDistribution(winners)
		
	case ProportionalByHashQuality:
		return rd.calculateProportionalDistribution(winners)
		
	case TieredByRanking:
		return rd.calculateTieredDistribution(winners)
		
	default:
		return rd.calculateEqualDistribution(winners)
	}
}

// calculateEqualDistribution gives equal rewards to all winners
func (rd *MiningRewardDistributor) calculateEqualDistribution(winners []*MiningSubmission) ([]*RewardPayout, error) {
	payouts := make([]*RewardPayout, 0, len(winners))
	
	for _, winner := range winners {
		payout := &RewardPayout{
			MinerADI:       winner.MinerADI,
			Amount:         new(big.Int).Set(rd.baseRewardPerWinner),
			EpochNumber:    winner.EpochNumber,
			Rank:           winner.Rank,
			SubmissionHash: winner.SubmissionHash,
			RewardType:     "mining",
			CreatedAt:      time.Now(),
		}
		
		payouts = append(payouts, payout)
	}
	
	return payouts, nil
}

// calculateProportionalDistribution gives rewards proportional to hash quality
func (rd *MiningRewardDistributor) calculateProportionalDistribution(winners []*MiningSubmission) ([]*RewardPayout, error) {
	if len(winners) == 0 {
		return nil, nil
	}
	
	// Calculate total reward pool
	totalRewardPool := new(big.Int).Mul(rd.baseRewardPerWinner, big.NewInt(int64(len(winners))))
	
	// Calculate hash quality scores (inverse of hash value for proportional rewards)
	// Better (smaller) hashes get higher scores
	hashScores := make([]*big.Int, len(winners))
	totalScore := big.NewInt(0)
	
	// Find the worst (largest) hash value to use as baseline
	worstHashValue := big.NewInt(0)
	for _, winner := range winners {
		hashValue := winner.HashValue()
		if hashValue.Cmp(worstHashValue) > 0 {
			worstHashValue = hashValue
		}
	}
	
	// Calculate scores as (worst_hash - current_hash + 1)
	for i, winner := range winners {
		hashValue := winner.HashValue()
		score := new(big.Int).Sub(worstHashValue, hashValue)
		score.Add(score, big.NewInt(1)) // Add 1 to avoid zero scores
		
		hashScores[i] = score
		totalScore.Add(totalScore, score)
	}
	
	// Distribute rewards proportionally
	payouts := make([]*RewardPayout, 0, len(winners))
	
	for i, winner := range winners {
		// Calculate proportional reward: (score / total_score) * total_pool
		rewardAmount := new(big.Int).Mul(totalRewardPool, hashScores[i])
		rewardAmount.Div(rewardAmount, totalScore)
		
		payout := &RewardPayout{
			MinerADI:       winner.MinerADI,
			Amount:         rewardAmount,
			EpochNumber:    winner.EpochNumber,
			Rank:           winner.Rank,
			SubmissionHash: winner.SubmissionHash,
			RewardType:     "mining",
			CreatedAt:      time.Now(),
		}
		
		payouts = append(payouts, payout)
	}
	
	return payouts, nil
}

// calculateTieredDistribution gives higher rewards to better ranks
func (rd *MiningRewardDistributor) calculateTieredDistribution(winners []*MiningSubmission) ([]*RewardPayout, error) {
	payouts := make([]*RewardPayout, 0, len(winners))
	
	// Define tier multipliers (rank 1 gets 2x, rank 2 gets 1.5x, others get 1x)
	getTierMultiplier := func(rank uint64) float64 {
		switch rank {
		case 1:
			return 2.0
		case 2:
			return 1.5
		case 3:
			return 1.2
		default:
			return 1.0
		}
	}
	
	for _, winner := range winners {
		multiplier := getTierMultiplier(winner.Rank)
		rewardAmount := new(big.Int).Set(rd.baseRewardPerWinner)
		
		// Apply multiplier (multiply by 100, then divide to handle decimals)
		multiplierInt := big.NewInt(int64(multiplier * 100))
		rewardAmount.Mul(rewardAmount, multiplierInt)
		rewardAmount.Div(rewardAmount, big.NewInt(100))
		
		payout := &RewardPayout{
			MinerADI:       winner.MinerADI,
			Amount:         rewardAmount,
			EpochNumber:    winner.EpochNumber,
			Rank:           winner.Rank,
			SubmissionHash: winner.SubmissionHash,
			RewardType:     "mining",
			CreatedAt:      time.Now(),
		}
		
		payouts = append(payouts, payout)
	}
	
	return payouts, nil
}

// getMinerTokenAccount returns the mining token account for a miner ADI
func (mv *MiningValidator) getMinerTokenAccount(minerADI *url.URL) (*url.URL, error) {
	// In a real implementation, this would:
	// 1. Query the state to find the miner's mining token account
	// 2. Create one if it doesn't exist
	// 3. Return the account URL
	
	// For now, generate a conventional mining token account URL
	miningAccountPath := fmt.Sprintf("%s/mining-tokens", minerADI.String())
	miningAccount, err := url.Parse(miningAccountPath)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("failed to parse mining account URL: %w", err)
	}
	
	return miningAccount, nil
}

// generateSyntheticTransactionHash creates a unique hash for a synthetic transaction
func (mv *MiningValidator) generateSyntheticTransactionHash(syntheticTx *SyntheticMiningTransaction) []byte {
	h := sha256.New()
	h.Write([]byte(syntheticTx.Source.String()))
	h.Write([]byte(syntheticTx.Destination.String()))
	h.Write(syntheticTx.Amount.Bytes())
	h.Write(syntheticTx.SubmissionHash)
	h.Write([]byte(fmt.Sprintf("%d", syntheticTx.EpochNumber)))
	h.Write([]byte(fmt.Sprintf("%d", syntheticTx.GeneratedAt.Unix())))
	return h.Sum(nil)
}

// ProcessEpochRewards handles the complete reward distribution for an epoch
func (mv *MiningValidator) ProcessEpochRewards(
	issuanceAccount *url.URL,
	rewardDistributor *MiningRewardDistributor,
) (*EpochRewardProcessingResult, error) {
	
	mv.mutex.Lock()
	defer mv.mutex.Unlock()
	
	// Get the current top-N winners
	winners := mv.priorityQueue.GetTopN()
	if len(winners) == 0 {
		return &EpochRewardProcessingResult{
			EpochNumber:      mv.currentEpoch,
			WinnerCount:      0,
			TotalRewardsIssued: big.NewInt(0),
			Message:          "No valid submissions to reward",
		}, nil
	}
	
	// Generate synthetic transactions
	syntheticTxs, err := mv.GenerateSyntheticTransactions(winners, issuanceAccount, rewardDistributor)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("failed to generate synthetic transactions: %w", err)
	}
	
	// Calculate total rewards
	totalRewards := big.NewInt(0)
	for _, syntheticTx := range syntheticTxs {
		totalRewards.Add(totalRewards, syntheticTx.Amount)
	}
	
	// Update distributor statistics
	rewardDistributor.totalRewardsDistributed.Add(rewardDistributor.totalRewardsDistributed, totalRewards)
	rewardDistributor.totalPayoutsSent += uint64(len(syntheticTxs))
	
	// Create epoch reward summary
	avgReward := new(big.Int)
	if len(syntheticTxs) > 0 {
		avgReward.Div(totalRewards, big.NewInt(int64(len(syntheticTxs))))
	}
	
	summary := &EpochRewardSummary{
		EpochNumber:        mv.currentEpoch,
		TotalRewardsIssued: new(big.Int).Set(totalRewards),
		WinnerCount:        uint64(len(winners)),
		AverageReward:      avgReward,
		BonusRewardsIssued: big.NewInt(0), // TODO: Implement bonus rewards
		DistributionTime:   time.Now(),
	}
	
	rewardDistributor.epochRewardHistory[mv.currentEpoch] = summary
	
	return &EpochRewardProcessingResult{
		EpochNumber:        mv.currentEpoch,
		WinnerCount:        uint64(len(winners)),
		TotalRewardsIssued: new(big.Int).Set(totalRewards),
		SyntheticTransactions: syntheticTxs,
		RewardSummary:      summary,
		Message:            fmt.Sprintf("Successfully distributed rewards to %d miners", len(winners)),
	}, nil
}

// EpochRewardProcessingResult contains the results of epoch reward processing
type EpochRewardProcessingResult struct {
	EpochNumber           uint64                        `json:"epochNumber"`
	WinnerCount           uint64                        `json:"winnerCount"`
	TotalRewardsIssued    *big.Int                      `json:"totalRewardsIssued"`
	SyntheticTransactions []*SyntheticMiningTransaction `json:"syntheticTransactions"`
	RewardSummary         *EpochRewardSummary          `json:"rewardSummary"`
	Message               string                        `json:"message"`
}