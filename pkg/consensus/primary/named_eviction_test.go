// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package primary

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// A batch this node voted for survives a flood of batches no header names,
// through the real path: the vote gate in OnHeaderReceived marks it on the
// worker, and the worker's eviction loop takes unmarked batches first
// (consensus spec, invariant 3; #4598). The worker tests prove the ordering;
// this proves the primary asks for it, which nothing else would catch if the
// call were dropped.
//
// The gate's check and mark are one call (NameBatches) under the worker's
// lock, so there is no window between them for eviction to take a batch the
// vote is about to certify. That is structural, not timed: this test and
// TestNameBatches_ReportsMissingAndMarksActive cover the one call, and no
// test races eviction between a check and a mark because there is no
// between.
func TestNamed_VotedBatchSurvivesFloodThroughVoteGate(t *testing.T) {
	validators := []*testValidator{newTestValidator(t), newTestValidator(t)}
	committee := newTestCommittee(validators, 1)
	d := newTestDAG()
	createGenesisCertificates(t, validators, committee, d)

	w := worker.New(worker.Config{
		ID:                  0,
		Partition:           "test",
		MaxBatchBytes:       1 << 10,
		MaxStoredBatchBytes: 64 << 10,
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = w.Start(ctx) }() // the eviction loop; blocks until ctx ends

	p := New(Config{Partition: "test", KeyPair: validators[0].priv, NewCertsChannelSize: 10},
		committee, nil, d, []*worker.Worker{w})

	voted := types.NewBatch([][]byte{[]byte("a batch this node votes for")})
	require.NoError(t, w.StoreBatch(voted))

	// No parents, so the parent gate passes and the batch gate is what the
	// header meets; epoch 1 is where the primary starts.
	header := types.NewHeader(validators[1].pub, 0, 1,
		[]types.PayloadEntry{{Digest: voted.Digest(), Worker: 0}}, nil)
	require.NoError(t, header.Sign(validators[1].priv))
	p.OnHeaderReceived(header)
	require.True(t, votedOn(p, header), "the batch is held, so the vote goes out")

	// A peer sends batches nobody names, three times the budget's worth.
	for i := 0; i < 200; i++ {
		b := types.NewBatch([][]byte{[]byte(fmt.Sprintf("unnamed-%04d-%01000d", i, 0))})
		require.NoError(t, w.StoreBatch(b))
	}
	require.Eventually(t, func() bool { return w.BatchCount() <= 64 }, 5*time.Second, 10*time.Millisecond,
		"the store evicts down to its budget")

	require.True(t, w.HasBatch(voted.Digest()), "the flood took the batch this node voted for")
}
