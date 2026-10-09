// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The overlap after a match ends on its clean run, whatever sections are not
// loaded yet, and a section a block loads ahead of itself -- as a peer held
// it some blocks later -- is settled by the records through its block plus
// SettleMargin, the clean run over or not; a difference there is the record
// reconciling the section, not a mismatch (Paul, 2026-10-09: "We pull the
// accounts for the touched sections from N or maybe N+1. We also apply the
// block ledger for N+1 to N+10 ... We should be able to continue executing
// txs during this process").
func TestASectionLoadedAheadIsSettledByTheRecordsThatFollowIt(t *testing.T) {
	o := &overlap{partition: protocol.PartitionUrl("BVN1"), label: "settle-test", log: slog.Default(), blocks: 3}
	o.start(100)
	for n := uint64(101); n <= 103; n++ {
		require.True(t, o.due(n))
		o.done(n, true, 0)
	}
	require.Equal(t, uint64(103), o.endedAt, "the clean run did not end the overlap")
	require.False(t, o.due(104), "a block after the overlap with nothing loaded ahead was settled")

	// Block 105 loads a section as a peer held it at 108.
	o.loadedAhead(108)
	for n := uint64(105); n <= 108+SettleMargin; n++ {
		require.True(t, o.due(n), "block %d is within the settle window and was not settled", n)
	}
	require.False(t, o.due(108+SettleMargin+1), "the settle window did not close")

	o.done(105, true, 4) // the record put the section's accounts back
	o.done(106, true, 1)
	_, mismatch := o.takeMismatch()
	require.False(t, mismatch, "a reconciled section was taken as a mismatch")
	require.Equal(t, int64(2), o.reconciled)
	require.Equal(t, uint64(103), o.endedAt, "a reconciliation withdrew the overlap's end")

	// A mismatch in the overlap with nothing loaded ahead still goes back
	// to the ledger.
	o.start(200)
	o.done(201, true, 2)
	at, mismatch := o.takeMismatch()
	require.True(t, mismatch)
	require.Equal(t, uint64(201), at)
}
