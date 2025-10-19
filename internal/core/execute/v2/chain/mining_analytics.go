package chain

import (
	"math"
	"math/big"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// MiningAnalytics provides comprehensive mining performance analysis and monitoring
type MiningAnalytics struct {
	mutex sync.RWMutex
	
	// Historical Data
	epochMetrics      map[uint64]*EpochMetrics    // epoch_number -> metrics
	minerStats        map[string]*MinerStatistics // miner_adi -> stats
	networkStats      *NetworkStatistics
	
	// Real-time Monitoring
	currentPeriodStart time.Time
	realtimeMetrics   *RealtimeMetrics
	
	// Configuration
	retentionPeriod   time.Duration  // How long to keep historical data
	maxEpochsStored   uint64         // Maximum epochs to store
	
	// Analytics Configuration
	config           *AnalyticsConfig
}

// AnalyticsConfig contains configuration for mining analytics
type AnalyticsConfig struct {
	RetentionPeriod       time.Duration `json:"retentionPeriod"`       // 30 days default
	MaxEpochsStored       uint64        `json:"maxEpochsStored"`       // 1000 epochs default
	EnableRealtimeMetrics bool          `json:"enableRealtimeMetrics"` // Real-time monitoring
	MetricsUpdateInterval time.Duration `json:"metricsUpdateInterval"` // 10 seconds default
}

// EpochMetrics contains comprehensive metrics for a single epoch
type EpochMetrics struct {
	// Basic Information
	EpochNumber      uint64    `json:"epochNumber"`
	StartTime        time.Time `json:"startTime"`
	EndTime          time.Time `json:"endTime"`
	Duration         time.Duration `json:"duration"`
	
	// Submission Metrics
	TotalSubmissions      uint64    `json:"totalSubmissions"`
	ValidSubmissions      uint64    `json:"validSubmissions"`
	InvalidSubmissions    uint64    `json:"invalidSubmissions"`
	SubmissionRate        float64   `json:"submissionRate"`       // submissions per second
	ValidationRate        float64   `json:"validationRate"`       // valid / total ratio
	
	// Miner Participation
	UniqueMinerCount      uint64    `json:"uniqueMinerCount"`
	NewMinersCount        uint64    `json:"newMinersCount"`       // First-time miners
	ReturnMinerCount      uint64    `json:"returnMinerCount"`     // Returning miners
	AverageSubmissionsPerMiner float64 `json:"averageSubmissionsPerMiner"`
	
	// Competition Analysis
	TopNSize             uint64    `json:"topNSize"`
	CompetitionRatio     float64   `json:"competitionRatio"`     // valid_submissions / top_n
	WinnerThreshold      *big.Int  `json:"winnerThreshold,omitempty"` // Hash quality needed to win
	
	// Hash Quality Analysis
	BestHash             []byte    `json:"bestHash,omitempty"`
	WorstWinningHash     []byte    `json:"worstWinningHash,omitempty"`
	AverageHashQuality   *big.Int  `json:"averageHashQuality,omitempty"`
	HashQualityStdDev    *big.Float `json:"hashQualityStdDev,omitempty"`
	
	// Difficulty Metrics
	BaselineTarget       []byte    `json:"baselineTarget"`
	DifficultyValue      *big.Float `json:"difficultyValue,omitempty"`
	DifficultyAdjustment float64   `json:"difficultyAdjustment"` // Percentage change from previous
	
	// Performance Metrics
	AverageValidationTime time.Duration `json:"averageValidationTime"`
	PeakSubmissionRate   float64       `json:"peakSubmissionRate"`
	MinSubmissionRate    float64       `json:"minSubmissionRate"`
	
	// Reward Metrics
	TotalRewardsIssued   *big.Int  `json:"totalRewardsIssued,omitempty"`
	AverageRewardPerWinner *big.Int `json:"averageRewardPerWinner,omitempty"`
	RewardDistributionStrategy string `json:"rewardDistributionStrategy,omitempty"`
	
	// Economic Metrics
	EstimatedHashRate    float64   `json:"estimatedHashRate"`    // Hashes per second
	NetworkValue         *big.Int  `json:"networkValue,omitempty"` // Total value secured
	MiningCostEstimate   *big.Int  `json:"miningCostEstimate,omitempty"` // Estimated mining costs
}

// MinerStatistics tracks individual miner performance over time
type MinerStatistics struct {
	MinerADI             *url.URL  `json:"minerADI"`
	FirstEpoch           uint64    `json:"firstEpoch"`
	LastEpoch            uint64    `json:"lastEpoch"`
	TotalEpochsParticipated uint64 `json:"totalEpochsParticipated"`
	
	// Submission Statistics
	TotalSubmissions     uint64    `json:"totalSubmissions"`
	ValidSubmissions     uint64    `json:"validSubmissions"`
	SuccessRate          float64   `json:"successRate"`         // valid / total
	
	// Competition Performance
	TotalWins            uint64    `json:"totalWins"`
	WinRate              float64   `json:"winRate"`             // wins / participations
	AverageRank          float64   `json:"averageRank"`
	BestRank             uint64    `json:"bestRank"`
	
	// Hash Quality
	BestHashEver         []byte    `json:"bestHashEver,omitempty"`
	AverageHashQuality   *big.Int  `json:"averageHashQuality,omitempty"`
	HashQualityImprovement float64 `json:"hashQualityImprovement"` // Trend over time
	
	// Rewards
	TotalRewardsEarned   *big.Int  `json:"totalRewardsEarned,omitempty"`
	AverageRewardPerEpoch *big.Int `json:"averageRewardPerEpoch,omitempty"`
	
	// Consistency Metrics
	ParticipationConsistency float64 `json:"participationConsistency"` // How regularly they mine
	PerformanceConsistency   float64 `json:"performanceConsistency"`   // How consistent their performance
	
	// Recent Performance (last 10 epochs)
	RecentWinRate        float64   `json:"recentWinRate"`
	RecentAverageRank    float64   `json:"recentAverageRank"`
	RecentSubmissionRate float64   `json:"recentSubmissionRate"`
}

// NetworkStatistics provides network-wide mining statistics
type NetworkStatistics struct {
	// Network Health
	TotalEpochs          uint64    `json:"totalEpochs"`
	ActiveMiners         uint64    `json:"activeMiners"`          // Active in last 10 epochs
	TotalMinersEver      uint64    `json:"totalMinersEver"`
	NewMinerGrowthRate   float64   `json:"newMinerGrowthRate"`    // New miners per epoch
	
	// Hash Rate Analysis
	EstimatedNetworkHashRate float64 `json:"estimatedNetworkHashRate"` // Network hashes/sec
	HashRateGrowth           float64 `json:"hashRateGrowth"`           // Growth rate
	HashRateVolatility       float64 `json:"hashRateVolatility"`       // Standard deviation
	
	// Difficulty Trends
	CurrentDifficulty        *big.Float `json:"currentDifficulty,omitempty"`
	DifficultyTrend          float64    `json:"difficultyTrend"`          // Moving average slope
	AverageDifficultyChange  float64    `json:"averageDifficultyChange"`  // Average % change per epoch
	
	// Competition Metrics
	AverageCompetitionRatio  float64    `json:"averageCompetitionRatio"`
	CompetitionTrend         float64    `json:"competitionTrend"`         // Increasing/decreasing competition
	
	// Economic Metrics
	TotalRewardsIssued       *big.Int   `json:"totalRewardsIssued,omitempty"`
	AverageRewardPerEpoch    *big.Int   `json:"averageRewardPerEpoch,omitempty"`
	RewardEfficiency         float64    `json:"rewardEfficiency"`         // Rewards / estimated costs
	
	// Network Performance
	AverageEpochDuration     time.Duration `json:"averageEpochDuration"`
	EpochConsistency         float64       `json:"epochConsistency"`       // How consistent epoch timing
	SubmissionProcessingTime time.Duration `json:"submissionProcessingTime"` // Average validation time
	
	// Decentralization Metrics
	MinerConcentrationIndex  float64    `json:"minerConcentrationIndex"`  // Herfindahl index
	TopMinerHashShare        float64    `json:"topMinerHashShare"`        // Top miner's share of wins
	GeographicDistribution   map[string]uint64 `json:"geographicDistribution,omitempty"` // Country -> miner count
}

// RealtimeMetrics provides real-time mining performance indicators
type RealtimeMetrics struct {
	// Current Period Metrics
	PeriodStart          time.Time `json:"periodStart"`
	SubmissionsThisPeriod uint64   `json:"submissionsThisPeriod"`
	ValidSubmissionsThisPeriod uint64 `json:"validSubmissionsThisPeriod"`
	CurrentSubmissionRate float64  `json:"currentSubmissionRate"`
	
	// Moving Averages
	SubmissionRateMA5    float64   `json:"submissionRateMA5"`    // 5-minute moving average
	SubmissionRateMA15   float64   `json:"submissionRateMA15"`   // 15-minute moving average
	ValidationRateMA5    float64   `json:"validationRateMA5"`
	
	// Real-time Competition
	CurrentTopNFull      bool      `json:"currentTopNFull"`
	CurrentWorstWinningHash []byte `json:"currentWorstWinningHash,omitempty"`
	EstimatedTimeToWin   time.Duration `json:"estimatedTimeToWin"`
	
	// Network Activity
	ActiveMinersLast5Min uint64    `json:"activeMinersLast5Min"`
	ActiveMinersLast15Min uint64   `json:"activeMinersLast15Min"`
	PeakSubmissionRateToday float64 `json:"peakSubmissionRateToday"`
}

// DefaultAnalyticsConfig returns default analytics configuration
func DefaultAnalyticsConfig() *AnalyticsConfig {
	return &AnalyticsConfig{
		RetentionPeriod:       time.Hour * 24 * 30, // 30 days
		MaxEpochsStored:       1000,                // 1000 epochs
		EnableRealtimeMetrics: true,
		MetricsUpdateInterval: time.Second * 10,    // 10 seconds
	}
}

// NewMiningAnalytics creates a new mining analytics instance
func NewMiningAnalytics(config *AnalyticsConfig) *MiningAnalytics {
	if config == nil {
		config = DefaultAnalyticsConfig()
	}
	
	analytics := &MiningAnalytics{
		epochMetrics:       make(map[uint64]*EpochMetrics),
		minerStats:         make(map[string]*MinerStatistics),
		networkStats:       &NetworkStatistics{},
		currentPeriodStart: time.Now(),
		realtimeMetrics:    &RealtimeMetrics{PeriodStart: time.Now()},
		retentionPeriod:    config.RetentionPeriod,
		maxEpochsStored:    config.MaxEpochsStored,
		config:             config,
	}
	
	return analytics
}

// RecordEpochCompletion records metrics when an epoch is completed
func (ma *MiningAnalytics) RecordEpochCompletion(epoch *MiningEpoch, submissions []*MiningSubmission) error {
	ma.mutex.Lock()
	defer ma.mutex.Unlock()
	
	if epoch.FinalizedAt == nil {
		finalizedAt := time.Now()
		epoch.FinalizedAt = &finalizedAt
	}
	
	// Create epoch metrics
	metrics := &EpochMetrics{
		EpochNumber:      epoch.EpochNumber,
		StartTime:        epoch.CreatedAt,
		EndTime:          *epoch.FinalizedAt,
		Duration:         epoch.FinalizedAt.Sub(epoch.CreatedAt),
		TotalSubmissions: epoch.TotalSubmissions,
		ValidSubmissions: epoch.ValidSubmissions,
		TopNSize:         uint64(len(epoch.TopNWinners)),
		BaselineTarget:   epoch.BaselineTarget,
	}
	
	// Calculate derived metrics
	metrics.InvalidSubmissions = metrics.TotalSubmissions - metrics.ValidSubmissions
	if metrics.TotalSubmissions > 0 {
		metrics.ValidationRate = float64(metrics.ValidSubmissions) / float64(metrics.TotalSubmissions)
	}
	
	if metrics.Duration > 0 {
		metrics.SubmissionRate = float64(metrics.TotalSubmissions) / metrics.Duration.Seconds()
	}
	
	if metrics.TopNSize > 0 {
		metrics.CompetitionRatio = float64(metrics.ValidSubmissions) / float64(metrics.TopNSize)
	}
	
	// Analyze hash qualities
	ma.analyzeHashQualities(metrics, epoch.TopNWinners)
	
	// Calculate difficulty metrics
	ma.calculateDifficultyMetrics(metrics, epoch)
	
	// Update miner statistics
	ma.updateMinerStatistics(epoch, submissions)
	
	// Store epoch metrics
	ma.epochMetrics[epoch.EpochNumber] = metrics
	
	// Update network statistics
	ma.updateNetworkStatistics(metrics)
	
	// Clean up old data
	ma.cleanupOldData()
	
	return nil
}

// GetEpochMetrics returns metrics for a specific epoch
func (ma *MiningAnalytics) GetEpochMetrics(epochNumber uint64) *EpochMetrics {
	ma.mutex.RLock()
	defer ma.mutex.RUnlock()
	
	if metrics, exists := ma.epochMetrics[epochNumber]; exists {
		// Return a copy
		metricsCopy := *metrics
		return &metricsCopy
	}
	
	return nil
}

// GetMinerStatistics returns statistics for a specific miner
func (ma *MiningAnalytics) GetMinerStatistics(minerADI *url.URL) *MinerStatistics {
	ma.mutex.RLock()
	defer ma.mutex.RUnlock()
	
	if stats, exists := ma.minerStats[minerADI.String()]; exists {
		// Return a copy
		statsCopy := *stats
		return &statsCopy
	}
	
	return nil
}

// GetNetworkStatistics returns current network-wide statistics
func (ma *MiningAnalytics) GetNetworkStatistics() *NetworkStatistics {
	ma.mutex.RLock()
	defer ma.mutex.RUnlock()
	
	// Return a copy
	statsCopy := *ma.networkStats
	return &statsCopy
}

// GetRealtimeMetrics returns current real-time metrics
func (ma *MiningAnalytics) GetRealtimeMetrics() *RealtimeMetrics {
	ma.mutex.RLock()
	defer ma.mutex.RUnlock()
	
	// Return a copy
	metricsCopy := *ma.realtimeMetrics
	return &metricsCopy
}

// GetTopMiners returns top performing miners by various criteria
func (ma *MiningAnalytics) GetTopMiners(criteria string, limit int) []*MinerStatistics {
	ma.mutex.RLock()
	defer ma.mutex.RUnlock()
	
	// Collect all miner stats
	allMiners := make([]*MinerStatistics, 0, len(ma.minerStats))
	for _, stats := range ma.minerStats {
		statsCopy := *stats
		allMiners = append(allMiners, &statsCopy)
	}
	
	// Sort by criteria
	switch criteria {
	case "wins":
		// Sort by total wins (already implemented in slice sorting)
		for i := 0; i < len(allMiners)-1; i++ {
			for j := i + 1; j < len(allMiners); j++ {
				if allMiners[i].TotalWins < allMiners[j].TotalWins {
					allMiners[i], allMiners[j] = allMiners[j], allMiners[i]
				}
			}
		}
	case "winrate":
		// Sort by win rate
		for i := 0; i < len(allMiners)-1; i++ {
			for j := i + 1; j < len(allMiners); j++ {
				if allMiners[i].WinRate < allMiners[j].WinRate {
					allMiners[i], allMiners[j] = allMiners[j], allMiners[i]
				}
			}
		}
	case "rewards":
		// Sort by total rewards
		for i := 0; i < len(allMiners)-1; i++ {
			for j := i + 1; j < len(allMiners); j++ {
				if allMiners[i].TotalRewardsEarned.Cmp(allMiners[j].TotalRewardsEarned) < 0 {
					allMiners[i], allMiners[j] = allMiners[j], allMiners[i]
				}
			}
		}
	}
	
	// Apply limit
	if limit > 0 && limit < len(allMiners) {
		allMiners = allMiners[:limit]
	}
	
	return allMiners
}

// Helper methods for analytics calculations

func (ma *MiningAnalytics) analyzeHashQualities(metrics *EpochMetrics, winners []*MiningSubmission) {
	if len(winners) == 0 {
		return
	}
	
	// Find best and worst winning hashes
	var bestHash []byte
	var worstHash []byte
	hashQualities := make([]*big.Int, 0, len(winners))
	
	for _, winner := range winners {
		if len(winner.ComputedHash) > 0 {
			hashQuality := new(big.Int).SetBytes(winner.ComputedHash)
			hashQualities = append(hashQualities, hashQuality)
			
			if bestHash == nil || hashQuality.Cmp(new(big.Int).SetBytes(bestHash)) < 0 {
				bestHash = winner.ComputedHash
			}
			
			if worstHash == nil || hashQuality.Cmp(new(big.Int).SetBytes(worstHash)) > 0 {
				worstHash = winner.ComputedHash
			}
		}
	}
	
	metrics.BestHash = bestHash
	metrics.WorstWinningHash = worstHash
	
	// Calculate average hash quality
	if len(hashQualities) > 0 {
		sum := big.NewInt(0)
		for _, quality := range hashQualities {
			sum.Add(sum, quality)
		}
		metrics.AverageHashQuality = new(big.Int).Div(sum, big.NewInt(int64(len(hashQualities))))
		
		// Calculate standard deviation
		metrics.HashQualityStdDev = ma.calculateHashQualityStdDev(hashQualities, metrics.AverageHashQuality)
	}
}

func (ma *MiningAnalytics) calculateHashQualityStdDev(qualities []*big.Int, average *big.Int) *big.Float {
	if len(qualities) <= 1 {
		return big.NewFloat(0)
	}
	
	variance := big.NewFloat(0)
	avgFloat := new(big.Float).SetInt(average)
	n := big.NewFloat(float64(len(qualities)))
	
	for _, quality := range qualities {
		qualityFloat := new(big.Float).SetInt(quality)
		diff := new(big.Float).Sub(qualityFloat, avgFloat)
		diffSquared := new(big.Float).Mul(diff, diff)
		variance.Add(variance, diffSquared)
	}
	
	variance.Quo(variance, n)
	
	// Return standard deviation (square root of variance)
	stdDev := new(big.Float).Sqrt(variance)
	return stdDev
}

func (ma *MiningAnalytics) calculateDifficultyMetrics(metrics *EpochMetrics, epoch *MiningEpoch) {
	if len(epoch.BaselineTarget) == 32 {
		// Calculate difficulty value (max_target / current_target)
		maxTarget := new(big.Int)
		maxTarget.SetString("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 16)
		
		currentTarget := new(big.Int).SetBytes(epoch.BaselineTarget)
		if currentTarget.Cmp(big.NewInt(0)) > 0 {
			difficulty := new(big.Float).Quo(new(big.Float).SetInt(maxTarget), new(big.Float).SetInt(currentTarget))
			metrics.DifficultyValue = difficulty
		}
	}
	
	// Calculate difficulty adjustment percentage
	if epoch.EpochNumber > 1 {
		if previousMetrics, exists := ma.epochMetrics[epoch.EpochNumber-1]; exists {
			if previousMetrics.DifficultyValue != nil && metrics.DifficultyValue != nil {
				prevDiff, _ := previousMetrics.DifficultyValue.Float64()
				currDiff, _ := metrics.DifficultyValue.Float64()
				
				if prevDiff > 0 {
					metrics.DifficultyAdjustment = ((currDiff - prevDiff) / prevDiff) * 100
				}
			}
		}
	}
}

func (ma *MiningAnalytics) updateMinerStatistics(epoch *MiningEpoch, submissions []*MiningSubmission) {
	// Track miners from submissions
	minerSubmissions := make(map[string][]*MiningSubmission)
	
	for _, submission := range submissions {
		minerKey := submission.MinerADI.String()
		if _, exists := minerSubmissions[minerKey]; !exists {
			minerSubmissions[minerKey] = make([]*MiningSubmission, 0)
		}
		minerSubmissions[minerKey] = append(minerSubmissions[minerKey], submission)
	}
	
	// Update statistics for each miner
	for minerKey, minerSubs := range minerSubmissions {
		stats, exists := ma.minerStats[minerKey]
		if !exists {
			// Create new miner statistics
			minerADI, _ := url.Parse(minerKey)
			stats = &MinerStatistics{
				MinerADI:         minerADI,
				FirstEpoch:       epoch.EpochNumber,
				TotalRewardsEarned: big.NewInt(0),
			}
			ma.minerStats[minerKey] = stats
		}
		
		// Update basic stats
		stats.LastEpoch = epoch.EpochNumber
		stats.TotalEpochsParticipated++
		stats.TotalSubmissions += uint64(len(minerSubs))
		
		// Count valid submissions
		validCount := uint64(0)
		for _, sub := range minerSubs {
			if sub.IsValid {
				validCount++
			}
		}
		stats.ValidSubmissions += validCount
		
		// Update success rate
		if stats.TotalSubmissions > 0 {
			stats.SuccessRate = float64(stats.ValidSubmissions) / float64(stats.TotalSubmissions)
		}
		
		// Check for wins and update rank information
		for _, winner := range epoch.TopNWinners {
			if winner.MinerADI.Equal(stats.MinerADI) {
				stats.TotalWins++
				
				if stats.BestRank == 0 || winner.Rank < stats.BestRank {
					stats.BestRank = winner.Rank
				}
				
				// Update best hash
				if stats.BestHashEver == nil || new(big.Int).SetBytes(winner.ComputedHash).Cmp(new(big.Int).SetBytes(stats.BestHashEver)) < 0 {
					stats.BestHashEver = winner.ComputedHash
				}
				
				// Add reward if available
				if winner.RewardAmount != nil {
					stats.TotalRewardsEarned.Add(stats.TotalRewardsEarned, winner.RewardAmount)
				}
				
				break
			}
		}
		
		// Update win rate
		if stats.TotalEpochsParticipated > 0 {
			stats.WinRate = float64(stats.TotalWins) / float64(stats.TotalEpochsParticipated)
		}
		
		// Calculate average reward per epoch
		if stats.TotalEpochsParticipated > 0 {
			stats.AverageRewardPerEpoch = new(big.Int).Div(stats.TotalRewardsEarned, big.NewInt(int64(stats.TotalEpochsParticipated)))
		}
	}
}

func (ma *MiningAnalytics) updateNetworkStatistics(newMetrics *EpochMetrics) {
	ma.networkStats.TotalEpochs++
	
	// Update total rewards
	if ma.networkStats.TotalRewardsIssued == nil {
		ma.networkStats.TotalRewardsIssued = big.NewInt(0)
	}
	if newMetrics.TotalRewardsIssued != nil {
		ma.networkStats.TotalRewardsIssued.Add(ma.networkStats.TotalRewardsIssued, newMetrics.TotalRewardsIssued)
	}
	
	// Calculate average reward per epoch
	if ma.networkStats.TotalEpochs > 0 {
		ma.networkStats.AverageRewardPerEpoch = new(big.Int).Div(ma.networkStats.TotalRewardsIssued, big.NewInt(int64(ma.networkStats.TotalEpochs)))
	}
	
	// Count unique miners across all time
	ma.networkStats.TotalMinersEver = uint64(len(ma.minerStats))
	
	// Count active miners (participated in last 10 epochs)
	activeThreshold := newMetrics.EpochNumber
	if activeThreshold > 10 {
		activeThreshold = newMetrics.EpochNumber - 10
	}
	
	activeCount := uint64(0)
	for _, stats := range ma.minerStats {
		if stats.LastEpoch >= activeThreshold {
			activeCount++
		}
	}
	ma.networkStats.ActiveMiners = activeCount
	
	// Calculate miner concentration (Herfindahl index)
	ma.calculateMinerConcentration()
}

func (ma *MiningAnalytics) calculateMinerConcentration() {
	if len(ma.minerStats) == 0 {
		ma.networkStats.MinerConcentrationIndex = 0
		return
	}
	
	// Calculate based on win shares
	totalWins := uint64(0)
	for _, stats := range ma.minerStats {
		totalWins += stats.TotalWins
	}
	
	if totalWins == 0 {
		ma.networkStats.MinerConcentrationIndex = 0
		return
	}
	
	herfindahl := 0.0
	topMinerShare := 0.0
	
	for _, stats := range ma.minerStats {
		share := float64(stats.TotalWins) / float64(totalWins)
		herfindahl += share * share
		
		if share > topMinerShare {
			topMinerShare = share
		}
	}
	
	ma.networkStats.MinerConcentrationIndex = herfindahl
	ma.networkStats.TopMinerHashShare = topMinerShare
}

func (ma *MiningAnalytics) cleanupOldData() {
	// Remove epochs beyond retention limit
	if uint64(len(ma.epochMetrics)) > ma.maxEpochsStored {
		// Find oldest epochs to remove
		oldestToKeep := uint64(0)
		for epochNum := range ma.epochMetrics {
			if oldestToKeep == 0 || epochNum > oldestToKeep {
				oldestToKeep = epochNum
			}
		}
		
		if oldestToKeep > ma.maxEpochsStored {
			oldestToKeep = oldestToKeep - ma.maxEpochsStored
			
			for epochNum := range ma.epochMetrics {
				if epochNum < oldestToKeep {
					delete(ma.epochMetrics, epochNum)
				}
			}
		}
	}
	
	// Clean up old miner data based on time
	cutoffTime := time.Now().Add(-ma.retentionPeriod)
	for minerKey, stats := range ma.minerStats {
		// Remove miners who haven't participated recently
		if len(ma.epochMetrics) > 0 {
			lastActiveEpoch, exists := ma.epochMetrics[stats.LastEpoch]
			if exists && lastActiveEpoch.EndTime.Before(cutoffTime) {
				delete(ma.minerStats, minerKey)
			}
		}
	}
}