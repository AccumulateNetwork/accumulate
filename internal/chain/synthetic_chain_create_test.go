package chain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	acctesting "gitlab.com/accumulatenetwork/accumulate/internal/testing"
	"gitlab.com/accumulatenetwork/accumulate/internal/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func TestSyntheticChainCreate_MultiSlash(t *testing.T) {
	db, err := database.Open("", true, nil)
	require.NoError(t, err)

	fooKey := generateKey()
	batch := db.Begin(true)
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, batch.Commit())

	book, err := url.Parse("foo/book0")
	require.NoError(t, err)

	account := protocol.NewTokenAccount()
	account.Url, err = url.Parse("foo/bar/baz")
	require.NoError(t, err)
	account.TokenUrl = protocol.AcmeUrl()
	account.KeyBook = book
	body := new(protocol.SyntheticCreateChain)
	body.Cause[0] = 1
	require.NoError(t, body.Create(account))

	env := acctesting.NewTransaction().
		WithOriginStr("foo").
		WithKeyPage(0, 1).
		WithNonce(1).
		WithBody(body).
		SignLegacyED25519(fooKey)

	st, err := NewStateManager(db.Begin(true), protocol.BvnUrl(t.Name()), env)
	require.NoError(t, err)

	_, err = SyntheticCreateChain{}.Validate(st, env)
	require.EqualError(t, err, `account type tokenAccount cannot contain more than one slash in its URL`)
}
