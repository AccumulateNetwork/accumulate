// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

// THE PULL CANNOT MOVE THE DAEMON'S OWN BLOCK, AND THE NODE THAT ESCAPES A
// WHOLE-NETWORK RESTART SERVES AGAIN (#4295, test audit gap 5).
//
// Two facts about the same number, and both were untested.
//
// `<partition>/ledger` is an ACCOUNT, and it is one of the accounts the pull
// overwrites with the peer's. A join that re-read its own block from the
// store after a round would read the PEER's. Measured on the live twelve-node
// network of 2026-09-18: the joining node's store answered 929 and the peer
// 946 while its executor stood at 76, so the join believed it was 17 blocks
// behind when it was 853 behind. `PulledState` therefore remembers the number
// from before the pull started, and `Executing` records THAT.
//
// And `Executing` is the whole-network-restart escape: no peer had staging to
// give, so the node starts from its own state, and its services must answer
// again. Leaving it BOOTING would make a running node refuse every request
// for the rest of its life — which is what "every read" costs if the exit is
// not wired. The gauge must follow, because the gauge is the object the node
// answers by (#4295).
//
// Production wiring: `join.NewState` and `join.QueryPeers` — FindService, one
// peer addressed by ID — against the simulator's real peers, and the real
// `Executing`. By hand: the node's own executed block is asserted rather than
// read out of a running executor, because the joining node here has none.
func TestThePullCannotMoveTheDaemonsOwnBlock(t *testing.T) {
	const ownBlock = 4 // where this node's executor stopped, before the pull

	sim := spineNetwork(t, 3)
	ctx := context.Background()
	part := PartitionUrl("BVN0")

	local := genesisOf(t, sim, "BVN0")
	state, err := join.NewState(join.StateOptions{
		Partition:     part,
		Database:      local,
		Sources:       namedPeers(t, sim),
		ExecutedBlock: ownBlock,
	})
	require.NoError(t, err)

	// It starts BOOTING, and the gauge says so.
	require.False(t, state.Machine().CanServeCurrent())
	require.Equal(t, float64(0), nodeStateGauge(t, "bvn0"))

	// Pull until the peer's ledger has landed in this node's store. It is one
	// of the accounts the pull overwrites, which is the whole point.
	var stored uint64
	for round := 0; round < 20 && stored == 0; round++ {
		require.NoError(t, state.Pull(ctx), "pull round %d", round)
		sim.StepN(3)
		View(t, local, func(batch *database.Batch) {
			var ledger *SystemLedger
			if err := batch.Account(part.JoinPath(Ledger)).Main().GetAs(&ledger); err == nil {
				stored = ledger.Index
			}
		})
	}
	require.NotZero(t, stored, "the pull never fetched the partition ledger")
	require.Greater(t, stored, uint64(ownBlock),
		"the pull did not overwrite this node's ledger, so the test proves nothing about whose block it is")

	// The escape: no peer had staging, so the node executes from its own
	// state — at ITS block, not the one now in its store.
	require.NoError(t, state.Executing(ownBlock))
	require.Equal(t, uint64(ownBlock), state.Machine().Get().SinceBlock,
		"the node recorded the PEER's block as its own: the pull moved the number the join stands on")

	// And it answers again, on the gauge and on the gate, which are one
	// object.
	require.Equal(t, nodestate.StateActive, state.Machine().State())
	require.True(t, state.Machine().CanServeCurrent(),
		"a node that escaped a whole-network restart still refuses every read")
	require.Equal(t, float64(2), nodeStateGauge(t, "bvn0"))
	require.Equal(t, nodestate.Number(nodestate.StateOf(state.Machine())), nodeStateGauge(t, "bvn0"))
}
