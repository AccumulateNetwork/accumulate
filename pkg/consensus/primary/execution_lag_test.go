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
// header carries no batches and the batches stay available for a later one;
// the workers refuse user work meanwhile, and both clear when execution
// catches up.
func TestCreateHeader_CarriesNoBatchesWhileExecutionLags(t *testing.T) {
	v := newTestValidator(t)
	committee := newTestCommittee([]*testValidator{v}, 1)
	d := newTestDAG()
	w := worker.New(worker.Config{ID: 1, Partition: "test", MaxStoredBatchBytes: 1 << 20}, nil)
	batch := types.NewBatch([][]byte{{1, 2, 3}})
	require.NoError(t, w.StoreBatch(batch))
	digest := batch.Digest()
	w.RequeueBatches([]types.BatchDigest{digest})

	lag := 0
	p := New(Config{Partition: "test", KeyPair: v.priv}, committee, nil, d, []*worker.Worker{w})
	p.SetExecutionLagSource(func() int { return lag }, 4)

	// Within the bound: the batch is proposed
	lag = 4
	header, err := p.CreateHeader()
	require.NoError(t, err)
	require.Len(t, header.Payload, 1, "within the bound, the header takes the batch")
	require.NoError(t, w.SubmitUser([]byte{9}))

	// Past the bound: no batches, the batch stays available, user work refused
	w.RequeueBatches([]types.BatchDigest{digest})
	lag = 5
	header, err = p.CreateHeader()
	require.NoError(t, err)
	require.Empty(t, header.Payload, "past the bound, the header carries no batches")
	require.Len(t, w.AvailableBatches(), 1, "the batch waits for a later header")
	require.ErrorIs(t, w.SubmitUser([]byte{9}), worker.ErrExecutionLagging)

	// Caught up: proposing again, accepting again (listing the queue above
	// drained it, so the batch is requeued as the worker would)
	w.RequeueBatches([]types.BatchDigest{digest})
	lag = 0
	header, err = p.CreateHeader()
	require.NoError(t, err)
	require.Len(t, header.Payload, 1)
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
