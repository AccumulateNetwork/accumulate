// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package bullshark implements the Bullshark consensus ordering algorithm.
// Bullshark is a DAG-based BFT consensus protocol that achieves consensus
// by building a DAG of certificates, electing "anchor" leaders at even rounds,
// and committing leaders that have sufficient support.
package bullshark

import (
	"encoding/hex"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// ConsensusOutput represents a certificate that has been ordered by consensus.
// The Certificate field contains the certificate to be committed.
type ConsensusOutput struct {
	// Certificate is the certificate that should be committed.
	Certificate *types.Certificate
}

// Bullshark implements the Bullshark consensus ordering algorithm.
// It processes certificates as they are added to the DAG and determines
// when leaders can be committed along with their sub-DAGs.
type Bullshark struct {
	// committee is the current validator committee.
	committee *types.Committee
	// dag is the certificate DAG.
	dag *dag.DAG

	// mu protects commit tracking state.
	mu sync.RWMutex
	// lastCommitRound is the most recently committed leader round.
	lastCommitRound types.Round
	// lastCommitted tracks the last committed round for each author.
	// Key is the hex-encoded author public key.
	lastCommitted map[string]types.Round
}

// New creates a new Bullshark consensus instance.
func New(committee *types.Committee, d *dag.DAG) *Bullshark {
	return &Bullshark{
		committee:     committee,
		dag:           d,
		lastCommitted: make(map[string]types.Round),
	}
}

// NewWithState creates a new Bullshark instance with existing state.
// This is used for crash recovery or catching up.
func NewWithState(committee *types.Committee, d *dag.DAG, lastCommitRound types.Round, lastCommitted map[string]types.Round) *Bullshark {
	committed := make(map[string]types.Round)
	for k, v := range lastCommitted {
		committed[k] = v
	}
	return &Bullshark{
		committee:       committee,
		dag:             d,
		lastCommitRound: lastCommitRound,
		lastCommitted:   committed,
	}
}

// ProcessCertificate is called when a new certificate is added to the DAG.
// It checks if this certificate enables any leaders to be committed.
// Returns an ordered list of certificates to commit, or nil if no commit is possible.
//
// The algorithm:
// 1. Leaders are elected at even rounds (2, 4, 6, ...)
// 2. We check for commits when we receive certificates from odd rounds
// 3. A leader commits when it has f+1 support from the next round
// 4. Committing a leader also commits all linked previous uncommitted leaders
func (b *Bullshark) ProcessCertificate(cert *types.Certificate) []ConsensusOutput {
	if cert == nil {
		return nil
	}

	round := cert.Round()

	// Leaders are elected at even rounds.
	// We check for commits when we receive certificates from odd rounds.
	// The leader round we're checking is round-1.
	leaderRound := round - 1

	// Only even rounds have leaders.
	if leaderRound%2 != 0 {
		return nil
	}

	// Round 0 has no leader (leaders start at round 2).
	if leaderRound < 2 {
		return nil
	}

	// Check if we've already committed this round.
	b.mu.RLock()
	alreadyCommitted := leaderRound <= b.lastCommitRound
	b.mu.RUnlock()

	if alreadyCommitted {
		return nil
	}

	// Elect leader for this round.
	leader := b.electLeader(leaderRound)
	if leader == nil {
		// No leader certificate exists for this round.
		return nil
	}

	// Check if leader has f+1 support from round+1 certificates.
	if !b.hasSupport(leader, round) {
		return nil
	}

	// Leader has support! Commit the leader chain.
	return b.commitLeaderChain(leader)
}

// LastCommitRound returns the most recently committed leader round.
func (b *Bullshark) LastCommitRound() types.Round {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastCommitRound
}

// GetLastCommitted returns a copy of the lastCommitted map.
// This is useful for state persistence or debugging.
func (b *Bullshark) GetLastCommitted() map[string]types.Round {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make(map[string]types.Round, len(b.lastCommitted))
	for k, v := range b.lastCommitted {
		result[k] = v
	}
	return result
}

// SetLastCommitRound sets the last commit round.
// This is useful for crash recovery.
func (b *Bullshark) SetLastCommitRound(round types.Round) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastCommitRound = round
}

// UpdateCommittee updates the validator committee.
// This should be called when the epoch changes.
func (b *Bullshark) UpdateCommittee(committee *types.Committee) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.committee = committee
}

// MarkCommitted marks a certificate as committed.
// This is used during crash recovery to rebuild state.
func (b *Bullshark) MarkCommitted(cert *types.Certificate) {
	if cert == nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	authorKey := hex.EncodeToString(cert.Author())
	if lastRound, ok := b.lastCommitted[authorKey]; !ok || cert.Round() > lastRound {
		b.lastCommitted[authorKey] = cert.Round()
	}
}
