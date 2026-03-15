// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bullshark

import (
	"crypto/sha256"
	"encoding/binary"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// electLeader performs deterministic leader election based on round number.
// The leader is selected using stake-weighted selection seeded by the round.
// This ensures all honest nodes elect the same leader for the same round.
func (b *Bullshark) electLeader(round types.Round) *types.Certificate {
	// Get all certificates in this round.
	certs := b.dag.GetRound(round)
	if len(certs) == 0 {
		return nil
	}

	// Deterministic selection: hash(round) to select validator.
	// This creates a pseudo-random seed based on the round number.
	seed := sha256.Sum256(binary.BigEndian.AppendUint64(nil, uint64(round)))

	// Select leader based on seed and stake.
	leaderPubKey := selectByStake(b.committee, seed[:])
	if leaderPubKey == nil {
		return nil
	}

	// Find the leader's certificate in this round.
	for _, cert := range certs {
		if types.ValidatorsEqual(cert.Author(), leaderPubKey) {
			return cert
		}
	}

	// Leader didn't produce a certificate for this round.
	return nil
}

// selectByStake selects a validator using stake-weighted random selection.
// The seed is used to generate a pseudo-random number that is mapped to
// a validator based on their stake proportion.
func selectByStake(committee *types.Committee, seed []byte) []byte {
	if committee == nil || committee.Len() == 0 {
		return nil
	}

	totalStake := committee.TotalStake()
	if totalStake == 0 {
		return nil
	}

	// Convert seed to a number in range [0, totalStake).
	// We use the first 8 bytes of the seed as a uint64.
	var seedNum uint64
	if len(seed) >= 8 {
		seedNum = binary.BigEndian.Uint64(seed[:8])
	}

	// Map to stake range.
	target := seedNum % totalStake

	// Find the validator whose stake range includes target.
	var accumulated uint64
	for _, v := range committee.Validators {
		accumulated += v.Stake
		if target < accumulated {
			return v.PublicKey
		}
	}

	// Fallback to last validator (shouldn't happen with valid input).
	return committee.Validators[committee.Len()-1].PublicKey
}

// hasSupport checks if the leader has f+1 (validity threshold) support
// from certificates in the voting round.
// Support means the certificate references the leader (directly or as an ancestor).
func (b *Bullshark) hasSupport(leader *types.Certificate, votingRound types.Round) bool {
	certs := b.dag.GetRound(votingRound)
	if len(certs) == 0 {
		return false
	}

	var supportStake uint64
	for _, cert := range certs {
		// Check if cert references leader (directly or transitively).
		if b.dag.IsAncestor(leader, cert) {
			supportStake += b.committee.StakeOf(cert.Author())
		}
	}

	// Need f+1 (validity threshold) support.
	return supportStake >= b.committee.ValidityThreshold()
}

// LeaderForRound returns the elected leader for a given round.
// Returns nil if the round is not a leader round (odd) or no leader exists.
// This is primarily for testing and debugging.
func (b *Bullshark) LeaderForRound(round types.Round) *types.Certificate {
	// Only even rounds have leaders.
	if round%2 != 0 || round < 2 {
		return nil
	}
	return b.electLeader(round)
}

// ComputeLeaderSeed returns the seed used for leader election at a given round.
// This is primarily for testing and debugging.
func ComputeLeaderSeed(round types.Round) []byte {
	seed := sha256.Sum256(binary.BigEndian.AppendUint64(nil, uint64(round)))
	return seed[:]
}
