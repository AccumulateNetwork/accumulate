// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// TestASourceBelowItsQuorumIsDegradedAndItsHoldBounded: a stream whose head
// is held for an anchor below quorum for DegradedAfterBlocks is degraded;
// staging then holds at most DegradedHoldLimit entries for it and refuses
// the rest; the head running clears it (executor spec, "A source below its
// quorum"). Run 20261002T145702Z: the Directory held 2,000 of a two-signer
// partition's anchors until it was out of memory (#4510).
func TestASourceBelowItsQuorumIsDegradedAndItsHoldBounded(t *testing.T) {
	s := NewStaging()
	id := StreamID{Ledger: url.MustParse("acc://dn.acme/anchors"), Source: url.MustParse("acc://bvn-BVN2.acme")}

	// The head and a few more arrive and are held.
	tx := s.Begin()
	for n := uint64(1); n <= 10; n++ {
		tx.Hold(id, n, held(n))
	}
	tx.Commit()
	require.Equal(t, 10, s.streams[id.key()].held)

	// Held below quorum, block after block, for less than the bound: not
	// degraded, and more is held.
	for b := uint64(100); b < 100+DegradedAfterBlocks; b++ {
		tx := s.Begin()
		tx.NoteBelowQuorum(id, b)
		require.False(t, tx.Degraded(id), "block %d", b)
		tx.Commit()
	}
	tx = s.Begin()
	for n := uint64(11); n <= DegradedHoldLimit+50; n++ {
		tx.Hold(id, n, held(n))
	}
	tx.Commit()
	require.Equal(t, DegradedHoldLimit+50, s.streams[id.key()].held, "before the bound every entry is held")

	// One more block below quorum makes the bound: degraded.
	tx = s.Begin()
	tx.NoteBelowQuorum(id, 100+DegradedAfterBlocks)
	require.True(t, tx.Degraded(id))
	tx.Commit()

	// Degraded and over the limit: nothing more is held.
	tx = s.Begin()
	for n := uint64(DegradedHoldLimit + 51); n <= DegradedHoldLimit+100; n++ {
		tx.Hold(id, n, held(n))
	}
	tx.Commit()
	require.Equal(t, DegradedHoldLimit+50, s.streams[id.key()].held, "a degraded stream over its limit holds nothing more")

	// The head runs: the stream is no longer degraded and holds again.
	tx = s.Begin()
	tx.Release(id, 1)
	tx.Commit()
	require.False(t, s.Begin().Degraded(id), "the head running clears the degradation")
	tx = s.Begin()
	tx.Hold(id, DegradedHoldLimit+101, held(DegradedHoldLimit+101))
	tx.Commit()
	require.Equal(t, DegradedHoldLimit+50, s.streams[id.key()].held, "one released, one held")
}
