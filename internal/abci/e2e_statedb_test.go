package abci_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/badger"
)

func TestStateDBConsistency(t *testing.T) {
	acctesting.SkipPlatformCI(t, "darwin", "flaky")

	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0)
	stores := map[*accumulated.Daemon]*badger.DB{}
	for _, netName := range subnets {
		for _, daemon := range daemons[netName] {
			store := new(badger.DB)
			err := store.InitDB(filepath.Join(daemon.Config.RootDir, "valacc.db"), nil)
			require.NoError(t, err)
			stores[daemon] = store

			// Call during test cleanup. This ensures that the app client is shutdown
			// before the database is closed.
			t.Cleanup(func() { store.Close() })
		}
	}

	getDb := func(d *accumulated.Daemon) (*database.Database, error) { return database.New(stores[d], d.Logger), nil }
	nodes := RunTestNet(t, subnets, daemons, getDb, true)
	n := nodes[subnets[1]][0]

	n.testLiteTx(10)

	for _, nodes := range nodes {
		for _, node := range nodes {
			node.client.Shutdown()
		}
	}

	ledger := n.network.NodeUrl(protocol.Ledger)
	ledger1 := protocol.NewInternalLedger()
	batch := n.db.Begin(false)
	require.NoError(t, batch.Account(ledger).GetStateAs(ledger1))
	rootHash := batch.RootHash()
	batch.Discard()

	// Reopen the database
	db := database.New(stores[daemons[subnets[1]][0]], nil)

	// Block 6 does not make changes so is not saved
	batch = db.Begin(false)
	ledger2 := protocol.NewInternalLedger()
	require.NoError(t, batch.Account(ledger).GetStateAs(ledger2))
	require.Equal(t, ledger1, ledger2, "Ledger does not match after load from disk")
	require.Equal(t, fmt.Sprintf("%X", rootHash), fmt.Sprintf("%X", batch.RootHash()), "Hash does not match after load from disk")
	batch.Discard()

	// Recreate the app and try to do more transactions
	nodes = RunTestNet(t, subnets, daemons, getDb, false)
	n = nodes[subnets[1]][0]
	n.testLiteTx(10)
}
