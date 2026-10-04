// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// A BVN lagging consensus, whose head Directory anchor is below quorum
// because the copies that would complete it were lost, recovers: it asks for
// the anchor whatever its lag, and the Directory's answer carries the quorum
// (healing spec, "An anchor stream is asked for whatever the lag"; #4517).
//
// Run 20261003T232435Z: BVN2's Directory anchor stream stood at 1,980 for
// eighteen minutes. Its executor lagged because the anchor was below quorum,
// every heal toward the Directory was skipped because it lagged, and an
// anchor is dispatched once, so nothing else would ever bring the missing
// signatures.
//
// Here the first copy of Directory anchor 3 reaches BVN0, so it is held below
// quorum, and every other copy is dropped while the drop window lasts. Every
// conductor is told its executor lags past the bound for the whole test.
func TestALaggingBVNHealsItsDirectoryAnchorStream(t *testing.T) {
	const stuck = 3
	bvn := PartitionUrl("BVN0")

	var dropping atomic.Bool
	dropping.Store(true)
	var dropped atomic.Int32
	var mu sync.Mutex
	passed := false

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		// Past MaxExecutionLag (8) on every node, for the whole test.
		simulator.WithExecutionLag(func() int { return 9 }),
		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (bool, error) {
			if !dropping.Load() {
				return true, nil
			}
			messages, err := env.Normalize()
			if err != nil {
				return false, err
			}
			for _, msg := range messages {
				anchor, ok := msg.(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				seq, ok := anchor.Anchor.(*messaging.SequencedMessage)
				if !ok || seq.Number != stuck || !seq.Destination.Equal(bvn) {
					continue
				}
				txn, ok := seq.Message.(*messaging.TransactionMessage)
				if !ok || txn.Transaction.Body.Type() != TransactionTypeDirectoryAnchor {
					continue
				}
				mu.Lock()
				first := !passed
				passed = true
				mu.Unlock()
				if !first {
					dropped.Add(1)
					return false, nil
				}
			}
			return true, nil
		}),
	)

	delivered := func() uint64 {
		var ledger *AnchorLedger
		View(t, sim.DatabaseFor(bvn.JoinPath(AnchorPool)), func(batch *database.Batch) {
			require.NoError(t, batch.Account(bvn.JoinPath(AnchorPool)).Main().GetAs(&ledger))
		})
		return ledger.Partition(DnUrl()).Delivered
	}

	// The copies of anchor 3 are lost, then the window closes. Nothing
	// dispatches an anchor twice.
	sim.StepUntil(True(func(*Harness) bool { return dropped.Load() >= 2 }))
	sim.StepN(20)
	dropping.Store(false)
	require.Equal(t, uint64(stuck-1), delivered(), "precondition: BVN0's Directory anchor stream stands below the lost anchor")

	sim.StepUntilN(400, True(func(*Harness) bool { return delivered() > stuck }))
	require.Greater(t, delivered(), uint64(stuck), "the lagging BVN asked for the anchor and the answer's quorum validated it")
}
