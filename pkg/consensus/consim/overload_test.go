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

// The soak's death, in five minutes. Two BVNs of four validators and the
// Directory at the soak's pacing; BVN1 offered 400 user tx/s and BVN2 250,
// each executor good for 400 tx/s; and the coupling the network has: every
// accepted user transaction produces 1.5 synthetics for the other BVN, a
// Directory round trip later, through the path nothing refuses. BVN1 keeps
// accepting users because its own lag is fine, so BVN2 receives more
// synthetics than it can execute and cannot refuse them; refusing its own
// users changes nothing. Without a header cap the backlog comes back as one
// block that doubles every cycle -- 512, 995, 1,329, 2,093, 3,261, 5,012
// transactions, the last twelve seconds of execution -- and BVN2 stops
// (soak 20260905T144928Z, forty-five minutes to the same place). With the cap
// the blocks stay bounded and BVN2 keeps producing, but its lag still drifts
// upward: the inflow exceeds its capacity, and only cross-partition
// back-pressure can change that (DIFFERENCES C7).
func asymmetricOverload(t *testing.T, maxHeaderBytes int) (*Result, error) {
	t.Helper()
	sim, err := New(Config{
		BVNs:             2,
		ValidatorsPerBVN: 4,
		NumWorkers:       4,
		TPS:              2,
		TPSByPartition:   map[string]int{"BVN1": 400, "BVN2": 250},
		UserLoad:         true,
		SyntheticPerUser: 1.5,
		ExecCostPerTx:    2500 * time.Microsecond,
		MinRoundInterval: 500 * time.Millisecond,
		BatchTimeout:     100 * time.Millisecond,
		BatchSize:        50,
		MaxExecutionLag:  8,
		MaxHeaderBytes:   maxHeaderBytes,
		Duration:         300 * time.Second,
		StallAfter:       60 * time.Second,
		Out:              os.Stdout,
	})
	require.NoError(t, err)
	defer sim.Close()
	r, err := sim.Run(context.Background())
	t.Logf("ok=%v reason=%q heights=%v maxBlockTxs=%v maxLag=%v refused=%d/%d",
		r.Ok, r.Reason, r.Heights, r.MaxBlockTxs, r.MaxLag, r.Refused, r.Submitted)
	return r, err
}

func TestOverload_UncappedHeadersDoubleTheDumpUntilThePartitionStops(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the consensus stack for five minutes")
	}
	r, _ := asymmetricOverload(t, 1<<30)
	require.Greater(t, r.MaxBlockTxs["BVN2"], uint64(2000), "the dumped blocks reach thousands of transactions")
	require.Greater(t, r.MaxLag["BVN2"], 30, "and the lag runs far past the bound")
}

func TestOverload_CappedHeadersKeepThePartitionMoving(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the consensus stack for five minutes")
	}
	txs := make([][]byte, 50)
	for i := range txs {
		txs[i] = []byte(fmt.Sprintf("consim-synth-BVN2-%d", 100000+i))
	}
	oneBatch := types.NewBatch(txs).Size()
	r, err := asymmetricOverload(t, 2*oneBatch+oneBatch/2)
	require.NoError(t, err, "with the cap no partition is declared stalled")
	require.LessOrEqual(t, r.MaxBlockTxs["BVN2"], uint64(4*2*50+50), "a block is bounded by validators x MaxHeaderBytes")
	require.Greater(t, r.Heights["BVN2"], uint64(200), "and BVN2 keeps producing blocks under the same load")
}
