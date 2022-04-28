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

func init() { acctesting.EnableDebugFeatures() }

func TestSyntheticCreateIdentity_MultiSlash(t *testing.T) {
	db := database.OpenInMemory(nil)

	fooKey := generateKey()
	batch := db.Begin(true)
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, batch.Commit())

	book, err := url.Parse("foo/book0")
	require.NoError(t, err)

	account := new(protocol.TokenAccount)
	account.Url, err = url.Parse("foo/bar/baz")
	require.NoError(t, err)
	account.TokenUrl = protocol.AcmeUrl()
	account.AddAuthority(book)
	body := new(protocol.SyntheticCreateIdentity)
	body.Accounts = []protocol.Account{account}
	cause := [32]byte{1}
	body.SetSyntheticOrigin(cause[:], acctesting.FakeBvn)

	env := acctesting.NewTransaction().
		WithPrincipal(url.MustParse("foo")).
		WithSigner(protocol.FormatKeyPageUrl(book, 0), 1).
		WithTimestamp(1).
		WithBody(body).
		Initiate(protocol.SignatureTypeED25519, fooKey).
		Build()

	st, d := NewStateManagerForTest(t, db, env)
	defer st.Discard()

	scc := SyntheticCreateIdentity{}
	_, err = scc.Validate(st, d)
	require.EqualError(t, err, `missing identity for acc://foo/bar/baz`) // We created ADI acc://foo not acc://foo/bar
}

func TestSyntheticCreateIdentity_MultiSlash_SubADI(t *testing.T) {
	db := database.OpenInMemory(nil)

	fooKey := generateKey()
	batch := db.Begin(true)
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateSubADI(batch, "foo", "foo/bar"))
	require.NoError(t, batch.Commit())

	book, err := url.Parse("foo/book0")
	require.NoError(t, err)

	account := new(protocol.TokenAccount)
	account.Url, err = url.Parse("foo/bar/baz")
	require.NoError(t, err)
	account.TokenUrl = protocol.AcmeUrl()
	account.AddAuthority(book)
	body := new(protocol.SyntheticCreateIdentity)
	body.Accounts = []protocol.Account{account}
	cause := [32]byte{1}
	body.SetSyntheticOrigin(cause[:], acctesting.FakeBvn)

	env := acctesting.NewTransaction().
		WithPrincipal(url.MustParse("foo")).
		WithSigner(protocol.FormatKeyPageUrl(book, 0), 1).
		WithTimestamp(1).
		WithBody(body).
		Initiate(protocol.SignatureTypeED25519, fooKey).
		Build()

	st, d := NewStateManagerForTest(t, db, env)
	defer st.Discard()

	require.NoError(t, err)
	_, err = SyntheticCreateIdentity{}.Validate(st, d)
	require.NoError(t, err) // We created ADI acc://foo not acc://foo/bar
}
