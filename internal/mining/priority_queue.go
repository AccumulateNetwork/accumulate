// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package mining

import (
	"sync"
)

// MiningSubmission represents a mining solution submission
type MiningSubmission struct {
	// Nonce is the nonce used to generate the solution
	Nonce []byte
	// ComputedHash is the hash computed from the nonce and block hash
	ComputedHash [32]byte
	// BlockHash is the hash of the block being mined
	BlockHash [32]byte
	// Signer is the URL of the signer
	Signer string
	// SignerVersion is the version of the signer
	SignerVersion uint64
	// Timestamp is the time the solution was submitted
	Timestamp uint64
	// Difficulty is the difficulty of the solution
	Difficulty uint64
}

// PriorityQueue implements a priority queue for mining submissions
// The priority queue is ordered by difficulty, with higher difficulty having higher priority
type PriorityQueue struct {
	mu          sync.RWMutex
	submissions []*MiningSubmission
	capacity    int
}

// NewPriorityQueue creates a new priority queue with the given capacity
func NewPriorityQueue(capacity int) *PriorityQueue {
	return &PriorityQueue{
		submissions: make([]*MiningSubmission, 0, capacity),
		capacity:    capacity,
	}
}

// Len returns the length of the priority queue
func (pq *PriorityQueue) Len() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return len(pq.submissions)
}

// Less returns whether the item at index i has lower priority than the item at index j
func (pq *PriorityQueue) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, difficulty so we use greater than here
	return pq.submissions[i].Difficulty > pq.submissions[j].Difficulty
}

// Swap swaps the items at indices i and j
func (pq *PriorityQueue) Swap(i, j int) {
	pq.submissions[i], pq.submissions[j] = pq.submissions[j], pq.submissions[i]
}

// Push adds an item to the priority queue
func (pq *PriorityQueue) Push(x interface{}) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	submission := x.(*MiningSubmission)
	
	// Add the submission to the queue
	pq.submissions = append(pq.submissions, submission)
	
	// Fix the heap property
	i := len(pq.submissions) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if pq.Less(i, parent) {
			pq.Swap(i, parent)
			i = parent
		} else {
			break
		}
	}
	
	// If we're over capacity, remove the lowest difficulty submission
	if len(pq.submissions) > pq.capacity {
		// Find the index of the submission with the lowest difficulty
		lowestIdx := 0
		for i := 1; i < len(pq.submissions); i++ {
			if pq.submissions[i].Difficulty < pq.submissions[lowestIdx].Difficulty {
				lowestIdx = i
			}
		}
		
		// Remove the submission with the lowest difficulty
		pq.submissions = append(pq.submissions[:lowestIdx], pq.submissions[lowestIdx+1:]...)
	}
}

// Pop removes and returns the highest priority item from the priority queue
func (pq *PriorityQueue) Pop() interface{} {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	if len(pq.submissions) == 0 {
		return nil
	}
	
	// Get the highest priority item (which should be at index 0)
	submission := pq.submissions[0]
	
	// Remove the item by replacing it with the last item and then removing the last item
	last := len(pq.submissions) - 1
	pq.submissions[0] = pq.submissions[last]
	pq.submissions = pq.submissions[:last]
	
	// Fix the heap property if there are still items in the queue
	if len(pq.submissions) > 0 {
		i := 0
		for {
			largest := i
			left := 2*i + 1
			right := 2*i + 2
			
			if left < len(pq.submissions) && pq.Less(left, largest) {
				largest = left
			}
			
			if right < len(pq.submissions) && pq.Less(right, largest) {
				largest = right
			}
			
			if largest == i {
				break
			}
			
			pq.Swap(i, largest)
			i = largest
		}
	}
	
	return submission
}

// GetSubmissions returns a deep copy of all submissions in the priority queue
func (pq *PriorityQueue) GetSubmissions() []*MiningSubmission {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	
	result := make([]*MiningSubmission, len(pq.submissions))
	
	// Create a deep copy of each submission
	for i, submission := range pq.submissions {
		// Create a new submission
		newSubmission := &MiningSubmission{
			Difficulty:    submission.Difficulty,
			Signer:        submission.Signer,
			SignerVersion: submission.SignerVersion,
			Timestamp:     submission.Timestamp,
		}
		
		// Copy the nonce
		if submission.Nonce != nil {
			newSubmission.Nonce = make([]byte, len(submission.Nonce))
			copy(newSubmission.Nonce, submission.Nonce)
		}
		
		// Copy the computed hash
		newSubmission.ComputedHash = submission.ComputedHash
		
		// Copy the block hash
		newSubmission.BlockHash = submission.BlockHash
		
		result[i] = newSubmission
	}
	
	return result
}

// Clear removes all submissions from the priority queue
func (pq *PriorityQueue) Clear() {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	pq.submissions = make([]*MiningSubmission, 0, pq.capacity)
}

// AddSubmission adds a new mining submission to the priority queue
// Returns true if the submission was added, false if it was rejected
func (pq *PriorityQueue) AddSubmission(submission *MiningSubmission) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	
	// Add the submission to the queue
	pq.submissions = append(pq.submissions, submission)
	
	// Fix the heap property
	i := len(pq.submissions) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if pq.Less(i, parent) {
			pq.Swap(i, parent)
			i = parent
		} else {
			break
		}
	}
	
	// If we're over capacity, remove the lowest difficulty submission
	if len(pq.submissions) > pq.capacity {
		// Find the index of the submission with the lowest difficulty
		lowestIdx := 0
		for i := 1; i < len(pq.submissions); i++ {
			if pq.submissions[i].Difficulty < pq.submissions[lowestIdx].Difficulty {
				lowestIdx = i
			}
		}
		
		// Remove the submission with the lowest difficulty
		pq.submissions = append(pq.submissions[:lowestIdx], pq.submissions[lowestIdx+1:]...)
	}
	
	return true
}
