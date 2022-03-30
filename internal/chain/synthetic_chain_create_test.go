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
	db := database.OpenInMemory(nil)

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
	body.SetSyntheticOrigin([]byte{1}, acctesting.FakeBvn)
	require.NoError(t, body.Create(account))

	env := acctesting.NewTransaction().
		WithPrincipal(url.MustParse("foo")).
		WithSigner(protocol.FormatKeyPageUrl(book, 0), 1).
		WithTimestamp(1).
		WithBody(body).
		Initiate(protocol.SignatureTypeED25519, fooKey)

	st := NewStateManagerForTest(t, db, env)
	defer st.Discard()

	scc := SyntheticCreateChain{}
	result, err := scc.Validate(st, env)
	require.EqualError(t, err, `missing identity for acc://foo/bar/baz`) // We created ADI acc://foo not acc://foo/bar

	status := &protocol.TransactionStatus{Delivered: true, Result: result}
	receiptEnv := CreateSynthReceipt(env, status, acctesting.FakeBvn)
	principalUrl := env.Transaction.Header.Principal
	env = acctesting.NewTransaction().
		WithPrincipal(principalUrl).
		WithSigner(protocol.FormatKeyPageUrl(book, 0), 1).
		WithCurrentTimestamp().
		WithBody(receiptEnv.SyntheticReceipt).
		Initiate(protocol.SignatureTypeED25519, fooKey)
	_, err = SyntheticReceipt{}.Validate(st, env)
	require.NoError(t, err)

}

func TestSyntheticChainCreate_MultiSlash_SubADI(t *testing.T) {
	db := database.OpenInMemory(nil)

	fooKey := generateKey()
	batch := db.Begin(true)
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateSubADI(batch, "foo", "foo/bar"))
	require.NoError(t, batch.Commit())

	book, err := url.Parse("foo/book0")
	require.NoError(t, err)

	account := protocol.NewTokenAccount()
	account.Url, err = url.Parse("foo/bar/baz")
	require.NoError(t, err)
	account.TokenUrl = protocol.AcmeUrl()
	account.KeyBook = book
	body := new(protocol.SyntheticCreateChain)
	body.SetSyntheticOrigin([]byte{1}, acctesting.FakeBvn)
	require.NoError(t, body.Create(account))

	env := acctesting.NewTransaction().
		WithPrincipal(url.MustParse("foo")).
		WithSigner(protocol.FormatKeyPageUrl(book, 0), 1).
		WithTimestamp(1).
		WithBody(body).
		Initiate(protocol.SignatureTypeED25519, fooKey)

	st := NewStateManagerForTest(t, db, env)
	defer st.Discard()

	_, err = SyntheticCreateChain{}.Validate(st, env)
	require.NoError(t, err) // We created ADI acc://foo not acc://foo/bar
}
