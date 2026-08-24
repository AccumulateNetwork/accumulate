// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bullshark

import (
	"bytes"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// commitLeaderChain commits the leader and all linked previous uncommitted leaders.
// It returns an ordered list of certificates to commit, with the oldest first.
func (b *Bullshark) commitLeaderChain(leader *types.Certificate) []ConsensusOutput {
	b.mu.Lock()

	// Find all uncommitted leaders linked to this one.
	leaders := b.orderLeaders(leader)
	if len(leaders) == 0 {
		b.mu.Unlock()
		return nil
	}

	var outputs []ConsensusOutput
	var committedCerts []*types.Certificate

	// Process each leader, oldest first.
	for _, l := range leaders {
		// Get all certificates in this leader's sub-dag.
		subdag := b.orderDag(l)

		for _, cert := range subdag {
			// Skip if already committed. This must be a DIGEST check, not the
			// old per-author round watermark: a certificate reaching the
			// committed history later than a higher-round certificate from
			// the same author (the normal case for a straggler rescued by a
			// weak link) was skipped by the watermark as "already committed"
			// when it never was — permanently, silently (#4111).
			digest := cert.Digest()
			if _, done := b.committed[digest]; done {
				continue
			}

			outputs = append(outputs, ConsensusOutput{
				Certificate: cert,
				Leader:      l.Round(),
			})
			committedCerts = append(committedCerts, cert)

			// Mark as committed. lastCommitted keeps the per-author maximum
			// for state reporting; the digest set is the actual dedup.
			b.committed[digest] = cert.Round()
			key := toAuthorKey(cert.Author())
			if lastRound, ok := b.lastCommitted[key]; !ok || cert.Round() > lastRound {
				b.lastCommitted[key] = cert.Round()
			}
		}

		// Update last commit round after processing each leader.
		b.lastCommitRound = l.Round()
	}

	// Prune the digest dedup as the frontier advances. Anything older than
	// the rescue window can never appear in another ancestor walk, so its
	// entry is dead weight.
	for d, r := range b.committed {
		if r+rescueWindow+32 < b.lastCommitRound {
			delete(b.committed, d)
		}
	}

	// Capture callback before unlocking.
	onCommit := b.onCommit
	b.mu.Unlock()

	// Call the commit callback outside the lock to avoid potential deadlocks.
	if onCommit != nil && len(committedCerts) > 0 {
		onCommit(committedCerts)
	}

	return outputs
}

// orderLeaders finds all leaders linked to the given leader, back to lastCommitRound.
// Leaders are returned in chronological order (oldest first).
func (b *Bullshark) orderLeaders(leader *types.Certificate) []*types.Certificate {
	var leaders []*types.Certificate
	current := leader

	// Add the initial leader first.
	if current != nil && current.Round() > b.lastCommitRound {
		leaders = append(leaders, current)
	}

	// Traverse backwards through leader rounds (even rounds, 2 apart).
	prevRound := leader.Round() - 2
	for prevRound > b.lastCommitRound && prevRound >= 2 {
		prevLeader := b.electLeader(prevRound)
		if prevLeader == nil {
			// Leader missing, skip to earlier round.
			prevRound -= 2
			continue
		}

		// Check if current leader references the previous leader.
		if !b.dag.IsAncestor(prevLeader, current) {
			// Not linked, skip to earlier round.
			prevRound -= 2
			continue
		}

		// Found a linked leader, prepend it and continue from there.
		leaders = append([]*types.Certificate{prevLeader}, leaders...)
		current = prevLeader
		prevRound -= 2
	}

	return leaders
}

// rescueWindow is how many rounds below lastCommitRound the ancestor walk
// still looks for uncommitted certificates. A straggler rescued by a weak
// link has a round BELOW the commit frontier by the time it becomes
// reachable; the old lastCommitRound+1 floor excluded exactly the
// certificates the weak links exist to save. The committed-digest set keeps
// the wider walk from re-emitting anything.
const rescueWindow = 32

// orderDag flattens the sub-dag referenced by a leader.
// Returns all reachable, not-yet-committed certificates in deterministic
// order (by round ascending, then by author).
func (b *Bullshark) orderDag(leader *types.Certificate) []*types.Certificate {
	minRound := types.Round(1)
	if b.lastCommitRound > rescueWindow {
		minRound = b.lastCommitRound - rescueWindow
	}

	// Get all ancestors including the leader itself.
	ancestors := b.dag.GetAncestors(leader, minRound)
	if len(ancestors) == 0 {
		return nil
	}

	// Filter out already committed certificates (by digest — see
	// commitLeaderChain for why the author-round watermark was wrong).
	var filtered []*types.Certificate
	for _, cert := range ancestors {
		if _, done := b.committed[cert.Digest()]; done {
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
