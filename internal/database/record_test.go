package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestMerkleRecord(t *testing.T) {
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	c, err := database.WrapChain(batch.Account(protocol.AcmeUrl()).MainChain())
	require.NoError(t, err)
	entry := [32]byte{1}
	require.NoError(t, c.AddEntry(entry[:], true))

	c2, err := database.WrapChain(batch.Account(protocol.AcmeUrl()).MainChain())
	require.NoError(t, err)
	require.NotZero(t, c2.Height())

	require.NoError(t, batch.Commit())

	batch2 := db.Begin(true)
	defer batch2.Discard()

	c3, err := database.WrapChain(batch2.Account(protocol.AcmeUrl()).MainChain())
	require.NoError(t, err)
	require.NotZero(t, c3.Height())
}
