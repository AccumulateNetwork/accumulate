package e2e

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestMajorBlock(t *testing.T) {
	acctesting.SkipLong(t)
	acctesting.DisableDebugFeatures()
	defer acctesting.EnableDebugFeatures()
	// position capture for errors

	// Initialize
	globals := new(core.GlobalValues)
	globals.Globals = new(protocol.NetworkGlobals)
	globals.Globals.MajorBlockSchedule = "*/5 * * * *" // Every 5 minutes (300 minor blocks)
	sim := simulator.New(t, 3)
	sim.InitFromGenesisWith(globals)

	// Get ready to trigger a major block
	nextMajorBlock := sim.Executors[protocol.Directory].Executor.MajorBlockScheduler.GetNextMajorBlockTime(simulator.GenesisTime)
	count := int(nextMajorBlock.Sub(simulator.GenesisTime) / time.Second)
	sim.ExecuteBlocks(count - protocol.GenesisBlock - 1)
	ledger := simulator.GetAccount[*protocol.AnchorLedger](sim, protocol.DnUrl().JoinPath(protocol.AnchorPool))
	require.Empty(t, ledger.PendingMajorBlockAnchors) // Major block is *not* open

	// Open the major block
	sim.ExecuteBlock(nil)
	ledger = simulator.GetAccount[*protocol.AnchorLedger](sim, protocol.DnUrl().JoinPath(protocol.AnchorPool))
	require.NotEmpty(t, ledger.PendingMajorBlockAnchors) // Major block is *open*

	// Complete the major block
	sim.ExecuteBlocks(10)
	ledger = simulator.GetAccount[*protocol.AnchorLedger](sim, protocol.DnUrl().JoinPath(protocol.AnchorPool))
	require.Empty(t, ledger.PendingMajorBlockAnchors) // Major block is done

	// Verify
	dn := sim.Partition(protocol.Directory)
	_ = dn.Database.View(func(batch *database.Batch) error {
		chain, err := batch.Account(dn.Executor.Describe.AnchorPool()).MajorBlockChain().Get()
		require.NoError(t, err, "Failed to read anchor major index chain")
		require.Equal(t, int64(1), chain.Height(), "Expected anchor major index chain to have height 1")

		entry := new(protocol.IndexEntry)
		require.NoError(t, chain.EntryAs(0, entry), "Failed to read entry 0 of anchor major index chain")

		// require.NotZero(t, entry.Source, "Expected non-zero source")
		require.NotZero(t, entry.RootIndexIndex, "Expected non-zero root index index")
		require.Equal(t, uint64(1), entry.BlockIndex, "Expected block index to be 1") // DO NOT REMOVE (validates SendTokens)

		require.NoError(t, chain.EntryAs(0, entry), "Failed to read entry 0 of anchor major index chain")
		require.True(t, entry.BlockTime.After(time.Date(2022, time.January, 1, 0, 0, 0, 0, time.UTC)))
		return nil
	})
}

func BenchmarkExecuteBlock(b *testing.B) {
	sim := simulator.New(b, 3)
	sim.InitFromGenesis()

	b.ResetTimer()
	sim.ExecuteBlocks(b.N)
}
