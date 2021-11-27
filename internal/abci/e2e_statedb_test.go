package abci_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/AccumulateNetwork/accumulate/smt/storage/badger"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto"
)

func TestStateDBConsistency(t *testing.T) {
	dir := t.TempDir()
	db := new(badger.DB)
	err := db.InitDB(filepath.Join(dir, "valacc.db"), nil)
	require.NoError(t, err)
	defer db.Close()

	sdb := new(state.StateDB)
	require.NoError(t, sdb.Load(db, true))

	n := createApp(t, sdb, crypto.Address{}, "error", false)
	n.testAnonTx(10)

	height, err := sdb.BlockIndex()
	require.NoError(t, err)
	rootHash := sdb.RootHash()

	// Reopen the database
	sdb = new(state.StateDB)
	require.NoError(t, sdb.Load(db, true))

	// Block 6 does not make changes so is not saved
	height2, err := sdb.BlockIndex()
	require.NoError(t, err)
	require.Equal(t, height, height2, "Block index does not match after load from disk")
	require.Equal(t, fmt.Sprintf("%X", rootHash), fmt.Sprintf("%X", sdb.RootHash()), "Hash does not match after load from disk")

	// Recreate the app and try to do more transactions
	n = createApp(t, sdb, crypto.Address{}, "error", false)
	n.testAnonTx(10)
}
