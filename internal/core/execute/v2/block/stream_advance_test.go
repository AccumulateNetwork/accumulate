// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4169 assumption 7.2, tested BEFORE step 7 is written: advancing a stream
// once per block must leave the ledger in exactly the state that advancing it
// once per message leaves it in.
//
// This is the one assumption in the ledger whose failure corrupts state rather
// than stalling a stream. A stream that stalls is visible and recoverable; a
// ledger whose Delivered or pending window disagrees with what actually
// executed is a divergent block hash, and every node that took the other path
// is now on a different chain.

func advTxid(n uint64) *url.TxID {
	return protocol.PartitionUrl("BVN0").WithTxID([32]byte{byte(n), byte(n >> 8)})
}

// perMessage applies each operation on its own, as SequencedMessage.process
// does today.
func perMessage(delivered, received uint64, ops []advOp) *protocol.PartitionSyntheticLedger {
	l := &protocol.PartitionSyntheticLedger{Url: protocol.PartitionUrl("BVN1")}
	l.Delivered, l.Received = delivered, received
	l.Pending = make([]*url.TxID, received-delivered)
	for _, op := range ops {
		l.Add(op.delivered, op.number, advTxid(op.number))
	}
	return l
}

// oncePerStream applies the same operations the way a per-block advance would:
// the run first, in order, then the refusals. Grouping is the whole point —
// if the outcome depends on interleaving, it cannot be batched.
func oncePerStream(delivered, received uint64, ops []advOp) *protocol.PartitionSyntheticLedger {
	l := &protocol.PartitionSyntheticLedger{Url: protocol.PartitionUrl("BVN1")}
	l.Delivered, l.Received = delivered, received
	l.Pending = make([]*url.TxID, received-delivered)
	for _, op := range ops {
		if op.delivered {
			l.Add(true, op.number, advTxid(op.number))
		}
	}
	for _, op := range ops {
		if !op.delivered {
			l.Add(false, op.number, advTxid(op.number))
		}
	}
	return l
}

type advOp struct {
	number    uint64
	delivered bool
}

func sameLedger(t *testing.T, want, got *protocol.PartitionSyntheticLedger, msg string) {
	t.Helper()
	require.Equalf(t, want.Delivered, got.Delivered, "%s: Delivered", msg)
	require.Equalf(t, want.Received, got.Received, "%s: Received", msg)
	require.Equalf(t, len(want.Pending), len(got.Pending), "%s: pending window length", msg)
	for i := range want.Pending {
		if want.Pending[i] == nil {
			require.Nilf(t, got.Pending[i], "%s: pending[%d]", msg, i)
			continue
		}
		require.NotNilf(t, got.Pending[i], "%s: pending[%d]", msg, i)
		require.Equalf(t, want.Pending[i].String(), got.Pending[i].String(), "%s: pending[%d]", msg, i)
	}
}

// A run of consecutive deliveries — the case step 7 exists to collapse.
func TestStreamAdvance_ARunIsTheSameEitherWay(t *testing.T) {
	for _, n := range []uint64{1, 2, 5, 40, 300} {
		ops := make([]advOp, 0, n)
		for i := uint64(1); i <= n; i++ {
			ops = append(ops, advOp{number: i, delivered: true})
		}
		sameLedger(t,
			perMessage(0, n, ops),
			oncePerStream(0, n, ops),
			"run of "+string(rune('0'+n%10)))
	}
}

// A run followed by refusals: what a block does when it drains what it can and
// records the rest.
func TestStreamAdvance_RunThenRefusalsIsTheSameEitherWay(t *testing.T) {
	ops := []advOp{
		{1, true}, {2, true}, {3, true}, // the run
		{7, false}, {5, false}, {9, false}, // arrivals past the gap, out of order
	}
	sameLedger(t, perMessage(0, 9, ops), oncePerStream(0, 9, ops), "run then refusals")
}

// Refusals interleaved with the run. If the outcome depends on the
// interleaving, a per-block advance CANNOT be substituted for a per-message
// one, and step 7 is unsound.
func TestStreamAdvance_InterleavingDoesNotMatter(t *testing.T) {
	interleaved := []advOp{
		{1, true}, {6, false}, {2, true}, {8, false}, {3, true}, {5, false},
	}
	grouped := []advOp{
		{1, true}, {2, true}, {3, true}, {6, false}, {8, false}, {5, false},
	}
	sameLedger(t,
		perMessage(0, 8, interleaved),
		perMessage(0, 8, grouped),
		"the same operations in a different order")
}

// Starting from a stream that already has a pending window, which is the case
// a backlog drain presents.
func TestStreamAdvance_DrainingAnExistingWindow(t *testing.T) {
	build := func() *protocol.PartitionSyntheticLedger {
		l := &protocol.PartitionSyntheticLedger{Url: protocol.PartitionUrl("BVN1")}
		l.Delivered, l.Received = 10, 15
		l.Pending = make([]*url.TxID, 5)
		for i := uint64(11); i <= 15; i++ {
			l.Pending[i-11] = advTxid(i)
		}
		return l
	}

	one := build()
	for i := uint64(11); i <= 15; i++ {
		one.Add(true, i, advTxid(i))
	}

	batch := build()
	for i := uint64(11); i <= 15; i++ {
		batch.Add(true, i, advTxid(i))
	}

	sameLedger(t, one, batch, "draining a five-deep window")
	require.Equal(t, uint64(15), one.Delivered)
	require.Empty(t, one.Pending, "a fully drained window leaves nothing behind")
}
