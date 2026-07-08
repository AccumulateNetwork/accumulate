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
	"crypto/ed25519"
	"encoding/hex"
	"log/slog"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// authorKey is a fixed-size array for author public key map keys.
// Using [32]byte instead of hex string saves memory and avoids encoding overhead.
type authorKey [32]byte

// toAuthorKey converts an ed25519 public key to an authorKey.
func toAuthorKey(author ed25519.PublicKey) authorKey {
	var key authorKey
	copy(key[:], author)
	return key
}

// authorKeyFromHex converts a hex-encoded public key string to an authorKey.
func authorKeyFromHex(hexStr string) (authorKey, error) {
	var key authorKey
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return key, err
	}
	copy(key[:], b)
	return key, nil
}

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
	// Uses [32]byte key instead of hex string for memory efficiency.
	lastCommitted map[authorKey]types.Round

	// onCommit is called when certificates are committed.
	// This callback is used to notify external components (e.g., workers)
	// that batches associated with these certificates can be pruned.
	onCommit func(certificates []*types.Certificate)
}

// New creates a new Bullshark consensus instance.
func New(committee *types.Committee, d *dag.DAG) *Bullshark {
	return &Bullshark{
		committee:     committee,
		dag:           d,
		lastCommitted: make(map[authorKey]types.Round),
	}
}

// NewWithState creates a new Bullshark instance with existing state.
// This is used for crash recovery or catching up.
// The lastCommitted map uses hex-encoded author keys for JSON compatibility.
func NewWithState(committee *types.Committee, d *dag.DAG, lastCommitRound types.Round, lastCommitted map[string]types.Round) *Bullshark {
	committed := make(map[authorKey]types.Round)
	for k, v := range lastCommitted {
		if key, err := authorKeyFromHex(k); err == nil {
			committed[key] = v
		}
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
		slog.Info("Bullshark: no leader certificate",
			"leaderRound", leaderRound,
			"dagCertsInRound", len(b.dag.GetRound(leaderRound)))
		return nil
	}

	// Check if leader has f+1 support from round+1 certificates.
	if !b.hasSupport(leader, round) {
		slog.Info("Bullshark: leader lacks support",
			"leaderRound", leaderRound,
			"votingRound", round,
			"votingCerts", len(b.dag.GetRound(round)))
		return nil
	}

	// Leader has support! Commit the leader chain.
	slog.Info("Bullshark: committing leader chain", "leaderRound", leaderRound)
	return b.commitLeaderChain(leader)
}

// LastCommitRound returns the most recently committed leader round.
func (b *Bullshark) LastCommitRound() types.Round {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lastCommitRound
}

// GetLastCommitted returns a copy of the lastCommitted map.
// The keys are hex-encoded author public keys for JSON compatibility.
func (b *Bullshark) GetLastCommitted() map[string]types.Round {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make(map[string]types.Round, len(b.lastCommitted))
	for k, v := range b.lastCommitted {
		k := k // Create local copy to avoid rangevarref lint warning
		result[hex.EncodeToString(k[:])] = v
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

	key := toAuthorKey(cert.Author())
	if lastRound, ok := b.lastCommitted[key]; !ok || cert.Round() > lastRound {
		b.lastCommitted[key] = cert.Round()
	}
}

// SetLastCommittedForAuthor sets the last committed round for an author.
// This is used during crash recovery to restore per-author commit state.
// The authorHex parameter should be a hex-encoded public key.
func (b *Bullshark) SetLastCommittedForAuthor(authorHex string, round types.Round) {
	key, err := authorKeyFromHex(authorHex)
	if err != nil {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if lastRound, ok := b.lastCommitted[key]; !ok || round > lastRound {
		b.lastCommitted[key] = round
	}
}

// SetOnCommit sets the callback function that is invoked when certificates
// are committed. This is used to notify external components (e.g., workers)
// that batches associated with these certificates can be pruned from memory.
// The callback receives the list of committed certificates.
func (b *Bullshark) SetOnCommit(fn func([]*types.Certificate)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onCommit = fn
}
