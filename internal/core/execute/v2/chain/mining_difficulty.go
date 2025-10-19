package chain

import (
	"math"
	"math/big"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// DifficultyAdjustmentStrategy defines different approaches to difficulty adjustment
type DifficultyAdjustmentStrategy int

const (
	DifficultyStrategyFixed DifficultyAdjustmentStrategy = iota      // Fixed difficulty (no adjustment)
	DifficultyStrategyLinear                                         // Linear adjustment based on submission rate
	DifficultyStrategyExponential                                    // Exponential adjustment (Bitcoin-style)
	DifficultyStrategyAdaptive                                       // Adaptive adjustment based on multiple factors
)

// DifficultyMetrics contains metrics used for difficulty calculation
type DifficultyMetrics struct {
	// Submission Rate Analysis
	TargetSubmissionsPerBlock  float64   `json:"targetSubmissionsPerBlock"`
	ActualSubmissionsPerBlock  float64   `json:"actualSubmissionsPerBlock"`
	SubmissionRateRatio        float64   `json:"submissionRateRatio"`       // actual / target
	
	// Time Analysis
	TargetEpochDuration        time.Duration `json:"targetEpochDuration"`
	ActualEpochDuration        time.Duration `json:"actualEpochDuration"`
	TimingRatio                float64       `json:"timingRatio"`           // actual / target
	
	// Participation Analysis
	UniqueMiners               uint64        `json:"uniqueMiners"`
	TotalSubmissions           uint64        `json:"totalSubmissions"`
	ValidSubmissions           uint64        `json:"validSubmissions"`
	AcceptanceRate             float64       `json:"acceptanceRate"`        // valid / total
	
	// Competition Analysis
	TopNSize                   uint64        `json:"topNSize"`
	CompetitionRatio           float64       `json:"competitionRatio"`      // valid / top_n
	
	// Hash Quality Analysis
	AverageHashQuality         *big.Int      `json:"averageHashQuality,omitempty"`
	BestHashQuality            *big.Int      `json:"bestHashQuality,omitempty"`
	HashQualityVariance        *big.Float    `json:"hashQualityVariance,omitempty"`
}

// calculateNewBaseline computes the new baseline difficulty target based on historical performance
func (em *EpochManager) calculateNewBaseline() ([]byte, error) {
	// If no history, use default difficulty
	if len(em.epochHistory) == 0 {
		return em.getDefaultDifficulty(), nil
	}
	
	// Analyze recent epochs for adjustment
	analysisWindow := em.difficultyWindow
	if analysisWindow > uint64(len(em.epochHistory)) {
		analysisWindow = uint64(len(em.epochHistory))
	}
	
	recentEpochs := em.epochHistory[len(em.epochHistory)-int(analysisWindow):]
	
	// Calculate difficulty metrics
	metrics, err := em.calculateDifficultyMetrics(recentEpochs)
	if err != nil {
		return nil, err
	}
	
	// Get current baseline target
	currentTarget := new(big.Int).SetBytes(em.getCurrentBaselineTarget())
	
	// Apply difficulty adjustment strategy
	newTarget, err := em.applyDifficultyAdjustment(currentTarget, metrics, DifficultyStrategyAdaptive)
	if err != nil {
		return nil, err
	}
	
	// Ensure new target is within bounds
	newTarget = em.clampDifficultyTarget(newTarget)
	
	// Convert to 32-byte array
	targetBytes := make([]byte, 32)
	newTargetBytes := newTarget.Bytes()
	
	// Copy to right-aligned 32-byte array (big-endian)
	if len(newTargetBytes) <= 32 {
		copy(targetBytes[32-len(newTargetBytes):], newTargetBytes)
	} else {
		// Target too large, use maximum difficulty
		copy(targetBytes, em.maxDifficultyTarget.Bytes()[:32])
	}
	
	return targetBytes, nil
}

// calculateDifficultyMetrics analyzes recent epochs to generate metrics for adjustment
func (em *EpochManager) calculateDifficultyMetrics(epochs []*MiningEpoch) (*DifficultyMetrics, error) {
	if len(epochs) == 0 {
		return nil, errors.BadRequest.WithFormat("no epochs provided for analysis")
	}
	
	metrics := &DifficultyMetrics{
		TargetSubmissionsPerBlock: em.targetSubmissionRate,
		TargetEpochDuration:       time.Duration(em.epochDurationBlocks) * time.Second,
	}
	
	// Aggregate statistics across epochs
	var totalSubmissions uint64
	var totalValidSubmissions uint64
	var totalMiners uint64
	var totalDuration time.Duration
	var totalBlocks uint64
	
	hashQualities := make([]*big.Int, 0)
	
	for _, epoch := range epochs {
		totalSubmissions += epoch.TotalSubmissions
		totalValidSubmissions += epoch.ValidSubmissions
		totalMiners += epoch.MinerCount
		
		if epoch.FinalizedAt != nil {
			epochDuration := epoch.FinalizedAt.Sub(epoch.CreatedAt)
			totalDuration += epochDuration
		} else {
			totalDuration += time.Since(epoch.CreatedAt)
		}
		
		epochBlocks := epoch.EndBlock - epoch.StartBlock
		totalBlocks += epochBlocks
		
		// Collect hash qualities from winners
		for _, winner := range epoch.TopNWinners {
			if len(winner.ComputedHash) > 0 {
				hashQuality := new(big.Int).SetBytes(winner.ComputedHash)
				hashQualities = append(hashQualities, hashQuality)
			}
		}
	}
	
	epochCount := float64(len(epochs))
	
	// Calculate submission rate metrics
	if totalBlocks > 0 {
		metrics.ActualSubmissionsPerBlock = float64(totalSubmissions) / float64(totalBlocks)
		metrics.SubmissionRateRatio = metrics.ActualSubmissionsPerBlock / metrics.TargetSubmissionsPerBlock
	}
	
	// Calculate timing metrics
	if epochCount > 0 {
		metrics.ActualEpochDuration = time.Duration(float64(totalDuration) / epochCount)
		metrics.TimingRatio = float64(metrics.ActualEpochDuration) / float64(metrics.TargetEpochDuration)
	}
	
	// Calculate participation metrics
	metrics.UniqueMiners = uint64(float64(totalMiners) / epochCount)
	metrics.TotalSubmissions = totalSubmissions
	metrics.ValidSubmissions = totalValidSubmissions
	
	if totalSubmissions > 0 {
		metrics.AcceptanceRate = float64(totalValidSubmissions) / float64(totalSubmissions)
	}
	
	// Calculate competition metrics
	if len(epochs) > 0 {
		// Use the most recent epoch's top-N size
		lastEpoch := epochs[len(epochs)-1]
		metrics.TopNSize = uint64(len(lastEpoch.TopNWinners))
		
		if metrics.TopNSize > 0 {
			avgValidPerEpoch := float64(totalValidSubmissions) / epochCount
			metrics.CompetitionRatio = avgValidPerEpoch / float64(metrics.TopNSize)
		}
	}
	
	// Calculate hash quality metrics
	if len(hashQualities) > 0 {
		metrics.AverageHashQuality = calculateAverageHashQuality(hashQualities)
		metrics.BestHashQuality = findBestHashQuality(hashQualities)
		metrics.HashQualityVariance = calculateHashQualityVariance(hashQualities, metrics.AverageHashQuality)
	}
	
	return metrics, nil
}

// applyDifficultyAdjustment applies the selected difficulty adjustment strategy
func (em *EpochManager) applyDifficultyAdjustment(
	currentTarget *big.Int,
	metrics *DifficultyMetrics,
	strategy DifficultyAdjustmentStrategy,
) (*big.Int, error) {
	
	switch strategy {
	case DifficultyStrategyFixed:
		return new(big.Int).Set(currentTarget), nil
		
	case DifficultyStrategyLinear:
		return em.applyLinearAdjustment(currentTarget, metrics)
		
	case DifficultyStrategyExponential:
		return em.applyExponentialAdjustment(currentTarget, metrics)
		
	case DifficultyStrategyAdaptive:
		return em.applyAdaptiveAdjustment(currentTarget, metrics)
		
	default:
		return nil, errors.BadRequest.WithFormat("unknown difficulty adjustment strategy: %d", strategy)
	}
}

// applyLinearAdjustment applies linear difficulty adjustment based on submission rate
func (em *EpochManager) applyLinearAdjustment(currentTarget *big.Int, metrics *DifficultyMetrics) (*big.Int, error) {
	// Simple linear adjustment based on submission rate ratio
	adjustmentFactor := 1.0 / metrics.SubmissionRateRatio
	
	// Clamp adjustment factor
	if adjustmentFactor > em.maxDifficultyChange {
		adjustmentFactor = em.maxDifficultyChange
	} else if adjustmentFactor < 1.0/em.maxDifficultyChange {
		adjustmentFactor = 1.0 / em.maxDifficultyChange
	}
	
	// Apply adjustment
	newTarget := new(big.Float).SetInt(currentTarget)
	newTarget.Mul(newTarget, big.NewFloat(adjustmentFactor))
	
	result, _ := newTarget.Int(nil)
	return result, nil
}

// applyExponentialAdjustment applies Bitcoin-style exponential difficulty adjustment
func (em *EpochManager) applyExponentialAdjustment(currentTarget *big.Int, metrics *DifficultyMetrics) (*big.Int, error) {
	// Calculate adjustment factor based on timing ratio (similar to Bitcoin)
	timingFactor := metrics.TimingRatio
	
	// Apply exponential scaling for larger deviations
	var adjustmentFactor float64
	if timingFactor > 1.0 {
		// Epochs taking too long, make difficulty easier (increase target)
		adjustmentFactor = math.Pow(timingFactor, 0.5) // Square root for smoother adjustment
	} else {
		// Epochs too fast, make difficulty harder (decrease target)
		adjustmentFactor = math.Pow(timingFactor, 2.0) // Square for faster response
	}
	
	// Clamp adjustment factor
	if adjustmentFactor > em.maxDifficultyChange {
		adjustmentFactor = em.maxDifficultyChange
	} else if adjustmentFactor < 1.0/em.maxDifficultyChange {
		adjustmentFactor = 1.0 / em.maxDifficultyChange
	}
	
	// Apply adjustment
	newTarget := new(big.Float).SetInt(currentTarget)
	newTarget.Mul(newTarget, big.NewFloat(adjustmentFactor))
	
	result, _ := newTarget.Int(nil)
	return result, nil
}

// applyAdaptiveAdjustment applies multi-factor adaptive difficulty adjustment
func (em *EpochManager) applyAdaptiveAdjustment(currentTarget *big.Int, metrics *DifficultyMetrics) (*big.Int, error) {
	// Weight different factors for comprehensive adjustment
	weights := map[string]float64{
		"submission_rate": 0.4,  // 40% weight on submission rate
		"timing":          0.3,  // 30% weight on epoch timing
		"competition":     0.2,  // 20% weight on competition level
		"participation":   0.1,  // 10% weight on miner participation
	}
	
	var totalAdjustment float64
	
	// Factor 1: Submission rate adjustment
	submissionRateAdjustment := 1.0 / metrics.SubmissionRateRatio
	totalAdjustment += weights["submission_rate"] * submissionRateAdjustment
	
	// Factor 2: Timing adjustment
	timingAdjustment := metrics.TimingRatio
	totalAdjustment += weights["timing"] * timingAdjustment
	
	// Factor 3: Competition adjustment
	var competitionAdjustment float64
	targetCompetition := 5.0 // Target 5x competition (5 valid submissions per slot)
	if metrics.CompetitionRatio > 0 {
		competitionAdjustment = targetCompetition / metrics.CompetitionRatio
	} else {
		competitionAdjustment = 1.0
	}
	totalAdjustment += weights["competition"] * competitionAdjustment
	
	// Factor 4: Participation adjustment
	var participationAdjustment float64
	targetMiners := 20.0 // Target 20 unique miners per epoch
	if metrics.UniqueMiners > 0 {
		participationAdjustment = float64(metrics.UniqueMiners) / targetMiners
		if participationAdjustment > 1.0 {
			participationAdjustment = 1.0 + (participationAdjustment-1.0)*0.5 // Dampen positive participation impact
		}
	} else {
		participationAdjustment = 0.5 // Encourage participation if no miners
	}
	totalAdjustment += weights["participation"] * participationAdjustment
	
	// Clamp total adjustment
	if totalAdjustment > em.maxDifficultyChange {
		totalAdjustment = em.maxDifficultyChange
	} else if totalAdjustment < 1.0/em.maxDifficultyChange {
		totalAdjustment = 1.0 / em.maxDifficultyChange
	}
	
	// Apply adjustment
	newTarget := new(big.Float).SetInt(currentTarget)
	newTarget.Mul(newTarget, big.NewFloat(totalAdjustment))
	
	result, _ := newTarget.Int(nil)
	return result, nil
}

// clampDifficultyTarget ensures the difficulty target is within valid bounds
func (em *EpochManager) clampDifficultyTarget(target *big.Int) *big.Int {
	// Ensure target is not too easy (below minimum)
	if target.Cmp(em.maxDifficultyTarget) > 0 {
		return new(big.Int).Set(em.maxDifficultyTarget)
	}
	
	// Ensure target is not too hard (above maximum)
	if target.Cmp(em.minDifficultyTarget) < 0 {
		return new(big.Int).Set(em.minDifficultyTarget)
	}
	
	return target
}

// getDefaultDifficulty returns the default difficulty target for new networks
func (em *EpochManager) getDefaultDifficulty() []byte {
	// Default to relatively easy difficulty for initial epochs
	// This allows early miners to participate and builds network effect
	defaultTarget := make([]byte, 32)
	
	// Set to 0x00000fffff... (about 20-bit difficulty, similar to early Bitcoin)
	defaultTarget[0] = 0x00
	defaultTarget[1] = 0x00
	defaultTarget[2] = 0x0f
	for i := 3; i < 32; i++ {
		defaultTarget[i] = 0xff
	}
	
	return defaultTarget
}

// getCurrentBaselineTarget returns the current epoch's baseline target or default
func (em *EpochManager) getCurrentBaselineTarget() []byte {
	if em.currentEpoch != nil && len(em.currentEpoch.BaselineTarget) == 32 {
		return em.currentEpoch.BaselineTarget
	}
	
	// Use most recent epoch if available
	if len(em.epochHistory) > 0 {
		lastEpoch := em.epochHistory[len(em.epochHistory)-1]
		if len(lastEpoch.BaselineTarget) == 32 {
			return lastEpoch.BaselineTarget
		}
	}
	
	// Fall back to default
	return em.getDefaultDifficulty()
}

// Hash quality analysis helper functions

func calculateAverageHashQuality(hashes []*big.Int) *big.Int {
	if len(hashes) == 0 {
		return big.NewInt(0)
	}
	
	sum := big.NewInt(0)
	for _, hash := range hashes {
		sum.Add(sum, hash)
	}
	
	avg := new(big.Int).Div(sum, big.NewInt(int64(len(hashes))))
	return avg
}

func findBestHashQuality(hashes []*big.Int) *big.Int {
	if len(hashes) == 0 {
		return big.NewInt(0)
	}
	
	best := new(big.Int).Set(hashes[0])
	for _, hash := range hashes[1:] {
		if hash.Cmp(best) < 0 { // Lower hash value is better
			best.Set(hash)
		}
	}
	
	return best
}

func calculateHashQualityVariance(hashes []*big.Int, average *big.Int) *big.Float {
	if len(hashes) <= 1 {
		return big.NewFloat(0)
	}
	
	variance := big.NewFloat(0)
	avgFloat := new(big.Float).SetInt(average)
	n := big.NewFloat(float64(len(hashes)))
	
	for _, hash := range hashes {
		hashFloat := new(big.Float).SetInt(hash)
		diff := new(big.Float).Sub(hashFloat, avgFloat)
		diffSquared := new(big.Float).Mul(diff, diff)
		variance.Add(variance, diffSquared)
	}
	
	variance.Quo(variance, n)
	return variance
}

// GetDifficultyInfo returns current difficulty information
func (em *EpochManager) GetDifficultyInfo() *DifficultyInfo {
	em.mutex.RLock()
	defer em.mutex.RUnlock()
	
	info := &DifficultyInfo{
		CurrentTarget:        em.getCurrentBaselineTarget(),
		MinTarget:           em.minDifficultyTarget.Bytes(),
		MaxTarget:           em.maxDifficultyTarget.Bytes(),
		TargetSubmissionRate: em.targetSubmissionRate,
		DifficultyWindow:     em.difficultyWindow,
		MaxDifficultyChange:  em.maxDifficultyChange,
		LastAdjustment:       em.lastDifficultyAdjustment,
	}
	
	// Calculate current difficulty value (higher number = harder)
	currentTargetInt := new(big.Int).SetBytes(info.CurrentTarget)
	maxTargetInt := new(big.Int).SetBytes(info.MaxTarget)
	
	if currentTargetInt.Cmp(big.NewInt(0)) > 0 {
		difficulty := new(big.Float).Quo(new(big.Float).SetInt(maxTargetInt), new(big.Float).SetInt(currentTargetInt))
		info.CurrentDifficulty = difficulty
	}
	
	return info
}

// DifficultyInfo contains current difficulty information
type DifficultyInfo struct {
	CurrentTarget        []byte      `json:"currentTarget"`
	CurrentDifficulty    *big.Float  `json:"currentDifficulty,omitempty"`
	MinTarget           []byte      `json:"minTarget"`
	MaxTarget           []byte      `json:"maxTarget"`
	TargetSubmissionRate float64     `json:"targetSubmissionRate"`
	DifficultyWindow     uint64      `json:"difficultyWindow"`
	MaxDifficultyChange  float64     `json:"maxDifficultyChange"`
	LastAdjustment       time.Time   `json:"lastAdjustment"`
}