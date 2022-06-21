package e2e

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestMajorBlock(t *testing.T) {
	acctesting.SkipLong(t)
	// TODO Make it possible to disable debug features, specifically call
	// position capture for errors

	// Initialize
	sim := simulator.New(t, 3)
	sim.InitFromGenesis()

	// Trigger a major block
	count := int(12 * time.Hour / time.Second)
	if os.Getenv("CI") != "" {
		sim.ExecuteBlocks(count)

	} else {
		// Give the user an indication that something is happening
		count -= 1
		for count > 0 {
			n := count
			if n > 5000 {
				n = 5000
			}
			sim.ExecuteBlocks(n)
			fmt.Printf(".\n")
			count -= n
		}
		sim.ExecuteBlock(nil)
	}

	// Complete the major block
	sim.ExecuteBlocks(10)

	// Verify
	dn := sim.Partition(protocol.Directory)
	_ = dn.Database.View(func(batch *database.Batch) error {
		chain, err := batch.Account(dn.Executor.Describe.AnchorPool()).ReadIndexChain(protocol.MainChain, true)
		require.NoError(t, err, "Failed to read anchor major index chain")
		require.Equal(t, int64(1), chain.Height(), "Expected anchor major index chain to have height 1")

		entry := new(protocol.IndexEntry)
		require.NoError(t, chain.EntryAs(0, entry), "Failed to read entry 0 of anchor major index chain")

		// require.NotZero(t, entry.Source, "Expected non-zero source")
		require.NotZero(t, entry.RootIndexIndex, "Expected non-zero root index index")
		require.Equal(t, uint64(1), entry.BlockIndex, "Expected block index to be 1")
		return nil
	})
}

func BenchmarkExecuteBlock(b *testing.B) {
	sim := simulator.New(b, 3)
	sim.InitFromGenesis()

	b.ResetTimer()
	sim.ExecuteBlocks(b.N)
}
