package consensus

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/worker"
)

// TestPerWorkerBytesIsAPartitionBudget pins that the batch-store budget is
// divided among workers rather than applied to each: raising num-workers is a
// parallelism decision, not a memory decision.
func TestPerWorkerBytesIsAPartitionBudget(t *testing.T) {
	const def = worker.DefaultMaxStoredBatchBytes
	const maxBatch = worker.DefaultMaxBatchBytes

	// One worker gets the whole budget.
	require.Equal(t, def, perWorkerBytes(0, def, 1, maxBatch))

	// The total across workers never exceeds the budget, until the floor
	// takes over.
	for _, n := range []int{1, 2, 4, 8, 16} {
		share := perWorkerBytes(0, def, n, maxBatch)
		if share > 2*maxBatch {
			require.LessOrEqual(t, share*n, def,
				"total across %d workers must stay within the partition budget", n)
		}
	}

	// The floor holds: a worker must always be able to hold two batches.
	require.GreaterOrEqual(t, perWorkerBytes(0, def, 1024, maxBatch), 2*maxBatch)

	// An explicit budget wins over the default, and is still divided.
	require.Equal(t, (64<<20)/4, perWorkerBytes(64<<20, def, 4, maxBatch))

	// Degenerate inputs do not produce a zero-size store.
	require.Positive(t, perWorkerBytes(0, def, 0, 0))
}
