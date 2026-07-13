// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package dag implements the DAG (Directed Acyclic Graph) structure
// for organizing certificates in the Bullshark consensus algorithm.
package dag

import (
	"crypto/ed25519"
	"errors"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// authorKey is a fixed-size array for author public key map keys.
// Using [32]byte instead of string saves 32 bytes per entry.
type authorKey [32]byte

// toAuthorKey converts an ed25519 public key to an authorKey.
func toAuthorKey(author ed25519.PublicKey) authorKey {
	var key authorKey
	copy(key[:], author)
	return key
}

// DAG stores certificates organized by round.
// It provides thread-safe access to the certificate graph.
type DAG struct {
	mu sync.RWMutex
	// rounds maps round numbers to certificates by author.
	// Uses fixed-size [32]byte key instead of hex string for memory efficiency.
	rounds map[types.Round]map[authorKey]*types.Certificate
	// digestIndex provides O(1) lookup by certificate digest.
	// This eliminates linear scans in ancestor traversals.
	digestIndex map[types.CertificateDigest]*types.Certificate
	// gcDepth is the number of rounds to keep after the last commit.
	gcDepth types.Round
	// lastCommitRound is the most recently committed round.
	lastCommitRound types.Round
	// latestRound is the highest round number seen.
	latestRound types.Round
}

// NewDAG creates a new DAG with the specified garbage collection depth.
// The gcDepth parameter determines how many rounds of history are kept
// after garbage collection.
func NewDAG(gcDepth types.Round) *DAG {
	return &DAG{
		rounds:      make(map[types.Round]map[authorKey]*types.Certificate),
		digestIndex: make(map[types.CertificateDigest]*types.Certificate),
		gcDepth:     gcDepth,
	}
}

// Insert adds a certificate to the DAG.
// It validates that all parent certificates exist (except for round 0).
// Returns an error if parents are missing or if a certificate for this
// author and round already exists.
func (d *DAG) Insert(cert *types.Certificate) error {
	if cert == nil {
		return errors.New("certificate is nil")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	round := cert.Round()
	key := toAuthorKey(cert.Author())

	// Check if certificate already exists
	if roundMap, ok := d.rounds[round]; ok {
		if _, exists := roundMap[key]; exists {
			return errors.New("certificate already exists for this author and round")
		}
	}

	// Validate parents exist (skip for round 0 which has no parents).
	// Once there is a commit floor, parents at or below it may be
	// legitimately absent: garbage collection prunes them, and a node
	// rejoining after a fast sync (#4058) starts with an EMPTY DAG whose
	// history is embodied in its restored state — requiring parents there
	// recurses into pruned history and wedges the rejoin. The certificate
	// itself is quorum-verified before insertion, so accepting it without
	// its pruned parents does not weaken trust.
	pruned := d.lastCommitRound > 0 && round <= d.lastCommitRound+1
	if round > 0 && !pruned && len(cert.Parents()) > 0 {
		for _, parentDigest := range cert.Parents() {
			// Use digest index for O(1) lookup
			if _, found := d.digestIndex[parentDigest]; !found {
				return errors.New("parent certificate not found in DAG")
			}
		}
	}

	// Create round map if needed
	if _, ok := d.rounds[round]; !ok {
		d.rounds[round] = make(map[authorKey]*types.Certificate)
	}

	// Insert certificate
	d.rounds[round][key] = cert

	// Add to digest index for O(1) lookups
	d.digestIndex[cert.Digest()] = cert

	// Update latest round
	if round > d.latestRound {
		d.latestRound = round
	}

	return nil
}

// InsertGenesis inserts a genesis certificate (round 0) without parent validation.
func (d *DAG) InsertGenesis(cert *types.Certificate) error {
	if cert == nil {
		return errors.New("certificate is nil")
	}

	if cert.Round() != 0 {
		return errors.New("genesis certificate must be round 0")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	key := toAuthorKey(cert.Author())

	// Check if certificate already exists
	if roundMap, ok := d.rounds[0]; ok {
		if _, exists := roundMap[key]; exists {
			return errors.New("certificate already exists for this author and round")
		}
	}

	// Create round map if needed
	if _, ok := d.rounds[0]; !ok {
		d.rounds[0] = make(map[authorKey]*types.Certificate)
	}

	// Insert certificate
	d.rounds[0][key] = cert

	// Add to digest index
	d.digestIndex[cert.Digest()] = cert

	return nil
}

// Get retrieves a certificate by round and author.
// Returns nil if not found.
func (d *DAG) Get(round types.Round, author ed25519.PublicKey) *types.Certificate {
	d.mu.RLock()
	defer d.mu.RUnlock()

	key := toAuthorKey(author)

	if roundMap, ok := d.rounds[round]; ok {
		return roundMap[key]
	}

	return nil
}

// GetByDigest retrieves a certificate by its digest.
// Uses O(1) digest index lookup instead of linear scan.
func (d *DAG) GetByDigest(digest types.CertificateDigest) *types.Certificate {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.digestIndex[digest]
}

// GetRound retrieves all certificates for a given round.
// Returns nil if no certificates exist for that round.
func (d *DAG) GetRound(round types.Round) []*types.Certificate {
	d.mu.RLock()
	defer d.mu.RUnlock()

	roundMap, ok := d.rounds[round]
	if !ok {
		return nil
	}

	certs := make([]*types.Certificate, 0, len(roundMap))
	for _, cert := range roundMap {
		certs = append(certs, cert)
	}

	return certs
}

// HasQuorum returns true if the given round has certificates from validators
// with at least 2f+1 stake.
func (d *DAG) HasQuorum(round types.Round, committee *types.Committee) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	roundMap, ok := d.rounds[round]
	if !ok {
		return false
	}

	var totalStake uint64
	for _, cert := range roundMap {
		totalStake += committee.StakeOf(cert.Author())
	}

	return committee.HasQuorum(totalStake)
}

// LatestRound returns the highest round number in the DAG.
func (d *DAG) LatestRound() types.Round {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.latestRound
}

// LastCommitRound returns the most recently committed round.
func (d *DAG) LastCommitRound() types.Round {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.lastCommitRound
}

// SetLastCommitRound sets the last committed round.
// This is typically called after consensus commits a leader.
func (d *DAG) SetLastCommitRound(round types.Round) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if round > d.lastCommitRound {
		d.lastCommitRound = round
	}
}

// GarbageCollect removes rounds older than (commitRound - gcDepth).
// This should be called periodically to prevent unbounded memory growth.
func (d *DAG) GarbageCollect(commitRound types.Round) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if commitRound <= d.gcDepth {
		return
	}

	cutoff := commitRound - d.gcDepth

	for round := range d.rounds {
		if round < cutoff {
			// Remove certificates from digest index before deleting round
			for _, cert := range d.rounds[round] {
				delete(d.digestIndex, cert.Digest())
			}
			delete(d.rounds, round)
		}
	}

	if commitRound > d.lastCommitRound {
		d.lastCommitRound = commitRound
	}
}

