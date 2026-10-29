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
	"time"

	"github.com/stretchr/testify/require"
)

// A node's state diverges from its partition's only when a quorum of the
// committee reports one other hash for the round: one validator's report, or
// a split, says nothing about which state is the partition's (consensus spec,
// "State hash comparison").
func TestStateHashTracker_DivergenceNeedsAQuorum(t *testing.T) {
	keys := make([]ed25519.PublicKey, 4)
	infos := make([]ValidatorInfo, 4)
	for i := range keys {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys[i] = pub
		infos[i] = ValidatorInfo{PublicKey: pub, Stake: 1}
	}
	committee := NewCommittee(infos, 1)
	outsider, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	local, other, third := StateHash{0xAA}, StateHash{0xBB}, StateHash{0xCC}
	at6 := StatePosition{Epoch: 1, Round: 6, BlockIndex: 3}

	newTracker := func() (*StateHashTracker, chan *StateDivergenceError) {
		tr := NewStateHashTracker(100)
		tr.SetCommittee(committee)
		got := make(chan *StateDivergenceError, 8)
		tr.OnDivergence(func(err *StateDivergenceError) { got <- err })
		return tr, got
	}
	none := func(t *testing.T, got chan *StateDivergenceError) {
		t.Helper()
		select {
		case err := <-got:
			t.Fatalf("divergence reported: %+v", err)
		case <-time.After(50 * time.Millisecond):
		}
	}

	t.Run("one validator", func(t *testing.T) {
		tr, got := newTracker()
		tr.RecordLocalHash(at6, local)
		require.Nil(t, tr.RecordRemoteHash(at6, keys[1], other))
		none(t, got)
	})

	t.Run("a split", func(t *testing.T) {
		tr, got := newTracker()
		tr.RecordLocalHash(at6, local)
		require.Nil(t, tr.RecordRemoteHash(at6, keys[1], other))
		require.Nil(t, tr.RecordRemoteHash(at6, keys[2], third))
		require.Nil(t, tr.RecordRemoteHash(at6, keys[3], other))
		none(t, got)
	})

	t.Run("not in the committee", func(t *testing.T) {
		tr, got := newTracker()
		tr.RecordLocalHash(at6, local)
		for i := 0; i < 4; i++ {
			require.Nil(t, tr.RecordRemoteHash(at6, outsider, other))
		}
		none(t, got)
	})

	t.Run("a quorum", func(t *testing.T) {
		tr, got := newTracker()
		tr.RecordLocalHash(at6, local)
		require.Nil(t, tr.RecordRemoteHash(at6, keys[1], other))
		require.Nil(t, tr.RecordRemoteHash(at6, keys[2], other))
		err := tr.RecordRemoteHash(at6, keys[3], other)
		require.NotNil(t, err)
		require.Equal(t, local, err.ExpectedHash)
		require.Equal(t, other, err.ActualHash)
		require.NotNil(t, <-got)
	})

	t.Run("a quorum at another position", func(t *testing.T) {
		for _, pos := range []StatePosition{
			{Epoch: 2, Round: 6, BlockIndex: 3},
			{Epoch: 1, Round: 6, BlockIndex: 4},
		} {
			tr, got := newTracker()
			tr.RecordLocalHash(at6, local)
			for _, k := range keys[1:] {
				require.Nil(t, tr.RecordRemoteHash(pos, k, other))
			}
			none(t, got)
		}
	})

	t.Run("reports of validators the committee no longer has", func(t *testing.T) {
		tr, got := newTracker()
		tr.RecordLocalHash(at6, local)
		require.Nil(t, tr.RecordRemoteHash(at6, keys[1], other))
		require.Nil(t, tr.RecordRemoteHash(at6, keys[2], other))
		// keys[1] and keys[2] leave; the committee is keys[0], keys[3] and
		// two newcomers.
		infos := []ValidatorInfo{{PublicKey: keys[0], Stake: 1}, {PublicKey: keys[3], Stake: 1}}
		for i := 0; i < 2; i++ {
			pub, _, err := ed25519.GenerateKey(rand.Reader)
			require.NoError(t, err)
			infos = append(infos, ValidatorInfo{PublicKey: pub, Stake: 1})
		}
		tr.SetCommittee(NewCommittee(infos, 2))
		require.Nil(t, tr.RecordRemoteHash(at6, keys[3], other))
		none(t, got)
	})

	t.Run("a quorum before the local hash", func(t *testing.T) {
		tr, got := newTracker()
		for _, k := range keys[1:] {
			require.Nil(t, tr.RecordRemoteHash(at6, k, other))
		}
		none(t, got)
		tr.RecordLocalHash(at6, local)
		require.NotNil(t, <-got)
	})
}

