// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package types

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func scheduleCommittee(t *testing.T, epoch uint64, activation Round) *Committee {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	c := NewCommittee([]ValidatorInfo{{PublicKey: pub, Stake: 1}}, epoch)
	c.ActivationRound = activation
	return c
}

// Each committee orders the rounds from its activation round up to the next
// one's, and the first every round below as well (#4593).
func TestCommitteeSchedule_EachRoundHasTheCommitteeThatActivatedAtOrBelowIt(t *testing.T) {
	first := scheduleCommittee(t, 1, 0)
	second := scheduleCommittee(t, 2, 300)
	third := scheduleCommittee(t, 3, 600)
	s := NewCommitteeSchedule(first)
	require.NoError(t, s.Schedule(second))
	require.NoError(t, s.Schedule(third))

	require.Same(t, first, s.At(0))
	require.Same(t, first, s.At(299))
	require.Same(t, second, s.At(300))
	require.Same(t, second, s.At(599))
	require.Same(t, third, s.At(600))
	require.Same(t, third, s.At(10_000))
	require.Same(t, third, s.Latest())
}

// A committee is scheduled after the last one, in epoch and in round; the
// same committee again is no change.
func TestCommitteeSchedule_RefusesACommitteeThatIsNotAfterTheLastOne(t *testing.T) {
	s := NewCommitteeSchedule(scheduleCommittee(t, 1, 0))
	second := scheduleCommittee(t, 2, 300)
	require.NoError(t, s.Schedule(second))
	require.NoError(t, s.Schedule(second), "scheduling the same committee again")

	require.Error(t, s.Schedule(scheduleCommittee(t, 2, 400)), "an epoch already scheduled")
	require.Error(t, s.Schedule(scheduleCommittee(t, 1, 400)), "an earlier epoch")
	require.Error(t, s.Schedule(scheduleCommittee(t, 3, 300)), "at the round the last one activated")
	require.Error(t, s.Schedule(scheduleCommittee(t, 3, 200)), "below it")
	require.Len(t, s.Committees(), 2)
}

// Pruning drops only the committees no round at or above the floor belongs
// to, so a certificate of the previous epoch below the activation round still
// has its committee while it can still be caught up.
func TestCommitteeSchedule_PruneKeepsTheCommitteeOfTheFloor(t *testing.T) {
	first := scheduleCommittee(t, 1, 0)
	second := scheduleCommittee(t, 2, 300)
	s := NewCommitteeSchedule(first)
	require.NoError(t, s.Schedule(second))

	s.Prune(299)
	require.Len(t, s.Committees(), 2)
	require.Same(t, first, s.At(299))

	s.Prune(300)
	require.Equal(t, []*Committee{second}, s.Committees())
}

// Replace is how a restart takes the schedule its checkpoint carries.
func TestCommitteeSchedule_ReplaceTakesAWholeSchedule(t *testing.T) {
	s := NewCommitteeSchedule(scheduleCommittee(t, 5, 0))
	first, second := scheduleCommittee(t, 1, 0), scheduleCommittee(t, 2, 300)
	require.NoError(t, s.Replace([]*Committee{first, second}))
	require.Same(t, first, s.At(10))
	require.Same(t, second, s.At(300))

	require.Error(t, s.Replace([]*Committee{second, first}), "out of order")
	require.Same(t, second, s.At(300), "a refused schedule changes nothing")
}
