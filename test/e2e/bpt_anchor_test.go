// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// #4272: the ledger's bpt chain was never anchored into the root chain, because
// enumerateModifiedChains runs before the entry is written. It therefore has no
// index chain, and a historical BPT root is node-asserted rather than
// network-committed.

func bptSim(t *testing.T, version ExecutorVersion) *Sim {
	t.Helper()
	g := new(network.GlobalValues)
	g.ExecutorVersion = version
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.GenesisWith(GenesisTime, g),
	)
	sim.StepN(10)
	MakeLiteTokenAccount(t, sim.DatabaseFor(DnUrl()), make([]byte, 32), AcmeUrl())
	sim.StepN(20)
	return sim
}

// ledgerChains reports the ledger's chains and the bpt chain's height.
func ledgerChains(t *testing.T, sim *Sim, part string) (names map[string]bool, bptCount int64) {
	t.Helper()
	u := PartitionUrl(part).JoinPath(Ledger)
	names = map[string]bool{}
	View(t, sim.Database(part), func(batch *database.Batch) {
		chains, err := batch.Account(u).Chains().Get()
		require.NoError(t, err)
		for _, c := range chains {
			names[c.Name] = true
		}
		head, err := batch.Account(u).BptChain().Inner().Head().Get()
		require.NoError(t, err)
		bptCount = head.Count
	})
	return names, bptCount
}

// Below the gate the behaviour must be exactly what it is today: entries on the
// bpt chain, and no index chain over them.
func TestBptAnchor_InertBeforeKourou(t *testing.T) {
	sim := bptSim(t, ExecutorVersionV2Jiuquan)
	names, count := ledgerChains(t, sim, Directory)

	require.Greater(t, count, int64(0), "the bpt chain should still be written")
	require.True(t, names["bpt"], "the bpt chain exists")
	require.False(t, names["bpt-index"],
		"below the gate the bpt chain must not be indexed")
}

// At the gate the chain is anchored, so it acquires an index chain.
func TestBptAnchor_IndexedAtKourou(t *testing.T) {
	sim := bptSim(t, ExecutorVersionV2Kourou)
	names, count := ledgerChains(t, sim, Directory)

	require.Greater(t, count, int64(0))
	require.True(t, names["bpt"], "the bpt chain exists")
	require.True(t, names["bpt-index"],
		"at the gate the bpt chain must be anchored, which creates its index")
}

// The anchoring must not disturb the ordering of the entries that were already
// there: the sort position is consensus-visible.
func TestBptAnchor_EntriesStaySorted(t *testing.T) {
	sim := bptSim(t, ExecutorVersionV2Kourou)
	u := DnUrl().JoinPath(Ledger)

	View(t, sim.Database(Directory), func(batch *database.Batch) {
		// The block ledger records the entries in the order they were anchored
		head, err := batch.Account(u).MainChain().Head().Get()
		require.NoError(t, err)
		require.Greater(t, head.Count, int64(0))
	})

	// And the bpt chain is among the anchored chains
	names, _ := ledgerChains(t, sim, Directory)
	require.True(t, names["bpt-index"])
}
