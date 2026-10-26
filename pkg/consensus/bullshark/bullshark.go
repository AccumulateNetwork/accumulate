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
	"errors"
	"fmt"
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

	// Leader is the round of the committed LEADER whose sub-DAG this
	// certificate belongs to. The leader sequence is deterministic across
	// validators regardless of certificate arrival timing — a node that
	// commits two leaders in one ProcessCertificate call and a node that
	// commits them in two produce identical (Leader, Certificate) sequences —
	// so it is the only safe boundary for grouping certificates into executor
	// blocks (#4164: one block per certificate multiplied end-of-block cost
	// by the committee size).
	Leader types.Round
}

// Bullshark implements the Bullshark consensus ordering algorithm.
// It processes certificates as they are added to the DAG and determines
// when leaders can be committed along with their sub-DAGs.
type Bullshark struct {
	// committees is the committee of each round: a leader is elected and
	// supported by the committee of its round, whatever this node had
	// executed when it ordered that round (#4593).
	committees *types.CommitteeSchedule
	// dag is the certificate DAG.
	dag *dag.DAG
	// partition names the partition in log output. Both partitions of a dual
	// node log through the same slog default; without this tag their commit
	// decisions are indistinguishable (#4054).
	partition string

	// mu protects commit tracking state.
	mu sync.RWMutex
	// lastCommitRound is the most recently committed leader round.
	lastCommitRound types.Round
	// lastCommitted tracks the last committed round for each author.
	// Uses [32]byte key instead of hex string for memory efficiency.
	// Kept for state reporting/persistence; it is NOT the commit dedup —
	// a per-author round watermark silently discards any straggler
	// certificate that commits after a higher-round one (#4111).
	lastCommitted map[authorKey]types.Round
	// committed is the digest-level commit dedup. It is pruned a rescue
	// window below the LOWER of the last commit and dedupFloor (#4594): the
	// checkpoint of the block being produced copies the set as of that
	// block's leader, which can be far below the commit frontier when the
	// executor lags, and a set pruned against the frontier has nothing left
	// for it. After a restart it is restored from the checkpoint (#4290).
	committed map[types.CertificateDigest]types.Round
	// dedupFloor is the oldest leader whose group the executor has not yet
	// produced a block for, as the node reports it; no checkpoint is taken
	// below it. The node never reports one more than its DAG GC depth below
	// the frontier: past that its checkpoint cannot write the tail, and a
	// node that takes no checkpoints would otherwise never move it. Zero
	// means none reported and the frontier is the floor.
	dedupFloor types.Round

	// stranded is set while ordering refuses to proceed over collected
	// rounds (#4595): the refusal is logged once, and the resumption.
	stranded bool

	// onCommit is called when certificates are committed.
	// This callback is used to notify external components (e.g., workers)
	// that batches associated with these certificates can be pruned.
	onCommit func(certificates []*types.Certificate)
}

// New creates a new Bullshark consensus instance.
func New(committee *types.Committee, d *dag.DAG) *Bullshark {
	return &Bullshark{
		committees:    types.NewCommitteeSchedule(committee),
		dag:           d,
		lastCommitted: make(map[authorKey]types.Round),
		committed:     make(map[types.CertificateDigest]types.Round),
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
		committees:      types.NewCommitteeSchedule(committee),
		dag:             d,
		lastCommitRound: lastCommitRound,
		lastCommitted:   committed,
		committed:       make(map[types.CertificateDigest]types.Round),
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
		slog.Debug("Bullshark: no leader certificate",
			"partition", b.partition,
			"leaderRound", leaderRound,
			"dagCertsInRound", len(b.dag.GetRound(leaderRound)))
		return nil
	}

	// Check if leader has f+1 support from round+1 certificates.
	if !b.hasSupport(leader, round) {
		slog.Debug("Bullshark: leader lacks support",
			"partition", b.partition,
			"leaderRound", leaderRound,
			"votingRound", round,
			"votingCerts", len(b.dag.GetRound(round)))
		return nil
	}

	// Leader has support! Commit the leader chain.
	outputs := b.commitLeaderChain(leader)
	slog.Debug("Bullshark: committing leader chain",
		"partition", b.partition,
		"leaderRound", leaderRound,
		"outputs", len(outputs))
	return outputs
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

// GetCommitted is the digest-level commit dedup as hex digests: what the
// ancestor walk must not emit again. A checkpoint carries it, because a
// restart that restored only the round watermarks left it empty and the
// first leader after the restart re-committed every ancestor within
// rescueWindow -- a different block from the peers' (#4290).
func (b *Bullshark) GetCommitted() map[string]types.Round {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make(map[string]types.Round, len(b.committed))
	for d, r := range b.committed {
		d := d
		out[hex.EncodeToString(d[:])] = r
	}
	return out
}

// GetCommittedFrom is GetCommitted limited to the entries committed at round
// floor or above, so a checkpoint copies the window it keeps and not the
// whole set (#4594).
func (b *Bullshark) GetCommittedFrom(floor types.Round) map[string]types.Round {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make(map[string]types.Round)
	for d, r := range b.committed {
		if r < floor {
			continue
		}
		d := d
		out[hex.EncodeToString(d[:])] = r
	}
	return out
}

// SetCommitted restores the digest-level commit dedup from a checkpoint.
func (b *Bullshark) SetCommitted(committed map[string]types.Round) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for h, r := range committed {
		raw, err := hex.DecodeString(h)
		if err != nil || len(raw) != len(types.CertificateDigest{}) {
			continue
		}
		var d types.CertificateDigest
		copy(d[:], raw)
		b.committed[d] = r
	}
}

// SetDedupFloor records the oldest leader whose group has been handed to
// the executor and not yet produced, or the leader just checkpointed when
// none is waiting. No checkpoint is taken below it, so the dedup set is
// kept a rescue window below it rather than below the commit frontier
// (#4594).
func (b *Bullshark) SetDedupFloor(leader types.Round) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dedupFloor = leader
}

// SetLastCommitRound sets the last commit round.
// This is useful for crash recovery.
func (b *Bullshark) SetLastCommitRound(round types.Round) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastCommitRound = round
}

// UpdateCommittee schedules a committee from its ActivationRound. A round
// already ordered keeps the committee it was ordered with: a committee that
// activates at or below the last committed leader is refused.
func (b *Bullshark) UpdateCommittee(committee *types.Committee) error {
	if committee == nil {
		return errors.New("committee is nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if committee.ActivationRound <= b.lastCommitRound {
		return fmt.Errorf("committee of epoch %d activates at round %d, at or below the last committed leader %d",
			committee.Epoch, committee.ActivationRound, b.lastCommitRound)
	}
	return b.committees.Schedule(committee)
}

// SetCommittees makes s the schedule this instance orders by, so a node's
// primary and Bullshark share one.
func (b *Bullshark) SetCommittees(s *types.CommitteeSchedule) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.committees = s
}

// Committees is the schedule this instance orders by.
func (b *Bullshark) Committees() *types.CommitteeSchedule {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.committees
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
	// The digest set is the actual commit dedup.
	b.committed[cert.Digest()] = cert.Round()
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

// SetPartition sets the partition name used to tag log output.
func (b *Bullshark) SetPartition(partition string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.partition = partition
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
