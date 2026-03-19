// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bullshark

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/dag"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestElectLeader_EmptyRound(t *testing.T) {
	h := newTestHelper(t, 4)
	bs := New(h.committee, h.dag)

	// No certificates in round 2.
	leader := bs.electLeader(2)
	require.Nil(t, leader)
}

func TestElectLeader_Deterministic(t *testing.T) {
	h := newTestHelper(t, 4)
	bs := New(h.committee, h.dag)

	r0 := h.insertGenesis()
	r1 := h.insertRound(1, r0)
	h.insertRound(2, r1)

	// Same round should always return the same leader.
	leader1 := bs.electLeader(2)
	leader2 := bs.electLeader(2)

	require.NotNil(t, leader1)
	require.Equal(t, leader1.Digest(), leader2.Digest())
}

func TestElectLeader_DifferentRounds(t *testing.T) {
	h := newTestHelper(t, 4)
	bs := New(h.committee, h.dag)

	r0 := h.insertGenesis()
	r1 := h.insertRound(1, r0)
	r2 := h.insertRound(2, r1)
	r3 := h.insertRound(3, r2)
	h.insertRound(4, r3)

	// Different rounds may have different leaders.
	leader2 := bs.electLeader(2)
	leader4 := bs.electLeader(4)

	require.NotNil(t, leader2)
	require.NotNil(t, leader4)
	// Leaders may or may not be the same validator depending on the seed.
}

func TestSelectByStake_NilCommittee(t *testing.T) {
	result := selectByStake(nil, []byte("seed"))
	require.Nil(t, result)
}

func TestSelectByStake_EmptyCommittee(t *testing.T) {
	committee := types.NewCommittee(nil, 1)
	result := selectByStake(committee, []byte("seed"))
	require.Nil(t, result)
}

func TestSelectByStake_EqualStakes(t *testing.T) {
	validators := make([]types.ValidatorInfo, 4)
	for i := 0; i < 4; i++ {
		pub, _, _ := ed25519.GenerateKey(rand.Reader)
		validators[i] = types.ValidatorInfo{
			PublicKey: pub,
			Stake:     100,
		}
	}
	committee := types.NewCommittee(validators, 1)

	// Different seeds should potentially select different validators.
	selected := make(map[string]bool)
	for i := 0; i < 100; i++ {
		seed := ComputeLeaderSeed(types.Round(i * 2))
		result := selectByStake(committee, seed)
		require.NotNil(t, result)
		selected[string(result)] = true
	}

	// With 100 different seeds and equal stakes, we should hit multiple validators.
	require.Greater(t, len(selected), 1)
}

func TestSelectByStake_UnequalStakes(t *testing.T) {
	// Validator 0 has 90% stake.
	validators := make([]types.ValidatorInfo, 4)
	for i := 0; i < 4; i++ {
		pub, _, _ := ed25519.GenerateKey(rand.Reader)
		stake := uint64(10)
		if i == 0 {
			stake = 900
		}
		validators[i] = types.ValidatorInfo{
			PublicKey: pub,
			Stake:     stake,
		}
	}
	committee := types.NewCommittee(validators, 1)

	// With 90% stake, validator 0 should be selected most of the time.
	validator0Count := 0
	for i := 0; i < 100; i++ {
		seed := ComputeLeaderSeed(types.Round(i * 2))
		result := selectByStake(committee, seed)
		require.NotNil(t, result)
		if types.ValidatorsEqual(result, validators[0].PublicKey) {
			validator0Count++
		}
	}

	// Should be selected significantly more often.
	require.Greater(t, validator0Count, 70)
}

func TestHasSupport_NoVotingRoundCerts(t *testing.T) {
	h := newTestHelper(t, 4)
	bs := New(h.committee, h.dag)

	r0 := h.insertGenesis()
	r1 := h.insertRound(1, r0)
	r2 := h.insertRound(2, r1)

	leader := r2[0]
	// No round 3 certificates exist.
	hasSupport := bs.hasSupport(leader, 3)
	require.False(t, hasSupport)
}

func TestHasSupport_InsufficientSupport(t *testing.T) {
	h := newTestHelper(t, 4) // f+1 = 34% of 400 = 134 stake.
	bs := New(h.committee, h.dag)

	r0 := h.insertGenesis()
	r1 := h.insertRound(1, r0)
	r2 := h.insertRound(2, r1)

	// Only 1 validator in round 3 (100 stake < 134).
	h.insertPartialRound(3, r2, []int{0})

	leader := bs.electLeader(2)
	if leader != nil {
		hasSupport := bs.hasSupport(leader, 3)
		require.False(t, hasSupport)
	}
}

func TestHasSupport_SufficientSupport(t *testing.T) {
	h := newTestHelper(t, 4)
	bs := New(h.committee, h.dag)

	r0 := h.insertGenesis()
	r1 := h.insertRound(1, r0)
	r2 := h.insertRound(2, r1)

	// All validators in round 3 (400 stake >= 134).
	h.insertRound(3, r2)

	leader := bs.electLeader(2)
	if leader != nil {
		hasSupport := bs.hasSupport(leader, 3)
		require.True(t, hasSupport)
	}
}

