// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #4423: a stream froze with an entry held and validated at every number above
// Delivered (waiting=0) and delivered none of them.
//
// The entry at the head of the stream was COLLECTED -- it arrived before the
// Directory anchor its package proof names, so staging holds the outer
// SyntheticMessage. The anchor executed and validated it, so it is runnable,
// and healing counts it as accounted for. Then a byte-identical copy of the
// same package arrived (a dispatcher retry after a failed dial commits the
// same envelope twice) while the entry was still not next. That copy passes
// as validated, its inner sequenced message is only held, and
// SyntheticMessage.Process records the OUTER message Delivered regardless.
// When the collected original -- the same outer hash -- is run from staging,
// checkStatus answers "already delivered" and nothing executes: the stream
// does not move, the run stops there every block, and no gap is ever
// reported, so nothing is ever asked for.
//
// This must deliver 2 and 3 once 1 arrives. The control, without the second
// copy, does.
func TestStaging_4423_DuplicateCopyAfterValidation_DoesNotWedgeTheStream(t *testing.T) {
	t.Run("control, one copy", func(t *testing.T) { run4423(t, false) })
	t.Run("the same package committed twice", func(t *testing.T) { run4423(t, true) })
}

func run4423(t *testing.T, duplicate bool) {
	s := newStagingSim(t, 6)

	// Entries 2..3 arrive as a package under Directory anchor 4, which has not
	// executed here, while 1 is missing: collected.
	for _, c := range s.packageArrives(1, 2, 4) {
		require.Equal(t, "pending", c.String(), "collected")
	}
	require.True(t, s.collected(2))
	require.True(t, s.collected(3))

	// Anchor 4 executes: the waiting proof validates 2..3.
	s.newBlock()
	s.anchorExecutes(4, s.rootAt(4))
	require.True(t, s.proven(1))
	require.True(t, s.proven(2))

	// The SAME package envelope is committed a second time while 1 is still
	// missing (a retried submission). 2..3 are validated, so the copies pass,
	// and are not next, so the sequenced layer only holds them.
	s.newBlock()
	if duplicate {
		s.packageArrives(1, 2, 4)
		require.True(t, s.collected(2), "first sighting wins: the collected original is still what is held")

		// The copy's outer message must not be recorded as delivered: its
		// stream has not moved past 1 and its inner sequenced message is only
		// held. Recorded delivered, it is the collected original's status too
		// (the same hash), and the original can never run.
		require.Equal(t, uint64(0), s.delivered())
		st, err := s.batch.Transaction2(s.member(1).Hash()).Status().Get()
		require.NoError(t, err)
		assert.False(t, st.Delivered(), "a held copy is recorded as %v", st.Code)
	}

	// 1 arrives, under the anchor that has executed, and delivers.
	s.newBlock()
	s.packageArrives(0, 0, 4)
	require.Equal(t, uint64(1), s.delivered())

	// The next block. Staging says there is nothing missing and nothing unproven above 1:
	// healing will never ask for 2.
	s.newBlock()
	st := s.b.staging.Status(s.str.id())
	require.Zero(t, st.Waiting, "no hole: every number above Delivered is held")
	require.True(t, s.proven(1), "and 2 is validated, so healing counts it as accounted for")
	pos, err := s.b.positionOf(s.str)
	require.NoError(t, err)
	require.True(t, pos.runnable(2), "and staging offers it to the run")

	// So the run must deliver it.
	require.Equal(t, []uint64{2, 3}, s.run(), "the held, validated entries must execute")
	require.Equal(t, uint64(3), s.delivered())
}