// RoundCount returns the number of certificates in a given round.
func (d *DAG) RoundCount(round types.Round) int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if roundMap, ok := d.rounds[round]; ok {
		return len(roundMap)
	}

	return 0
}

// Contains returns true if a certificate with the given digest exists.
func (d *DAG) Contains(digest types.CertificateDigest) bool {
	return d.GetByDigest(digest) != nil
}

// ContainsRoundAuthor returns true if a certificate exists for the given round and author.
func (d *DAG) ContainsRoundAuthor(round types.Round, author ed25519.PublicKey) bool {
	return d.Get(round, author) != nil
}

// Size returns the total number of certificates in the DAG.
func (d *DAG) Size() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	total := 0
	for _, roundMap := range d.rounds {
		total += len(roundMap)
	}

	return total
}

// Rounds returns all round numbers present in the DAG.
func (d *DAG) Rounds() []types.Round {
	d.mu.RLock()
	defer d.mu.RUnlock()

	rounds := make([]types.Round, 0, len(d.rounds))
	for r := range d.rounds {
		rounds = append(rounds, r)
	}

	return rounds
}

// GetParents returns the parent certificates of a given certificate.
// Returns nil for certificates with no parents (round 0).
// Uses O(1) digest index lookup for each parent.
func (d *DAG) GetParents(cert *types.Certificate) []*types.Certificate {
	if cert == nil || len(cert.Parents()) == 0 {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	parents := make([]*types.Certificate, 0, len(cert.Parents()))

	for _, parentDigest := range cert.Parents() {
		if parentCert, ok := d.digestIndex[parentDigest]; ok {
			parents = append(parents, parentCert)
		}
	}

	return parents
}
