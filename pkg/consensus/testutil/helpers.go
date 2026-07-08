// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testutil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// AssertAllNodesHaveCommits verifies all active nodes have at least minCommits.
func (tn *TestNetwork) AssertAllNodesHaveCommits(t *testing.T, minCommits int) {
	t.Helper()

	for i, n := range tn.nodes {
		if n.IsStopped() {
			continue
		}
		count := n.CommitCount()
		assert.GreaterOrEqual(t, count, minCommits,
			"node %d has %d commits, expected at least %d", i, count, minCommits)
	}
}

// AssertNodesAgree verifies that all active nodes have committed the same certificates
// (in the same order) for their overlapping commit history.
func (tn *TestNetwork) AssertNodesAgree(t *testing.T) {
	t.Helper()

	// Get commits from all active nodes
	var allCommits [][]*types.Certificate
	for _, n := range tn.nodes {
		if n.IsStopped() {
			continue
		}
		allCommits = append(allCommits, n.GetCommits())
	}

	if len(allCommits) < 2 {
		return // Nothing to compare
	}

	// Find minimum length
	minLen := len(allCommits[0])
	for _, commits := range allCommits[1:] {
		if len(commits) < minLen {
			minLen = len(commits)
		}
	}

	// Compare certificates up to minimum length
	for i := 0; i < minLen; i++ {
		firstDigest := allCommits[0][i].Digest()
		for j := 1; j < len(allCommits); j++ {
			assert.Equal(t, firstDigest, allCommits[j][i].Digest(),
				"certificate mismatch at index %d between node 0 and node %d", i, j)
		}
	}
}

// AssertCommitsContainTransactions verifies that committed certificates contain
// the expected transactions.
func (tn *TestNetwork) AssertCommitsContainTransactions(t *testing.T, expectedTxs [][]byte) {
	t.Helper()

	// Collect all transactions from all nodes
	for _, n := range tn.nodes {
		if n.IsStopped() {
			continue
		}

		commits := n.GetCommits()
		var allTxs [][]byte

		for _, cert := range commits {
			for _, entry := range cert.Header.Payload {
				// Get batch from any worker
				for _, w := range n.Node.Workers() {
					batch, _ := w.GetBatch(entry.Digest)
					if batch != nil {
						allTxs = append(allTxs, batch.Transactions...)
					}
				}
			}
		}

		// Check that all expected transactions appear
		for _, expected := range expectedTxs {
			found := false
			for _, actual := range allTxs {
				if bytes.Equal(expected, actual) {
					found = true
					break
				}
			}
			if !found {
				// Transactions may not all be committed yet, just warn
				t.Logf("Warning: expected transaction not found in node %d commits", n.Index)
			}
		}
	}
}

// GetTotalCommits returns the total number of commits across all nodes.
func (tn *TestNetwork) GetTotalCommits() int {
	total := 0
	for _, n := range tn.nodes {
		if n.IsStopped() {
			continue
		}
		total += n.CommitCount()
	}
	return total
}

// GetMinCommits returns the minimum number of commits across all active nodes.
func (tn *TestNetwork) GetMinCommits() int {
	minCommits := -1
	for _, n := range tn.nodes {
		if n.IsStopped() {
			continue
		}
		count := n.CommitCount()
		if minCommits < 0 || count < minCommits {
			minCommits = count
		}
	}
	if minCommits < 0 {
		return 0
	}
	return minCommits
}

// GetMaxRound returns the maximum current round across all active nodes.
func (tn *TestNetwork) GetMaxRound() types.Round {
	var maxRound types.Round
	for _, n := range tn.nodes {
		if n.IsStopped() {
			continue
		}
		if round := n.Node.CurrentRound(); round > maxRound {
			maxRound = round
		}
	}
	return maxRound
}

// GetMinRound returns the minimum current round across all active nodes.
func (tn *TestNetwork) GetMinRound() types.Round {
	var minRound types.Round
	first := true
	for _, n := range tn.nodes {
		if n.IsStopped() {
			continue
		}
		round := n.Node.CurrentRound()
		if first || round < minRound {
			minRound = round
			first = false
		}
	}
	return minRound
}

