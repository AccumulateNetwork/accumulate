// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
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

// heartbeatEvery is the most blocks an idle partition goes between anchors:
// anchorHeartbeatSkip (3) skipped, the fourth anchored.
const heartbeatEvery = 4

// requireHeartbeat steps the network n blocks and requires every partition,
// on every node, to have anchored within the last heartbeatEvery blocks after
// each one: "anchors every 4th block when everything is quiet".
func requireHeartbeat(t *testing.T, sim *Sim, nodes, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		sim.Step()
		for _, p := range sim.Partitions() {
			block := sim.S.BlockIndex(p.ID)
			part := sim.S.Partition(p.ID)
			for j := 0; j < nodes; j++ {
				db := part.NodeDatabase(j)
				var ledger *AnchorLedger
				View(t, db, func(batch *database.Batch) {
					require.NoError(t, batch.Account(PartitionUrl(p.ID).JoinPath(AnchorPool)).Main().GetAs(&ledger))
				})
				require.Less(t, block-ledger.LastAnchorBlock, uint64(heartbeatEvery),
					"%s node %d is at block %d and last anchored at block %d", p.ID, j, block, ledger.LastAnchorBlock)
			}
		}
	}
}

// TestHeartbeatStartsFromNothing — #4463. An idle network on which no anchor
// is ever delivered: the initial anchor is dropped, and so is every anchor
// dispatched after it. No block receives a Directory anchor, so a heartbeat
// that fires only on one never fires; each partition must anchor every fourth
// block from its own state alone.
func TestHeartbeatStartsFromNothing(t *testing.T) {
	drop := func(_ context.Context, env *messaging.Envelope) (bool, error) {
		for _, m := range env.Messages {
			if _, ok := m.(*messaging.BlockAnchor); ok {
				return false, nil
			}
		}
		return true, nil
	}
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 1),
		simulator.Genesis(GenesisTime),
		simulator.DropInitialAnchor(),
		simulator.CaptureDispatchedMessages(drop),
	)
	sim.StepN(10)
	requireHeartbeat(t, sim, 1, 40)
}

// TestHeartbeatResumesAfterIdleNetworkRestart — #4463. The Docker network of
// 2026-09-28: an idle network restarts whole, every validator at once, and
// the anchors held in staging are lost with it. With no load after the
// restart, every partition must go on anchoring every fourth block. On
// Docker, before this, the Directory produced 1,745 blocks, 1,739 empty, and
// no partition anchored again. The anchors in flight across the restart are
// dropped, as the restart lost them; those dispatched after it are delivered.
func TestHeartbeatResumesAfterIdleNetworkRestart(t *testing.T) {
	var dropping atomic.Bool
	drop := func(_ context.Context, env *messaging.Envelope) (bool, error) {
		if !dropping.Load() {
			return true, nil
		}
		for _, m := range env.Messages {
			if _, ok := m.(*messaging.BlockAnchor); ok {
				return false, nil
			}
		}
		return true, nil
	}
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		simulator.CaptureDispatchedMessages(drop),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.StepN(20)

	var all []restartTarget
	for _, name := range []string{Directory, "BVN0"} {
		p := sim.S.Partition(name)
		for i := 0; i < 3; i++ {
			all = append(all, restartTarget{name, p, i})
		}
	}
	dropping.Store(true)
	restartAndJoin(t, sim, all)
	dropping.Store(false)
	requireHeartbeat(t, sim, 3, 40)
}
