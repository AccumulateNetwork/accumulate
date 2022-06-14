package abci_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/accumulated"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/v1"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

func init() { acctesting.EnableDebugFeatures() }

func TestStateDBConsistency(t *testing.T) {
	acctesting.SkipPlatformCI(t, "darwin", "flaky")

	subnets, daemons := acctesting.CreateTestNet(t, 1, 1, 0, false)
	stores := map[*accumulated.Daemon]storage.KeyValueStore{}
	for _, netName := range subnets {
		for _, daemon := range daemons[netName] {
			// store, err := badger.New(filepath.Join(daemon.Config.RootDir, "badger.db"), nil)
			// require.NoError(t, err)
			// stores[daemon] = store
			store := memory.New(nil)
			stores[daemon] = store

			// Call during test cleanup. This ensures that the app client is shutdown
			// before the database is closed.
			t.Cleanup(func() { store.Close() })
		}
	}

	getDb := func(d *accumulated.Daemon) (*database.Database, error) { return database.New(stores[d], d.Logger), nil }
	nodes := RunTestNet(t, subnets, daemons, getDb, true, nil)
	n := nodes[subnets[1]][0]

	credits := 40.0
	n.testLiteTx(10, 1, credits)

	for _, nodes := range nodes {
		for _, node := range nodes {
			node.client.Shutdown()
		}
	}

	ledger := n.network.NodeUrl(protocol.Ledger)
	var ledger1 *protocol.SystemLedger
	batch := n.db.Begin(false)
	require.NoError(t, batch.Account(ledger).GetStateAs(&ledger1))
	anchor, err := batch.GetMinorRootChainAnchor(n.network)
	if err != nil {
		panic(fmt.Errorf("failed to get anchor: %v", err))
	}
	rootHash := anchor
	batch.Discard()

	// Reopen the database
	db := database.New(stores[daemons[subnets[1]][0]], nil)

	// Block 6 does not make changes so is not saved
	batch = db.Begin(false)
	var ledger2 *protocol.SystemLedger
	anchor, err = batch.GetMinorRootChainAnchor(n.network)
	require.NoError(t, err)
	require.NoError(t, batch.Account(ledger).GetStateAs(&ledger2))
	require.Equal(t, ledger1, ledger2, "Ledger does not match after load from disk")
	require.Equal(t, fmt.Sprintf("%X", rootHash), fmt.Sprintf("%X", anchor), "Hash does not match after load from disk")
	batch.Discard()

	// Recreate the app and try to do more transactions
	nodes = RunTestNet(t, subnets, daemons, getDb, false, nil)
	n = nodes[subnets[1]][0]

	n.testLiteTx(10, 1, credits)
}