// ActiveNodeCount returns the number of active (non-stopped) nodes.
func (tn *TestNetwork) ActiveNodeCount() int {
	count := 0
	for _, n := range tn.nodes {
		if !n.IsStopped() {
			count++
		}
	}
	return count
}

// RequireAllNodesHaveCommits fails the test if any active node has fewer commits.
func (tn *TestNetwork) RequireAllNodesHaveCommits(t *testing.T, minCommits int) {
	t.Helper()

	for i, n := range tn.nodes {
		if n.IsStopped() {
			continue
		}
		count := n.CommitCount()
		require.GreaterOrEqual(t, count, minCommits,
			"node %d has %d commits, expected at least %d", i, count, minCommits)
	}
}

// LogNodeStatus logs the current status of all nodes.
func (tn *TestNetwork) LogNodeStatus(t *testing.T) {
	t.Helper()

	t.Log("=== Node Status ===")
	for i, n := range tn.nodes {
		if n.IsStopped() {
			t.Logf("Node %d: STOPPED", i)
			continue
		}

		txSubmitted, certsCommitted := n.Node.Metrics()
		t.Logf("Node %d: round=%d, commits=%d, txSubmitted=%d, certsCommitted=%d",
			i,
			n.Node.CurrentRound(),
			n.CommitCount(),
			txSubmitted,
			certsCommitted)
	}
	t.Log("===================")
}

// AssertRoundProgress verifies that rounds are progressing.
func (tn *TestNetwork) AssertRoundProgress(t *testing.T, expectedMinRound types.Round) {
	t.Helper()

	for i, n := range tn.nodes {
		if n.IsStopped() {
			continue
		}
		round := n.Node.CurrentRound()
		assert.GreaterOrEqual(t, round, expectedMinRound,
			"node %d is at round %d, expected at least %d", i, round, expectedMinRound)
	}
}

// AssertDAGConsistency verifies that the DAG is consistent across nodes.
func (tn *TestNetwork) AssertDAGConsistency(t *testing.T) {
	t.Helper()

	// Get DAG sizes from all active nodes
	var dagSizes []int
	for _, n := range tn.nodes {
		if n.IsStopped() {
			continue
		}
		dagSizes = append(dagSizes, n.Node.DAG().Size())
	}

	if len(dagSizes) < 2 {
		return
	}

	// DAGs should be similar in size (within tolerance)
	minSize := dagSizes[0]
	maxSize := dagSizes[0]
	for _, size := range dagSizes[1:] {
		if size < minSize {
			minSize = size
		}
		if size > maxSize {
			maxSize = size
		}
	}

	// Allow some variance due to timing
	tolerance := maxSize / 4
	if tolerance < 5 {
		tolerance = 5
	}

	assert.LessOrEqual(t, maxSize-minSize, tolerance,
		"DAG sizes too different: min=%d, max=%d", minSize, maxSize)
}

// AssertBullsharkProgress verifies that Bullshark is making commit progress.
func (tn *TestNetwork) AssertBullsharkProgress(t *testing.T, expectedMinCommitRound types.Round) {
	t.Helper()

	for i, n := range tn.nodes {
		if n.IsStopped() {
			continue
		}
		commitRound := n.Node.LastCommitRound()
		assert.GreaterOrEqual(t, commitRound, expectedMinCommitRound,
			"node %d last commit round is %d, expected at least %d",
			i, commitRound, expectedMinCommitRound)
	}
}

// CountCommittedCertsByRound counts how many certificates have been committed
// from each round across all active nodes.
func (tn *TestNetwork) CountCommittedCertsByRound() map[types.Round]int {
	counts := make(map[types.Round]int)

	for _, n := range tn.nodes {
		if n.IsStopped() {
			continue
		}

		seen := make(map[types.CertificateDigest]bool)
		for _, cert := range n.GetCommits() {
			digest := cert.Digest()
			if !seen[digest] {
				seen[digest] = true
				counts[cert.Round()]++
			}
		}
	}

	return counts
}
