package database_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/block/simulator"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

func init() { acctesting.EnableDebugFeatures() }

var delivered = (*protocol.TransactionStatus).Delivered

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
	f, err := os.Create(filepath.Join(t.TempDir(), "state.bpt"))
	require.NoError(t, err)
	defer f.Close()

	bvn := sim.PartitionFor(aliceUrl)
	var blockHash, bptRoot []byte
	_ = bvn.Database.View(func(b *database.Batch) error {
		blockHash, err = b.GetMinorRootChainAnchor(&bvn.Executor.Describe)
		require.NoError(t, err)
		require.NoError(t, b.SaveSnapshot(f, &bvn.Executor.Describe))
		bptRoot = b.BptRoot()
		return nil
	})

	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)

	// Load the file into a new database
	db := database.OpenInMemory(nil)
	require.NoError(t, db.Update(func(b *database.Batch) error {
		require.NoError(t, b.RestoreSnapshot(f, &bvn.Executor.Describe))

		// Does it match?
		blockHash2, err := b.GetMinorRootChainAnchor(&bvn.Executor.Describe)
		require.NoError(t, err)
		require.Equal(t, blockHash, blockHash2)
		require.Equal(t, bptRoot, b.BptRoot())

		// Verify that transactions and signatures are saved
		c, err := b.Account(protocol.FaucetUrl).MainChain().Get()
		require.NoError(t, err)
		hash, err := c.Entry(0)
		require.NoError(t, err)
		_, err = b.Transaction(hash).Main().Get()
		require.NoError(t, err)

		c, err = b.Account(protocol.FaucetUrl).SignatureChain().Get()
		require.NoError(t, err)
		hash, err = c.Entry(0)
		require.NoError(t, err)
		_, err = b.Transaction(hash).Main().Get()
		require.NoError(t, err)

		return nil
	}))

}
