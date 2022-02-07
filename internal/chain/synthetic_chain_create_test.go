package chain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/types/api/transactions"
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
	account.Url = "foo/bar/baz"
	account.TokenUrl = protocol.ACME
	account.KeyBook = book.String()
	body := new(protocol.SyntheticCreateChain)
	body.Cause[0] = 1
	require.NoError(t, body.Create(account))

	tx, err := transactions.New("foo", 1, edSigner(fooKey, 1), body)
	require.NoError(t, err)

	st, err := NewStateManager(db.Begin(), protocol.BvnUrl(t.Name()), tx)
	require.NoError(t, err)

	_, err = SyntheticCreateChain{}.Validate(st, tx)
	require.EqualError(t, err, `account type tokenAccount cannot contain more than one slash in its URL`)
}
