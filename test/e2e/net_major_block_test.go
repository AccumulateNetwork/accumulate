// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestMajorBlock(t *testing.T) {
	acctesting.DisableDebugFeatures()
	defer acctesting.EnableDebugFeatures()
	// position capture for errors

	// Initialize
	g := new(core.GlobalValues)
	g.Globals = new(NetworkGlobals)
	g.Globals.MajorBlockSchedule = "*/5 * * * *" // Every 5 minutes (300 minor blocks)
	g.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 3),
		simulator.GenesisWith(GenesisTime, g),
	)

	// Get ready to trigger a major block
	nextMajorBlock := g.MajorBlockSchedule().Next(GenesisTime)
	count := int(nextMajorBlock.Sub(GenesisTime) / time.Second)
	sim.StepN(count - GenesisBlock - 1)
	ledger := GetAccount[*AnchorLedger](t, sim.Database(Directory), DnUrl().JoinPath(AnchorPool))
	require.Empty(t, ledger.PendingMajorBlockAnchors) // Major block is *not* open

	// Open the major block
	sim.Step()
	ledger = GetAccount[*AnchorLedger](t, sim.Database(Directory), DnUrl().JoinPath(AnchorPool))
	require.NotEmpty(t, ledger.PendingMajorBlockAnchors) // Major block is *open*

	// Complete the major block
	sim.StepN(10)
	ledger = GetAccount[*AnchorLedger](t, sim.Database(Directory), DnUrl().JoinPath(AnchorPool))
	require.Empty(t, ledger.PendingMajorBlockAnchors) // Major block is done

	// Verify
	_ = sim.Database(Directory).View(func(batch *database.Batch) error {
		chain, err := batch.Account(DnUrl().JoinPath(AnchorPool)).MajorBlockChain().Get()
		require.NoError(t, err, "Failed to read anchor major index chain")
		require.Equal(t, int64(1), chain.Height(), "Expected anchor major index chain to have height 1")

		entry := new(IndexEntry)
		require.NoError(t, chain.EntryAs(0, entry), "Failed to read entry 0 of anchor major index chain")

		// require.NotZero(t, entry.Source, "Expected non-zero source")
		require.NotZero(t, entry.RootIndexIndex, "Expected non-zero root index index")
		require.Equal(t, uint64(1), entry.BlockIndex, "Expected block index to be 1") // DO NOT REMOVE (validates SendTokens)

		require.NoError(t, chain.EntryAs(0, entry), "Failed to read entry 0 of anchor major index chain")
		require.True(t, entry.BlockTime.After(time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC)))
		return nil
	})
}
