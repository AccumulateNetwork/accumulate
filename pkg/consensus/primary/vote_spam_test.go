// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestDuplicateVoteSpamAttack(t *testing.T) {
	// Critical security test for issue #3869: Verify that duplicate votes are rejected BEFORE
	// counting against the vote limit. This prevents a spam attack where a single
	// malicious validator sends many duplicate votes to fill the vote limit and
	// block legitimate votes from other validators.

	// Create 25 validators: f = 8, quorum = 17, max_votes = 34
	validators := make([]*testValidator, 25)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	// Create header from validator 0
	header := types.NewHeader(validators[0].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	digest := header.Digest()
	p.pendingMu.Lock()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil
	p.pendingMu.Unlock()

	// Verify our test parameters
	quorumCount := committee.QuorumCount()
	require.Equal(t, 17, quorumCount)

	maxVotes := quorumCount * VotesPerHeaderMultiplier
	require.Equal(t, 34, maxVotes)

	// ATTACK: Malicious validator 1 tries to spam with many duplicate votes
	// Send 40 duplicate votes (exceeding the maxVotes limit)
	attackerVote := types.NewVote(digest, 0, 1, validators[1].pub)
	require.NoError(t, attackerVote.Sign(validators[1].priv))

	for i := 0; i < 40; i++ {
		p.OnVoteReceived(attackerVote)
	}

	// CRITICAL: Should only have ONE vote from the attacker (all duplicates rejected)
	require.Equal(t, 1, p.PendingVoteCount(digest), "Duplicate votes should be rejected")

	// DEFENSE: Now legitimate validators can still vote
	// Send votes from validators 2-17 (16 more validators = 17 total including attacker)
	for i := 2; i < 18; i++ {
		vote := types.NewVote(digest, 0, 1, validators[i].pub)
		require.NoError(t, vote.Sign(validators[i].priv))
		p.OnVoteReceived(vote)
	}

	// Should have 17 votes now (one from attacker + 16 from legitimate validators)
	// But certificate will be created at quorum (17), so pending votes are cleaned
	time.Sleep(100 * time.Millisecond)

	// Certificate should be created successfully (not blocked by spam attack)
	require.True(t, p.HasCertificateForRound(0), "Certificate should be created despite spam attack")
}

func TestDuplicateVoteRejectionBeforeLimitCheck(t *testing.T) {
	// Test that duplicate votes are rejected BEFORE the vote limit check
	// This is the core fix for issue #3869

	validators := make([]*testValidator, 7)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition: "test",
		KeyPair:   validators[0].priv,
	}

	p := New(config, committee, nil, d, nil)

	header := types.NewHeader(validators[0].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	digest := header.Digest()
	p.pendingMu.Lock()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil
	p.pendingMu.Unlock()

	// Send first vote from validator 1
	vote1 := types.NewVote(digest, 0, 1, validators[1].pub)
	require.NoError(t, vote1.Sign(validators[1].priv))
	p.OnVoteReceived(vote1)

	require.Equal(t, 1, p.PendingVoteCount(digest))

	// Send duplicate vote from validator 1
	vote2 := types.NewVote(digest, 0, 1, validators[1].pub)
	require.NoError(t, vote2.Sign(validators[1].priv))
	p.OnVoteReceived(vote2)

	// Should still be 1 (duplicate rejected)
	require.Equal(t, 1, p.PendingVoteCount(digest), "Duplicate vote should be rejected")

	// Send vote from different validator
	vote3 := types.NewVote(digest, 0, 1, validators[2].pub)
	require.NoError(t, vote3.Sign(validators[2].priv))
	p.OnVoteReceived(vote3)

	// Should now be 2
	require.Equal(t, 2, p.PendingVoteCount(digest), "Unique vote should be accepted")
}

func TestByzantineSpamAttackWith9Of25Validators(t *testing.T) {
	// Realistic Byzantine attack scenario for issue #3869:
	// 9 out of 25 validators (within f=8 tolerance) try to spam with
	// duplicate votes to exhaust the vote limit
	//
	// Setup: 25 validators, f=8, quorum=17, max_votes=34
	// Attack: 9 Byzantine validators each send 10 duplicate votes (90 total spam votes)
	// Expected: Only 9 votes counted (one per Byzantine validator)
	// Defense: 16 honest validators can still vote and reach quorum

	validators := make([]*testValidator, 25)
	for i := range validators {
		validators[i] = newTestValidator(t)
	}

	committee := newTestCommittee(validators, 1)
	d := newTestDAG()

	config := Config{
		Partition:           "test",
		KeyPair:             validators[0].priv,
		NewCertsChannelSize: 10,
	}

	p := New(config, committee, nil, d, nil)

	header := types.NewHeader(validators[0].pub, 0, 1, nil, nil)
	require.NoError(t, header.Sign(validators[0].priv))

	digest := header.Digest()
	p.pendingMu.Lock()
	p.ourHeaders[digest] = header
	p.pendingVotes[digest] = nil
	p.pendingMu.Unlock()

	quorumCount := committee.QuorumCount()
	require.Equal(t, 17, quorumCount)

	// ATTACK PHASE: 9 Byzantine validators (validators 1-9) spam with duplicates
	byzantineCount := 9
	spamPerByzantine := 10

	for i := 1; i <= byzantineCount; i++ {
		vote := types.NewVote(digest, 0, 1, validators[i].pub)
		require.NoError(t, vote.Sign(validators[i].priv))

		// Each Byzantine validator sends 10 duplicate votes
		for j := 0; j < spamPerByzantine; j++ {
			p.OnVoteReceived(vote)
		}
	}

	// After spam attack: should only have 9 votes (one per Byzantine validator)
	voteCount := p.PendingVoteCount(digest)
	require.Equal(t, byzantineCount, voteCount,
		"Should only count unique votes from Byzantine validators")

	// DEFENSE PHASE: Honest validators (validators 10-25) vote normally
	// We need 17 total votes for quorum, we have 9, so we need 8 more honest votes
	for i := 10; i < 18; i++ {
		vote := types.NewVote(digest, 0, 1, validators[i].pub)
		require.NoError(t, vote.Sign(validators[i].priv))
		p.OnVoteReceived(vote)
	}

	// Give time for certificate creation
	time.Sleep(100 * time.Millisecond)

	// VERIFICATION: Certificate should be created (spam attack was ineffective)
	require.True(t, p.HasCertificateForRound(0),
		"Certificate should be created despite Byzantine spam attack")

	// Verify certificate exists in DAG
	cert := d.Get(0, validators[0].pub)
	require.NotNil(t, cert, "Certificate should be in DAG")

	// Verify certificate is valid
	require.NoError(t, cert.Verify(committee), "Certificate should be valid")
}
