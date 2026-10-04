// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// Consensus does not outrun execution (consensus spec, invariant 9): while the
// executor is more than MaxExecutionLag blocks behind the DAG's commits, a
// header carries no user work and those batches stay available for a later
// one; the workers refuse user work meanwhile, and both clear when execution
// catches up.
//
// System traffic is still proposed (#4517). A lagging executor is often
// waiting on it: a Directory anchor below quorum is completed only by copies
// that are ordered. Run 20261003T232435Z: BVN2 lagged 13 to 75 blocks while
// every Directory validator's copy of each anchor after 1,980 sat in its
// workers, and no anchor after 1,980 ever collected a quorum.
func TestCreateHeader_CarriesNoBatchesWhileExecutionLags(t *testing.T) {
	v := newTestValidator(t)
	committee := newTestCommittee([]*testValidator{v}, 1)
	d := newTestDAG()

	w := worker.New(worker.Config{ID: 1, Partition: "test", MaxStoredBatchBytes: 1 << 20}, nil)
	carried := func(h *types.Header) map[types.BatchDigest]bool {
		out := map[types.BatchDigest]bool{}
		for _, e := range h.Payload {
			out[e.Digest] = true
		}
		return out
	}
	only := func(t *testing.T) types.BatchDigest {
		t.Helper()
		a := w.AvailableBatches()
		require.Len(t, a, 1)
		w.RequeueBatches(a)
		return a[0]
	}

	// One batch of user work, one of system traffic (an anchor copy, say).
	require.NoError(t, w.SubmitUser([]byte{1, 2, 3}))
	w.Seal()
	user := only(t)
	require.NoError(t, w.Submit([]byte{4, 5, 6}))
	w.Seal()
	all := w.AvailableBatches()
	require.Len(t, all, 2)
	w.RequeueBatches(all)
	var system types.BatchDigest
	for _, dg := range all {
		if dg != user {
			system = dg
		}
	}

	lag := 0
	p := New(Config{Partition: "test", KeyPair: v.priv}, committee, nil, d, []*worker.Worker{w})
	p.SetExecutionLagSource(func() int { return lag }, 4)

	// Past the bound: the system batch is proposed, the user batch waits,
	// and user work is refused.
	lag = 5
	header, err := p.CreateHeader()
	require.NoError(t, err)
	got := carried(header)
	require.True(t, got[system], "past the bound, the header still carries system traffic")
	require.False(t, got[user], "past the bound, the header carries no user work")
	require.Len(t, w.AvailableBatches(), 1, "the user batch waits for a later header")
	w.RequeueBatches([]types.BatchDigest{user})
	require.ErrorIs(t, w.SubmitUser([]byte{9}), worker.ErrExecutionLagging)

	// Caught up: the user batch is proposed, and user work is accepted.
	lag = 0
	header, err = p.CreateHeader()
	require.NoError(t, err)
	require.True(t, carried(header)[user], "caught up, the user batch is proposed")
	require.NoError(t, w.SubmitUser([]byte{9}))
}

// After a refusal window the backlog comes back a header at a time: a header
// carries at most MaxHeaderBytes of batches, always at least one, and the rest
// wait for the next header in order.
func TestCreateHeader_MetersTheBacklog(t *testing.T) {
	v := newTestValidator(t)
	committee := newTestCommittee([]*testValidator{v}, 1)
	d := newTestDAG()
	w := worker.New(worker.Config{ID: 1, Partition: "test", MaxStoredBatchBytes: 1 << 24}, nil)
	var digests []types.BatchDigest
	for i := 0; i < 5; i++ {
		b := types.NewBatch([][]byte{make([]byte, 300)})
		b.Transactions[0][0] = byte(i)
		require.NoError(t, w.StoreBatch(b))
		digests = append(digests, b.Digest())
	}
	w.RequeueBatches(digests)
	one := types.NewBatch([][]byte{make([]byte, 300)}).Size()

	p := New(Config{Partition: "test", KeyPair: v.priv, MaxHeaderBytes: 2*one + one/2}, committee, nil, d, []*worker.Worker{w})
	header, err := p.CreateHeader()
	require.NoError(t, err)
	carried := func(h *types.Header) []types.BatchDigest {
		var out []types.BatchDigest
		for _, e := range h.Payload {
			out = append(out, e.Digest)
		}
		return out
	}
	// A header sorts its payload into canonical order, so compare as sets.
	require.ElementsMatch(t, digests[0:2], carried(header), "the first two fit the budget")

	header, err = p.CreateHeader()
	require.NoError(t, err)
	require.ElementsMatch(t, digests[2:4], carried(header), "the next two")

	header, err = p.CreateHeader()
	require.NoError(t, err)
	require.ElementsMatch(t, digests[4:5], carried(header), "the last one")
	require.Empty(t, w.AvailableBatches())

	// A budget smaller than one batch still moves one batch per header.
	w.RequeueBatches(digests[:1])
	p2 := New(Config{Partition: "test", KeyPair: v.priv, MaxHeaderBytes: 10}, committee, nil, d, []*worker.Worker{w})
	header, err = p2.CreateHeader()
	require.NoError(t, err)
	require.Len(t, header.Payload, 1)
}

// The byte bound is per block, not per header: a block executes the headers
// of every validator over two rounds, so each header gets one share of
// MaxBlockBytes, and eight validators cannot each fill a header to
// MaxHeaderBytes (#4230). A single validator's share is the whole block,
// still capped by MaxHeaderBytes.
func TestCreateHeader_BudgetIsPerBlock(t *testing.T) {
	one := types.NewBatch([][]byte{make([]byte, 300)}).Size()
	require.Equal(t, one, headerBudget(100*one, 16*one, 8), "eight validators, two rounds: a sixteenth of the block")
	require.Equal(t, 8*one, headerBudget(100*one, 16*one, 1), "one validator: half the block, two rounds")
	require.Equal(t, 3*one, headerBudget(3*one, 16*one, 1), "and never more than the header cap")
	require.Equal(t, 1, headerBudget(3*one, 1, 8), "a share smaller than a byte still moves one batch")

	// Through the header builder: eight validators, a block of sixteen
	// batches, so one batch per header where the header cap alone allowed
	// all five.
	var validators []*testValidator
	for i := 0; i < 8; i++ {
		validators = append(validators, newTestValidator(t))
	}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	w := worker.New(worker.Config{ID: 1, Partition: "test", MaxStoredBatchBytes: 1 << 24}, nil)
	var digests []types.BatchDigest
	for i := 0; i < 5; i++ {
		b := types.NewBatch([][]byte{make([]byte, 300)})
		b.Transactions[0][0] = byte(i)
		require.NoError(t, w.StoreBatch(b))
		digests = append(digests, b.Digest())
	}
	w.RequeueBatches(digests)

	p := New(Config{Partition: "test", KeyPair: validators[0].priv, MaxHeaderBytes: 100 * one, MaxBlockBytes: 16 * one}, committee, nil, d, []*worker.Worker{w})
	for i := 0; i < 5; i++ {
		header, err := p.CreateHeader()
		require.NoError(t, err)
		require.Len(t, header.Payload, 1, "header %d carries one share of the block", i)
	}
	require.Empty(t, w.AvailableBatches())
}
