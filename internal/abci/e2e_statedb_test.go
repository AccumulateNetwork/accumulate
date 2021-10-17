package abci_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/AccumulateNetwork/accumulated/smt/storage/badger"
	"github.com/AccumulateNetwork/accumulated/types/state"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto"
)

func TestStateDBConsistency(t *testing.T) {
	dir := t.TempDir()
	db := new(badger.DB)
	err := db.InitDB(filepath.Join(dir, "valacc.db"))
	require.NoError(t, err)
	defer db.Close()

	bvcId := sha256.Sum256([]byte("foo bar"))
	sdb := new(state.StateDB)
	sdb.Load(db, bvcId[:], true)

	n := createApp(t, sdb, crypto.Address{})
	n.testAnonTx(10)

	height := sdb.BlockIndex()
	rootHash := sdb.RootHash()

	// Reopen the database
	sdb = new(state.StateDB)
	sdb.Load(db, bvcId[:], true)

	// Block 6 does not make changes so is not saved
	require.Equal(t, height, sdb.BlockIndex(), "Block index does not match after load from disk")
	require.Equal(t, fmt.Sprintf("%X", rootHash), fmt.Sprintf("%X", sdb.RootHash()), "Hash does not match after load from disk")

	// Recreate the app and try to do more transactions
	n = createApp(t, sdb, crypto.Address{})
	n.testAnonTx(10)
}