// A state hash message is signed for one partition: changing the partition
// it names breaks the signature.
func TestStateHashMessage_SignedForItsPartition(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	msg := NewStateHashMessage("BVN0", 6, 1, 3, StateHash{0xAA}, pub)
	require.NoError(t, msg.Sign(priv))
	require.NoError(t, msg.Verify())

	data, err := msg.Marshal()
	require.NoError(t, err)
	got, err := UnmarshalStateHashMessage(data)
	require.NoError(t, err)
	require.Equal(t, msg, got)

	msg.Partition = "Directory"
	require.Error(t, msg.Verify())
}

// A committee member holds one report per round, its latest: varying the
// position does not grow the tracker.
func TestStateHashTracker_OneReportPerMemberPerRound(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	tr := NewStateHashTracker(100)
	tr.SetCommittee(NewCommittee([]ValidatorInfo{{PublicKey: pub, Stake: 1}}, 1))

	for block := uint64(0); block < 100; block++ {
		require.Nil(t, tr.RecordRemoteHash(StatePosition{Epoch: 1, Round: 6, BlockIndex: block}, pub, StateHash{0xBB}))
	}
	reports := 0
	for _, r := range tr.stateHashes {
		reports += len(r)
	}
	require.Equal(t, 1, reports)
	require.Contains(t, tr.stateHashes, StatePosition{Epoch: 1, Round: 6, BlockIndex: 99}, "the latest report is kept")
}

// A report for a round counts in the committee of that round (#4593): across
// a change scheduled at an activation round, the committee before it decides
// the rounds below that round and the new one the rounds from it, and a key
// in one committee only counts for nothing in the other's rounds.
func TestStateHashTracker_CountsTheCommitteeOfTheRound(t *testing.T) {
	keys := make([]ed25519.PublicKey, 5)
	for i := range keys {
		pub, _, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys[i] = pub
	}
	infos := func(ks ...ed25519.PublicKey) []ValidatorInfo {
		var v []ValidatorInfo
		for _, k := range ks {
			v = append(v, ValidatorInfo{PublicKey: k, Stake: 1})
		}
		return v
	}
	// Epoch 1 is keys 0-3; epoch 2, from round 100, drops key 3 and adds
	// key 4.
	before := NewCommittee(infos(keys[0], keys[1], keys[2], keys[3]), 1)
	after := NewCommittee(infos(keys[0], keys[1], keys[2], keys[4]), 2)
	after.ActivationRound = 100
	schedule := NewCommitteeSchedule(before)

	tr := NewStateHashTracker(1000)
	tr.SetSchedule(schedule)
	local, other := StateHash{0xAA}, StateHash{0xBB}

	// Scheduled after the tracker took the schedule: it applies all the same.
	require.NoError(t, schedule.Schedule(after))

	// Below the activation round the committee before decides: the position
	// is at its epoch, a report naming the new epoch there is not recorded,
	// key 4 is not recorded, and key 3 counts.
	at98 := StatePosition{Epoch: 1, Round: 98, BlockIndex: 10}
	tr.RecordLocalHash(at98, local)
	epoch, ok := tr.EpochAt(98)
	require.True(t, ok)
	require.Equal(t, uint64(1), epoch)
	require.Nil(t, tr.RecordRemoteHash(StatePosition{Epoch: 2, Round: 98, BlockIndex: 10}, keys[2], other))
	require.Empty(t, tr.GetRemoteHashes(98), "a report naming another epoch than its round's committee's is not recorded")
	require.Nil(t, tr.RecordRemoteHash(at98, keys[4], other))
	require.NotContains(t, tr.GetRemoteHashes(98), hexEncode(keys[4]), "a key outside the round's committee is not recorded")
	require.Nil(t, tr.RecordRemoteHash(at98, keys[0], other))
	require.Nil(t, tr.RecordRemoteHash(at98, keys[1], other))
	require.NotNil(t, tr.RecordRemoteHash(at98, keys[3], other), "the committee of round 98 includes key 3")

	// From the activation round the new committee decides: key 3 is not
	// recorded, and key 4 counts.
	at100 := StatePosition{Epoch: 2, Round: 100, BlockIndex: 11}
	tr.RecordLocalHash(at100, local)
	require.Nil(t, tr.RecordRemoteHash(at100, keys[3], other))
	require.NotContains(t, tr.GetRemoteHashes(100), hexEncode(keys[3]), "a key that left is not recorded for the new committee's rounds")
	require.Nil(t, tr.RecordRemoteHash(at100, keys[0], other))
	require.Nil(t, tr.RecordRemoteHash(at100, keys[1], other))
	require.NotNil(t, tr.RecordRemoteHash(at100, keys[4], other), "the committee of round 100 includes key 4")
}