func TestHasSupport_PartialReference(t *testing.T) {
	// Test when only some round 3 certs reference the leader.
	h := newTestHelper(t, 4)
	bs := New(h.committee, h.dag)

	r0 := h.insertGenesis()
	r1 := h.insertRound(1, r0)
	r2 := h.insertRound(2, r1)

	// Create round 3 certs that reference round 2.
	// Since all r2 certs are linked and the leader is one of them,
	// all r3 certs should support the leader.
	h.insertRound(3, r2)

	leader := bs.electLeader(2)
	if leader != nil {
		hasSupport := bs.hasSupport(leader, 3)
		require.True(t, hasSupport)
	}
}

func TestLeaderForRound_OddRound(t *testing.T) {
	h := newTestHelper(t, 4)
	bs := New(h.committee, h.dag)

	// Odd rounds have no leader.
	leader := bs.LeaderForRound(1)
	require.Nil(t, leader)

	leader = bs.LeaderForRound(3)
	require.Nil(t, leader)
}

func TestLeaderForRound_RoundZero(t *testing.T) {
	h := newTestHelper(t, 4)
	bs := New(h.committee, h.dag)

	// Round 0 has no leader.
	leader := bs.LeaderForRound(0)
	require.Nil(t, leader)
}

func TestLeaderForRound_ValidRound(t *testing.T) {
	h := newTestHelper(t, 4)
	bs := New(h.committee, h.dag)

	r0 := h.insertGenesis()
	r1 := h.insertRound(1, r0)
	h.insertRound(2, r1)

	leader := bs.LeaderForRound(2)
	require.NotNil(t, leader)
	require.Equal(t, types.Round(2), leader.Round())
}

func TestComputeLeaderSeed_Deterministic(t *testing.T) {
	seed1 := ComputeLeaderSeed(2)
	seed2 := ComputeLeaderSeed(2)

	require.Equal(t, seed1, seed2)
}

func TestComputeLeaderSeed_DifferentRounds(t *testing.T) {
	seed2 := ComputeLeaderSeed(2)
	seed4 := ComputeLeaderSeed(4)

	require.NotEqual(t, seed2, seed4)
}

func TestSelectByStake_SingleValidator(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	validators := []types.ValidatorInfo{
		{PublicKey: pub, Stake: 100},
	}
	committee := types.NewCommittee(validators, 1)

	// With only one validator, they should always be selected.
	for i := 0; i < 10; i++ {
		seed := ComputeLeaderSeed(types.Round(i * 2))
		result := selectByStake(committee, seed)
		require.NotNil(t, result)
		require.True(t, types.ValidatorsEqual(result, pub))
	}
}

func TestSelectByStake_ZeroStake(t *testing.T) {
	// Committee with zero total stake should return nil.
	// But NewCommittee with zero stake validators would fail validation.
	// So we test with empty committee instead.
	committee := &types.Committee{
		Validators: []types.ValidatorInfo{},
		Epoch:      1,
	}

	result := selectByStake(committee, []byte("seed"))
	require.Nil(t, result)
}

func TestElectLeader_MissingLeaderCertificate(t *testing.T) {
	// Create a scenario where the elected leader didn't produce a cert.
	h := newTestHelperWithStakes(t, []uint64{900, 10, 10, 10})
	bs := New(h.committee, h.dag)

	// Insert genesis for all validators.
	r0 := h.insertGenesis()
	r1 := h.insertRound(1, r0)

	// For round 2, only insert certs for validators 1, 2, 3 (not 0).
	// Validator 0 has 90% stake, so should be elected leader often.
	// But without their certificate, electLeader returns nil.
	h.insertPartialRound(2, r1, []int{1, 2, 3})

	// Find which rounds would elect validator 0.
	// Check if round 2 elects validator 0.
	seed := ComputeLeaderSeed(2)
	target := selectByStake(h.committee, seed)

	if types.ValidatorsEqual(target, h.committee.Validators[0].PublicKey) {
		// Validator 0 is the leader, but has no certificate.
		leader := bs.electLeader(2)
		require.Nil(t, leader)
	}
}

func TestHasSupport_IndirectAncestor(t *testing.T) {
	// Test support through indirect ancestry (leader in round N-2).
	h := newTestHelper(t, 4)

	// Create a custom DAG for this test.
	d := dag.NewDAG(50)
	bs := New(h.committee, d)

	// Round 0 (genesis).
	var r0 []*types.Certificate
	for i := range h.keys {
		cert := h.createCert(i, 0, nil)
		require.NoError(t, d.InsertGenesis(cert))
		r0 = append(r0, cert)
	}

	// Round 1.
	var r1 []*types.Certificate
	for i := range h.keys {
		cert := h.createCert(i, 1, r0)
		require.NoError(t, d.Insert(cert))
		r1 = append(r1, cert)
	}

	// Round 2.
	var r2 []*types.Certificate
	for i := range h.keys {
		cert := h.createCert(i, 2, r1)
		require.NoError(t, d.Insert(cert))
		r2 = append(r2, cert)
	}

	// Round 3.
	for i := range h.keys {
		cert := h.createCert(i, 3, r2)
		require.NoError(t, d.Insert(cert))
	}

	// The leader at round 2 should be an ancestor of round 3 certs.
	leader := bs.electLeader(2)
	if leader != nil {
		hasSupport := bs.hasSupport(leader, 3)
		require.True(t, hasSupport)
	}
}
