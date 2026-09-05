// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consim

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// A partition whose executor cannot keep up with its offered load. The
// execution-lag bound empties headers and refuses user work; when the lag
// clears, the backlog must come back a header at a time (consensus spec,
// invariant 9). Soak 20260905T144928Z: without a cap the backlog came back as
// one block of ten to seventeen seconds, which re-crossed the bound, and both
// BVNs oscillated between refusing and stalling with load accepted at 392 of
// 500 tps. This runs the same shape in-process, in under a minute, with the
// cap off and on.
func overloaded(t *testing.T, maxHeaderBytes int, seconds time.Duration) *Result {
	t.Helper()
	sim, err := New(Config{
		BVNs:             1,
		ValidatorsPerBVN: 4,
		NumWorkers:       1,
		TPS:              5,
		TPSByPartition:   map[string]int{"BVN1": 800},
		UserLoad:         true,
		// Cross-partition traffic is never refused and keeps arriving while
		// user work is: it is what piles up in the store during a refusal
		// window and what one header drains afterwards.
		SystemTPSByPartition: map[string]int{"BVN1": 250},
		// 2.5 ms per transaction: BVN1 executes 400 tx/s of the ~1,000
		// offered. Rounds every 100 ms, a block ~250 ms, the bound of 8
		// blocks ~2 s: a refusal window lets ~500 system transactions pile
		// up on top of the user backlog, seconds of execution if it comes
		// back as one block -- the soak's proportions (8 s of 500 tps
		// against a ~400 tps executor).
		ExecCostByPartition: map[string]time.Duration{"BVN1": 2500 * time.Microsecond},
		MinRoundInterval:    100 * time.Millisecond,
		BatchTimeout:        50 * time.Millisecond,
		BatchSize:           50,
		MaxExecutionLag:     8,
		MaxHeaderBytes:      maxHeaderBytes,
		Duration:            seconds,
		StallAfter:          20 * time.Second,
		Out:                 os.Stdout,
	})
	require.NoError(t, err)
	defer sim.Close()
	r, err := sim.Run(context.Background())
	require.NoError(t, err)
	t.Logf("heights=%v maxBlockTxs=%v maxLag=%v submitted=%d refused=%d",
		r.Heights, r.MaxBlockTxs, r.MaxLag, r.Submitted, r.Refused)
	return r
}

func TestOverload_BacklogComesBackAHeaderAtATime(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the consensus stack twice for ~25s each")
	}
	// One batch of fifty transactions the size the load submits: about one
	// block interval of BVN1's execution capacity.
	txs := make([][]byte, 50)
	for i := range txs {
		txs[i] = []byte(fmt.Sprintf("consim-BVN1-%d", 100000+i))
	}
	oneBatch := types.NewBatch(txs).Size()

	// Cap off: the header takes everything that piled up while it was empty.
	uncapped := overloaded(t, 1<<30, 30*time.Second)
	// Cap on: two batches per header, ~one block interval of capacity each.
	capped := overloaded(t, 2*oneBatch+oneBatch/2, 30*time.Second)

	require.Greater(t, uncapped.Refused, uint64(0), "the bound engaged: user work was refused")
	require.Greater(t, uncapped.MaxLag["BVN1"], 8, "and the lag crossed it")

	// With the cap, one block is at most what four validators' headers may
	// carry in one committed group, two batches each: its execution time is
	// bounded, where the uncapped block carries whatever piled up. The lag
	// itself stays high in both runs -- the partition is offered more than it
	// can execute for the whole run, and only the user half can be refused --
	// so the block size, which is the block's execution time, is the measure.
	require.LessOrEqual(t, capped.MaxBlockTxs["BVN1"], uint64(4*2*50+50),
		"with the cap a block is bounded by validators x MaxHeaderBytes")
	require.Greater(t, uncapped.MaxBlockTxs["BVN1"], capped.MaxBlockTxs["BVN1"]*3/2,
		"without the cap the backlog comes back in one block")
	require.Greater(t, capped.Heights["BVN1"], uint64(0))
}
