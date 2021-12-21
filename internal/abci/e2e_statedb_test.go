package abci_test

import (
	"fmt"
	"path/filepath"
	"testing"

	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/smt/storage/badger"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto"
)

func TestStateDBConsistency(t *testing.T) {
	acctesting.SkipPlatformCI(t, "darwin", "flaky")

	dir := t.TempDir()
	db := new(badger.DB)
	err := db.InitDB(filepath.Join(dir, "valacc.db"), nil)
	require.NoError(t, err)

	// Call during test cleanup. This ensures that the app client is shutdown
	// before the database is closed.
	t.Cleanup(func() { db.Close() })

	sdb := new(state.StateDB)
	require.NoError(t, sdb.Load(db, true))

	n := createApp(t, sdb, crypto.Address{}, true)
	n.testLiteTx(10)

	height, err := sdb.BlockIndex()
	require.NoError(t, err)
	rootHash := sdb.RootHash()
	n.client.Shutdown()

	// Reopen the database
	sdb = new(state.StateDB)
	require.NoError(t, sdb.Load(db, true))

	// Block 6 does not make changes so is not saved
	height2, err := sdb.BlockIndex()
	require.NoError(t, err)
	require.Equal(t, height, height2, "Block index does not match after load from disk")
	require.Equal(t, fmt.Sprintf("%X", rootHash), fmt.Sprintf("%X", sdb.RootHash()), "Hash does not match after load from disk")

	// Recreate the app and try to do more transactions
	n = createApp(t, sdb, crypto.Address{}, false)
	n.testLiteTx(10)
}
