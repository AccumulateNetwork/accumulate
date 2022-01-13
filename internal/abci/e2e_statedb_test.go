package abci_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/AccumulateNetwork/accumulate/internal/accumulated"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage/badger"
	"github.com/stretchr/testify/require"
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

	ledger := n.network.NodeUrl(protocol.Ledger)
	ledger1 := protocol.NewInternalLedger()
	batch := n.db.Begin()
	require.NoError(t, batch.Account(ledger).GetStateAs(ledger1))
	rootHash := batch.RootHash()
	batch.Discard()
	n.client.Shutdown()

	// Reopen the database
	db := database.New(stores[daemons[subnets[1]][0]], nil)

	// Block 6 does not make changes so is not saved
	batch = db.Begin()
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
