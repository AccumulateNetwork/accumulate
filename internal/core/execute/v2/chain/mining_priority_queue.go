package chain

import (
	"bytes"
	"math/big"
	"sort"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// MiningSubmission represents a validated mining submission with metadata
type MiningSubmission struct {
	// Core Mining Data
	MinerADI           *url.URL  `json:"minerADI,omitempty"`
	SubmissionHash     []byte    `json:"submissionHash,omitempty"`
	BoundNonce         []byte    `json:"boundNonce,omitempty"`
	TransactionData    []byte    `json:"transactionData,omitempty"`
	BlockHash          []byte    `json:"blockHash,omitempty"`
	Timestamp          uint64    `json:"timestamp,omitempty"`
	EpochNumber        uint64    `json:"epochNumber,omitempty"`
	
	// Validation Results
	IsValid            bool      `json:"isValid,omitempty"`
	ValidationError    string    `json:"validationError,omitempty"`
	ComputedHash       []byte    `json:"computedHash,omitempty"`
	
	// Ranking and Rewards
	Rank               uint64    `json:"rank,omitempty"`
	RewardAmount       *big.Int  `json:"rewardAmount,omitempty"`
	RewardPaid         bool      `json:"rewardPaid,omitempty"`
	
	// Consensus Tracking
	TransactionBodyHash []byte   `json:"transactionBodyHash,omitempty"`
	ConsensusVotes      uint64   `json:"consensusVotes,omitempty"`
}

// Copy creates a deep copy of the MiningSubmission
func (s *MiningSubmission) Copy() *MiningSubmission {
	if s == nil {
		return nil
	}
	
	copy := &MiningSubmission{
		MinerADI:           s.MinerADI,
		SubmissionHash:     bytes.Clone(s.SubmissionHash),
		BoundNonce:         bytes.Clone(s.BoundNonce),
		TransactionData:    bytes.Clone(s.TransactionData),
		BlockHash:          bytes.Clone(s.BlockHash),
		Timestamp:          s.Timestamp,
		EpochNumber:        s.EpochNumber,
		IsValid:            s.IsValid,
		ValidationError:    s.ValidationError,
		ComputedHash:       bytes.Clone(s.ComputedHash),
		Rank:               s.Rank,
		RewardPaid:         s.RewardPaid,
		TransactionBodyHash: bytes.Clone(s.TransactionBodyHash),
		ConsensusVotes:     s.ConsensusVotes,
	}
	
	if s.RewardAmount != nil {
		copy.RewardAmount = new(big.Int).Set(s.RewardAmount)
	}
	
	return copy
}

// HashValue returns the computed hash as a big.Int for comparison
func (s *MiningSubmission) HashValue() *big.Int {
	if len(s.ComputedHash) == 0 {
		return big.NewInt(0)
	}
	return new(big.Int).SetBytes(s.ComputedHash)
}

// MiningPriorityQueue manages a priority queue of mining submissions
// Maintains the top-N submissions with the best (lowest) hashes
type MiningPriorityQueue struct {
	mutex           sync.RWMutex
	submissions     []*MiningSubmission
	maxSize         uint64
	worstHashIndex  int  // Index of submission with worst (largest) hash
	totalSubmitted  uint64
	totalValid      uint64
}

// NewMiningPriorityQueue creates a new priority queue with the specified maximum size
func NewMiningPriorityQueue(maxSize uint64) *MiningPriorityQueue {
	return &MiningPriorityQueue{
		submissions:    make([]*MiningSubmission, 0, maxSize),
		maxSize:        maxSize,
		worstHashIndex: -1,
	}
}

// InsertOrReplace attempts to insert a submission into the priority queue
// Returns true if the submission was accepted (better than worst or queue not full)
func (pq *MiningPriorityQueue) InsertOrReplace(submission *MiningSubmission) bool {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()
	
	pq.totalSubmitted++
	
	if !submission.IsValid {
		return false
	}
	
	pq.totalValid++
	
	// If queue is not full, add the submission
	if uint64(len(pq.submissions)) < pq.maxSize {
		pq.submissions = append(pq.submissions, submission.Copy())
		pq.updateWorstHashIndex()
		return true
	}
	
	// Queue is full, check if this submission is better than the worst
	if pq.worstHashIndex == -1 {
		pq.updateWorstHashIndex()
	}
	
	worstSubmission := pq.submissions[pq.worstHashIndex]
	submissionHashValue := submission.HashValue()
	worstHashValue := worstSubmission.HashValue()
	
	// If new submission has a better (smaller) hash, replace the worst
	if submissionHashValue.Cmp(worstHashValue) < 0 {
		pq.submissions[pq.worstHashIndex] = submission.Copy()
		pq.updateWorstHashIndex()
		return true
	}
	
	return false
}

// GetTopN returns the top N submissions sorted by hash quality (best first)
func (pq *MiningPriorityQueue) GetTopN() []*MiningSubmission {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	if len(pq.submissions) == 0 {
		return nil
	}
	
	// Create a copy of submissions for sorting
	result := make([]*MiningSubmission, len(pq.submissions))
	for i, sub := range pq.submissions {
		result[i] = sub.Copy()
	}
	
	// Sort by hash value (ascending - best hashes first)
	sort.Slice(result, func(i, j int) bool {
		hashI := result[i].HashValue()
		hashJ := result[j].HashValue()
		return hashI.Cmp(hashJ) < 0
	})
	
	// Update ranks
	for i, sub := range result {
		sub.Rank = uint64(i + 1)
	}
	
	return result
}

// GetWorstHash returns the hash value of the worst submission in the queue
func (pq *MiningPriorityQueue) GetWorstHash() []byte {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	if pq.worstHashIndex == -1 || pq.worstHashIndex >= len(pq.submissions) {
		return nil
	}
	
	return bytes.Clone(pq.submissions[pq.worstHashIndex].ComputedHash)
}

// IsFull returns true if the priority queue is at maximum capacity
func (pq *MiningPriorityQueue) IsFull() bool {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	return uint64(len(pq.submissions)) >= pq.maxSize
}

// Size returns the current number of submissions in the queue
func (pq *MiningPriorityQueue) Size() uint64 {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	return uint64(len(pq.submissions))
}

// GetStatistics returns statistics about the priority queue
func (pq *MiningPriorityQueue) GetStatistics() *PriorityQueueStats {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	stats := &PriorityQueueStats{
		TotalSubmitted: pq.totalSubmitted,
		TotalValid:     pq.totalValid,
		CurrentSize:    uint64(len(pq.submissions)),
		MaxSize:        pq.maxSize,
		IsFull:         uint64(len(pq.submissions)) >= pq.maxSize,
	}
	
	if len(pq.submissions) > 0 && pq.worstHashIndex != -1 {
		stats.WorstHash = bytes.Clone(pq.submissions[pq.worstHashIndex].ComputedHash)
		stats.BestHash = pq.findBestHash()
	}
	
	return stats
}

// PriorityQueueStats contains statistics about the priority queue
type PriorityQueueStats struct {
	TotalSubmitted uint64  `json:"totalSubmitted"`
	TotalValid     uint64  `json:"totalValid"`
	CurrentSize    uint64  `json:"currentSize"`
	MaxSize        uint64  `json:"maxSize"`
	IsFull         bool    `json:"isFull"`
	WorstHash      []byte  `json:"worstHash,omitempty"`
	BestHash       []byte  `json:"bestHash,omitempty"`
}

// Clear removes all submissions from the priority queue
func (pq *MiningPriorityQueue) Clear() {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()
	
	pq.submissions = pq.submissions[:0]
	pq.worstHashIndex = -1
	pq.totalSubmitted = 0
	pq.totalValid = 0
}

// updateWorstHashIndex finds and updates the index of the submission with the worst (largest) hash
// Must be called with mutex locked
func (pq *MiningPriorityQueue) updateWorstHashIndex() {
	if len(pq.submissions) == 0 {
		pq.worstHashIndex = -1
		return
	}
	
	worstIndex := 0
	worstHashValue := pq.submissions[0].HashValue()
	
	for i := 1; i < len(pq.submissions); i++ {
		hashValue := pq.submissions[i].HashValue()
		if hashValue.Cmp(worstHashValue) > 0 {
			worstIndex = i
			worstHashValue = hashValue
		}
	}
	
	pq.worstHashIndex = worstIndex
}

// findBestHash finds the best (smallest) hash in the queue
// Must be called with mutex locked
func (pq *MiningPriorityQueue) findBestHash() []byte {
	if len(pq.submissions) == 0 {
		return nil
	}
	
	bestIndex := 0
	bestHashValue := pq.submissions[0].HashValue()
	
	for i := 1; i < len(pq.submissions); i++ {
		hashValue := pq.submissions[i].HashValue()
		if hashValue.Cmp(bestHashValue) < 0 {
			bestIndex = i
			bestHashValue = hashValue
		}
	}
	
	return bytes.Clone(pq.submissions[bestIndex].ComputedHash)
}

// GetSubmissionByMiner returns the submission from a specific miner, if any
func (pq *MiningPriorityQueue) GetSubmissionByMiner(minerADI *url.URL) *MiningSubmission {
	pq.mutex.RLock()
	defer pq.mutex.RUnlock()
	
	for _, sub := range pq.submissions {
		if sub.MinerADI.Equal(minerADI) {
			return sub.Copy()
		}
	}
	
	return nil
}

// RemoveSubmissionByMiner removes a submission from a specific miner
func (pq *MiningPriorityQueue) RemoveSubmissionByMiner(minerADI *url.URL) bool {
	pq.mutex.Lock()
	defer pq.mutex.Unlock()
	
	for i, sub := range pq.submissions {
		if sub.MinerADI.Equal(minerADI) {
			// Remove the submission
			pq.submissions = append(pq.submissions[:i], pq.submissions[i+1:]...)
			pq.updateWorstHashIndex()
			return true
		}
	}
	
	return false
}