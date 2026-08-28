// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The sequence ledger's cost model, pinned (#4164).
//
// The executor used to read the ledger with GetAs inside each message's own
// child batch (updateLedger, deleted in #4169 step 7). A child batch does not
// share its parent's value: the read runs copyValue, which DEEP COPIES the
// whole SyntheticLedger — every stream, and every entry of every stream's
// pending window. So the per-message cost was O(total backlog), and draining
// a backlog of n cost O(n^2). This test models both access patterns directly
// so the cost model stays pinned whatever the executor does;
// TestStreamAdvance_CostDoesNotScaleWithBacklog is the same claim made
// against the live advance path.
//
// This is NOT re-marshaling, which is what the old comment used to say.
// Measured by timing the three steps separately: `put` and `commit` are
// flat at ~0.2us and ~1.5us regardless of backlog, while `get` runs
// 1.1us -> 78.6us across backlogs of 100 -> 16,000. Writes are pointer
// assignments into the parent record and marshal once at the block's commit;
// the read is the whole cost.
//
// It matters because it decides the fix. Splitting the ledger into one record
// per stream — the obvious layout change — does not fix it: a single stream's
// own backlog is still copied per message. Reading the ledger ONCE PER BLOCK
// does fix it, and needs no layout change at all. Measured on the same
// workload at a backlog of 16,000: 80.8us per message becomes 0.32us, a ~250x
// reduction, because the copy is paid once for the block instead of once per
// message. In allocation terms at that backlog: 133,123 B/msg becomes
// 1,419 B/msg, and the gap widens as the drain gets longer.
//
// Asserted on allocation rather than time, so it does not flake.
func TestSequenceLedgerCostIsPerRead(t *testing.T) {
	synth := protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic)
	src := protocol.PartitionUrl("BVN1")

	// perMessageBytes reports bytes allocated per message for each access
	// pattern at the given backlog.
	measure := func(backlog uint64) (perMessage, oncePerBlock uint64) {
		const N = 100

		db := database.OpenInMemory(nil)
		root := db.Begin(true)
		ledger := new(protocol.SyntheticLedger)
		ledger.Url = synth
		part := ledger.Partition(src)
		part.Received = backlog
		part.Pending = make([]*url.TxID, backlog)
		for i := uint64(0); i < backlog; i++ {
			part.Pending[i] = synth.WithTxID([32]byte{byte(i), byte(i >> 8)})
		}
		require.NoError(t, root.Account(synth).Main().Put(ledger))
		require.NoError(t, root.Commit())

		alloc := func(fn func()) uint64 {
			var a, b runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&a)
			fn()
			runtime.ReadMemStats(&b)
			return b.TotalAlloc - a.TotalAlloc
		}

		// What the executor does today: a child batch per message.
		batch := db.Begin(true)
		var warm *protocol.SyntheticLedger
		require.NoError(t, batch.Account(synth).Main().GetAs(&warm)) // pay the unmarshal once, as both patterns do
		perMessage = alloc(func() {
			for i := 0; i < N; i++ {
				child := batch.Begin(true)
				var l *protocol.SyntheticLedger
				require.NoError(t, child.Account(synth).Main().GetAs(&l))
				pl := l.Partition(src)
				pl.Add(true, pl.Delivered+1, synth.WithTxID([32]byte{9}))
				require.NoError(t, child.Account(synth).Main().Put(l))
				require.NoError(t, child.Commit())
			}
		}) / N
		batch.Discard()

		// What a per-block decide pass does: read once, apply the run, write once.
		batch2 := db.Begin(true)
		require.NoError(t, batch2.Account(synth).Main().GetAs(&warm))
		oncePerBlock = alloc(func() {
			child := batch2.Begin(true)
			var l *protocol.SyntheticLedger
			require.NoError(t, child.Account(synth).Main().GetAs(&l))
			pl := l.Partition(src)
			for i := 0; i < N; i++ {
				pl.Add(true, pl.Delivered+1, synth.WithTxID([32]byte{9}))
			}
			require.NoError(t, child.Account(synth).Main().Put(l))
			require.NoError(t, child.Commit())
		}) / N
		batch2.Discard()

		return perMessage, oncePerBlock
	}

	smallPer, smallOnce := measure(100)
	largePer, largeOnce := measure(16000)

	t.Logf("backlog    100: per-message %6d B/msg   once-per-block %5d B/msg", smallPer, smallOnce)
	t.Logf("backlog  16000: per-message %6d B/msg   once-per-block %5d B/msg", largePer, largeOnce)

	// The per-message pattern pays for the whole backlog on every message, so
	// a 160x backlog costs enormously more per message. A generous floor:
	// asserting the SHAPE, not a number that would drift.
	require.Greater(t, largePer, smallPer*20,
		"per-message ledger access must scale with backlog — that is the defect (#4164)")

	// Reading once per block still shows the backlog — the copy is real, it is
	// just paid ONCE and divided across the run. So its per-message figure
	// falls as the run lengthens, while the per-message pattern's does not
	// fall at all. The property is the RATIO, and the ratio must widen as the
	// backlog grows.
	require.Greater(t, largePer/largeOnce, smallPer/smallOnce,
		"the advantage of reading once per block must grow with backlog")

	require.Less(t, largeOnce*10, largePer,
		fmt.Sprintf("once-per-block must be at least 10x cheaper at a 16,000 backlog (got %d vs %d B/msg)", largeOnce, largePer))
}
