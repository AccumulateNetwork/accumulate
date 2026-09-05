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
