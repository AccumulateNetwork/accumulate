package database_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func delivered(status *protocol.TransactionStatus) bool {
	return status.Delivered
}

func TestState(t *testing.T) {
	// Create some state
	sim := simulator.New(t, 1)
	sim.InitFromGenesis()
	alice := acctesting.GenerateTmKey(t.Name(), "Alice")
	aliceUrl := acctesting.AcmeLiteAddressTmPriv(alice)
	faucet := protocol.Faucet.Signer()
	env := acctesting.NewTransaction().
		WithPrincipal(protocol.FaucetUrl).
		WithTimestamp(faucet.Timestamp()).
		WithBody(&protocol.AcmeFaucet{Url: aliceUrl}).
		Faucet()
	sim.MustSubmitAndExecuteBlock(env)
	sim.WaitForTransactionFlow(delivered, env.Transaction[0].GetHash())

	sim.ExecuteBlocks(10)

	// Save to a file
	filename := filepath.Join(t.TempDir(), "state.bpt")
	bvn := sim.SubnetFor(aliceUrl)
	var blockHash, bptRoot []byte
	var err error
	_ = bvn.Database.View(func(b *database.Batch) error {
		blockHash, err = b.GetMinorRootChainAnchor(&bvn.Executor.Network)
		require.NoError(t, err)
		require.NoError(t, b.SaveState(filename, &bvn.Executor.Network))
		bptRoot = b.BptRoot()
		return nil
	})

	// Load the file into a new database
	db := database.OpenInMemory(nil)
	var blockHash2, bptRoot2 []byte
	require.NoError(t, db.Update(func(b *database.Batch) error {
		require.NoError(t, b.LoadState(filename))
		blockHash2, err = b.GetMinorRootChainAnchor(&bvn.Executor.Network)
		require.NoError(t, err)
		bptRoot2 = b.BptRoot()
		return nil
	}))

	// Does it match?
	require.Equal(t, blockHash, blockHash2)
	require.Equal(t, bptRoot, bptRoot2)
}
