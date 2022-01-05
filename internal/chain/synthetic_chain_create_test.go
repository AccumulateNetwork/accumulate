package chain_test

import (
	"testing"

	. "github.com/AccumulateNetwork/accumulate/internal/chain"
	"github.com/AccumulateNetwork/accumulate/internal/database"
	acctesting "github.com/AccumulateNetwork/accumulate/internal/testing"
	"github.com/AccumulateNetwork/accumulate/internal/url"
	"github.com/AccumulateNetwork/accumulate/protocol"
	"github.com/AccumulateNetwork/accumulate/types"
	"github.com/AccumulateNetwork/accumulate/types/api/transactions"
	"github.com/stretchr/testify/require"
)

func TestSyntheticChainCreate_MultiSlash(t *testing.T) {
	db, err := database.Open("", true, nil)
	require.NoError(t, err)

	fooKey := generateKey()
	batch := db.Begin()
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, batch.Commit())

	book, err := url.Parse("foo/book0")
	require.NoError(t, err)

	account := protocol.NewTokenAccount()
	account.ChainUrl = "foo/bar/baz"
	account.TokenUrl = protocol.ACME
	account.KeyBook = types.String(book.String())
	body := new(protocol.SyntheticCreateChain)
	body.Cause[0] = 1
	require.NoError(t, body.Create(account))

	tx, err := transactions.New("foo", 1, edSigner(fooKey, 1), body)
	require.NoError(t, err)

	st, err := NewStateManager(db.Begin(), protocol.BvnUrl(t.Name()), tx)
	require.NoError(t, err)

	err = SyntheticCreateChain{}.Validate(st, tx)
	require.EqualError(t, err, `chain type tokenAccount cannot contain more than one slash in its URL`)
}
