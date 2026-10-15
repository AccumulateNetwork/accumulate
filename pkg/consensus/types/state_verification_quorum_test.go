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
		tr.RecordLocalHash(6, local)
		require.Nil(t, tr.RecordRemoteHash(6, keys[1], other))
		none(t, got)
	})

	t.Run("a split", func(t *testing.T) {
		tr, got := newTracker()
		tr.RecordLocalHash(6, local)
		require.Nil(t, tr.RecordRemoteHash(6, keys[1], other))
		require.Nil(t, tr.RecordRemoteHash(6, keys[2], third))
		require.Nil(t, tr.RecordRemoteHash(6, keys[3], other))
		none(t, got)
	})

	t.Run("not in the committee", func(t *testing.T) {
		tr, got := newTracker()
		tr.RecordLocalHash(6, local)
		for i := 0; i < 4; i++ {
			require.Nil(t, tr.RecordRemoteHash(6, outsider, other))
		}
		none(t, got)
	})

	t.Run("a quorum", func(t *testing.T) {
		tr, got := newTracker()
		tr.RecordLocalHash(6, local)
		require.Nil(t, tr.RecordRemoteHash(6, keys[1], other))
		require.Nil(t, tr.RecordRemoteHash(6, keys[2], other))
		err := tr.RecordRemoteHash(6, keys[3], other)
		require.NotNil(t, err)
		require.Equal(t, local, err.ExpectedHash)
		require.Equal(t, other, err.ActualHash)
		require.NotNil(t, <-got)
	})

	t.Run("a quorum before the local hash", func(t *testing.T) {
		tr, got := newTracker()
		for _, k := range keys[1:] {
			require.Nil(t, tr.RecordRemoteHash(6, k, other))
		}
		none(t, got)
		tr.RecordLocalHash(6, local)
		require.NotNil(t, <-got)
	})
}
