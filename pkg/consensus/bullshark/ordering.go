// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bullshark

import (
	"bytes"
	"encoding/hex"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// commitLeaderChain commits the leader and all linked previous uncommitted leaders.
// It returns an ordered list of certificates to commit, with the oldest first.
func (b *Bullshark) commitLeaderChain(leader *types.Certificate) []ConsensusOutput {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Find all uncommitted leaders linked to this one.
	leaders := b.orderLeaders(leader)
	if len(leaders) == 0 {
		return nil
	}

	var outputs []ConsensusOutput

	// Process each leader, oldest first.
	for _, l := range leaders {
		// Get all certificates in this leader's sub-dag.
		subdag := b.orderDag(l)

		for _, cert := range subdag {
			// Skip if already committed.
			authorKey := hex.EncodeToString(cert.Author())
			if lastRound, ok := b.lastCommitted[authorKey]; ok && cert.Round() <= lastRound {
				continue
			}

			outputs = append(outputs, ConsensusOutput{
				Certificate: cert,
			})

			// Mark as committed.
			b.lastCommitted[authorKey] = cert.Round()
		}

		// Update last commit round after processing each leader.
		b.lastCommitRound = l.Round()
	}

	return outputs
}

// orderLeaders finds all leaders linked to the given leader, back to lastCommitRound.
// Leaders are returned in chronological order (oldest first).
func (b *Bullshark) orderLeaders(leader *types.Certificate) []*types.Certificate {
	var leaders []*types.Certificate
	current := leader

	for current != nil && current.Round() > b.lastCommitRound {
		// Prepend to get oldest first.
		leaders = append([]*types.Certificate{current}, leaders...)

		// Find previous leader (2 rounds back, since leaders are at even rounds).
		prevRound := current.Round() - 2
		if prevRound <= b.lastCommitRound || prevRound < 2 {
			break
		}

		prevLeader := b.electLeader(prevRound)
		if prevLeader == nil {
			// Previous leader didn't produce a certificate.
			break
		}

		// Check if current leader references the previous leader.
		if !b.dag.IsAncestor(prevLeader, current) {
			// No link to previous leader, stop here.
			break
		}

		current = prevLeader
	}

	return leaders
}

// orderDag flattens the sub-dag referenced by a leader.
// Returns all certificates reachable from the leader down to lastCommitRound+1,
// in deterministic order (by round ascending, then by author).
func (b *Bullshark) orderDag(leader *types.Certificate) []*types.Certificate {
	// We want certificates from lastCommitRound+1 to leader's round.
	minRound := b.lastCommitRound + 1

	// Get all ancestors including the leader itself.
	ancestors := b.dag.GetAncestors(leader, minRound)
	if len(ancestors) == 0 {
		return nil
	}

	// Filter out already committed certificates.
	var filtered []*types.Certificate
	for _, cert := range ancestors {
		authorKey := hex.EncodeToString(cert.Author())
		if lastRound, ok := b.lastCommitted[authorKey]; ok && cert.Round() <= lastRound {
			continue
		}
		filtered = append(filtered, cert)
	}

	// Sort deterministically: by round ascending, then by author.
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Round() != filtered[j].Round() {
			return filtered[i].Round() < filtered[j].Round()
		}
		return bytes.Compare(filtered[i].Author(), filtered[j].Author()) < 0
	})

	return filtered
}

// IsLinked checks if there is a path from descendant to ancestor.
// This is used to verify that leaders are properly linked in the DAG.
func (b *Bullshark) IsLinked(ancestor, descendant *types.Certificate) bool {
	return b.dag.IsAncestor(ancestor, descendant)
}
