package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/smt/storage/memory"
)

func init() { acctesting.EnableDebugFeatures() }

func TestAccountChainMetadata(t *testing.T) {
	store := memory.New(nil)
	db := database.New(store, nil)
	cs := db.Begin(true)
	defer cs.Discard()

	alice := protocol.AccountUrl("alice")
	chains, err := cs.Account(alice).Chains().Get()
	require.NoError(t, err)
	require.Empty(t, chains)

	err = cs.Account(alice).MainChain().AddHash(make([]byte, 32), false)
	require.NoError(t, err)
	require.NoError(t, cs.Commit())

	cs = db.Begin(true)
	defer cs.Discard()
	chains, err = cs.Account(alice).Chains().Get()
	require.NoError(t, err)
	require.Len(t, chains, 1)

	err = cs.Account(alice).Pending().Add(url.MustParseTxID("e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5@bob"))
	require.NoError(t, err)
	err = cs.Account(alice).Pending().Add(url.MustParseTxID("e43be90e349210456662d8b8bdc9cc9e5e46ccb07f2129e7b57a8195e5e916d5@alice"))
	require.NoError(t, err)
	pending, err := cs.Account(alice).Pending().Get()
	require.NoError(t, err)
	require.Len(t, pending, 2)
}
