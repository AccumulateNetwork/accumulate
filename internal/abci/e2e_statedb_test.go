package abci_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/AccumulateNetwork/accumulate/internal/database"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/smt/storage/badger"
	"github.com/AccumulateNetwork/accumulate/types/state"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/crypto"
)

func TestStateDBConsistency(t *testing.T) {
	acctesting.SkipPlatformCI(t, "darwin", "flaky")

	dir := t.TempDir()
	store := new(badger.DB)
	err := store.InitDB(filepath.Join(dir, "valacc.db"), nil)
	require.NoError(t, err)

	// Call during test cleanup. This ensures that the app client is shutdown
	// before the database is closed.
	t.Cleanup(func() { store.Close() })

	db := database.New(store, nil)
	n := createApp(t, db, crypto.Address{}, true)
	n.testLiteTx(10)

	rootUrl := n.network.NodeUrl().JoinPath(protocol.MinorRoot)
	batch := db.Begin()
	root := new(state.Anchor)
	require.NoError(t, batch.Record(rootUrl).GetStateAs(root))
	rootHash := batch.RootHash()
	batch.Discard()
	n.client.Shutdown()

	// Reopen the database
	db = database.New(store, nil)

	// Block 6 does not make changes so is not saved
	batch = db.Begin()
	root2 := new(state.Anchor)
	require.NoError(t, batch.Record(rootUrl).GetStateAs(root2))
	require.Equal(t, root, root, "Block index does not match after load from disk")
	require.Equal(t, fmt.Sprintf("%X", rootHash), fmt.Sprintf("%X", batch.RootHash()), "Hash does not match after load from disk")
	batch.Discard()

	// Recreate the app and try to do more transactions
	n = createApp(t, db, crypto.Address{}, false)
	n.testLiteTx(10)
}
