// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package mining

import (
	"testing"
)

func TestPriorityQueue_AddSubmission(t *testing.T) {
	// Create a priority queue with capacity 3
	pq := NewPriorityQueue(3)
	
	// Create some test submissions
	submissions := []*MiningSubmission{
		{Difficulty: 100, Signer: "signer1"},
		{Difficulty: 200, Signer: "signer2"},
		{Difficulty: 150, Signer: "signer3"},
		{Difficulty: 300, Signer: "signer4"},
		{Difficulty: 50, Signer: "signer5"},
	}
	
	// Add the submissions to the queue
	for _, s := range submissions {
		pq.AddSubmission(s)
	}
	
	// Check that the queue has the correct number of submissions
	if pq.Len() != 3 {
		t.Errorf("Expected queue length to be 3, got %d", pq.Len())
	}
	
	// Check that the queue has the highest difficulty submissions
	queuedSubmissions := pq.GetSubmissions()
	
	// The queue should have the submissions with difficulties 300, 200, and 150
	expectedDifficulties := map[uint64]bool{
		300: false,
		200: false,
		150: false,
	}
	
	for _, s := range queuedSubmissions {
		if _, ok := expectedDifficulties[s.Difficulty]; !ok {
			t.Errorf("Unexpected submission with difficulty %d in queue", s.Difficulty)
		}
		expectedDifficulties[s.Difficulty] = true
	}
	
	// Check that all expected difficulties were found
	for d, found := range expectedDifficulties {
		if !found {
			t.Errorf("Expected submission with difficulty %d not found in queue", d)
		}
	}
}

func TestPriorityQueue_Clear(t *testing.T) {
	// Create a priority queue with capacity 3
	pq := NewPriorityQueue(3)
	
	// Add some submissions
	pq.AddSubmission(&MiningSubmission{Difficulty: 100})
	pq.AddSubmission(&MiningSubmission{Difficulty: 200})
	
	// Clear the queue
	pq.Clear()
	
	// Check that the queue is empty
	if pq.Len() != 0 {
		t.Errorf("Expected queue to be empty after clear, got length %d", pq.Len())
	}
}

func TestPriorityQueue_Pop(t *testing.T) {
	// Create a priority queue with capacity 3
	pq := NewPriorityQueue(3)
	
	// Add some submissions
	pq.Push(&MiningSubmission{Difficulty: 100})
	pq.Push(&MiningSubmission{Difficulty: 200})
	pq.Push(&MiningSubmission{Difficulty: 150})
	
	// Pop a submission
	submission := pq.Pop().(*MiningSubmission)
	
	// Check that the highest difficulty submission was popped
	if submission.Difficulty != 200 {
		t.Errorf("Expected popped submission to have difficulty 200, got %d", submission.Difficulty)
	}
	
	// Check that the queue length decreased
	if pq.Len() != 2 {
		t.Errorf("Expected queue length to be 2 after pop, got %d", pq.Len())
	}
}

func TestPriorityQueue_GetSubmissions(t *testing.T) {
	// Create a priority queue with capacity 3
	pq := NewPriorityQueue(3)
	
	// Add some submissions
	pq.AddSubmission(&MiningSubmission{Difficulty: 100, Signer: "signer1"})
	pq.AddSubmission(&MiningSubmission{Difficulty: 200, Signer: "signer2"})
	
	// Get the submissions
	submissions := pq.GetSubmissions()
	
	// Check that the correct number of submissions was returned
	if len(submissions) != 2 {
		t.Errorf("Expected 2 submissions, got %d", len(submissions))
	}
	
	// Check that the submissions are in the correct order (highest difficulty first)
	if submissions[0].Difficulty != 200 || submissions[1].Difficulty != 100 {
		t.Errorf("Submissions not in correct order: %v", submissions)
	}
	
	// Modify the returned submissions and check that the original queue is not affected
	submissions[0].Difficulty = 300
	
	queuedSubmissions := pq.GetSubmissions()
	if queuedSubmissions[0].Difficulty != 200 {
		t.Errorf("Expected original queue to be unchanged, but got difficulty %d", queuedSubmissions[0].Difficulty)
	}
}
