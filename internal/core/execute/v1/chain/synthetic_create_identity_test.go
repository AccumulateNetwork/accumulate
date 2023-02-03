// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package chain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v1/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() { acctesting.EnableDebugFeatures() }

func TestSyntheticCreateIdentity_MultiSlash(t *testing.T) {
	db := database.OpenInMemory(nil)

	fooKey := generateKey()
	batch := db.Begin(true)
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, batch.Commit())

	book := protocol.AccountUrl("foo", "book0")

	account := new(protocol.TokenAccount)
	account.Url = protocol.AccountUrl("foo", "bar", "baz")
	account.TokenUrl = protocol.AcmeUrl()
	account.AddAuthority(book)
	body := new(protocol.SyntheticCreateIdentity)
	body.Accounts = []protocol.Account{account}
	cause := [32]byte{1}
	body.SetCause(cause, protocol.PartitionUrl("X"))

	env := acctesting.NewTransaction().
		WithPrincipal(protocol.AccountUrl("foo")).
		WithSigner(protocol.FormatKeyPageUrl(book, 0), 1).
		WithTimestamp(1).
		WithBody(body).
		Initiate(protocol.SignatureTypeED25519, fooKey).
		Build()

	st, d := LoadStateManagerForTest(t, db, env)
	defer st.Discard()

	scc := SyntheticCreateIdentity{}
	_, err := scc.Validate(st, d)
	require.EqualError(t, err, `missing identity for acc://foo.acme/bar/baz`) // We created ADI acc://foo not acc://foo/bar
}

func TestSyntheticCreateIdentity_MultiSlash_SubADI(t *testing.T) {
	db := database.OpenInMemory(nil)

	fooKey := generateKey()
	batch := db.Begin(true)
	require.NoError(t, acctesting.CreateADI(batch, fooKey, "foo"))
	require.NoError(t, acctesting.CreateSubADI(batch, "foo", "foo/bar"))
	require.NoError(t, batch.Commit())

	book := protocol.AccountUrl("foo", "book0")

	account := new(protocol.TokenAccount)
	account.Url = protocol.AccountUrl("foo", "bar", "baz")
	account.TokenUrl = protocol.AcmeUrl()
	account.AddAuthority(book)
	body := new(protocol.SyntheticCreateIdentity)
	body.Accounts = []protocol.Account{account}
	cause := [32]byte{1}
	body.SetCause(cause, protocol.PartitionUrl("X"))

	env := acctesting.NewTransaction().
		WithPrincipal(protocol.AccountUrl("foo")).
		WithSigner(protocol.FormatKeyPageUrl(book, 0), 1).
		WithTimestamp(1).
		WithBody(body).
		Initiate(protocol.SignatureTypeED25519, fooKey).
		Build()

	st, d := LoadStateManagerForTest(t, db, env)
	defer st.Discard()

	_, err := SyntheticCreateIdentity{}.Validate(st, d)
	require.NoError(t, err) // We created ADI acc://foo not acc://foo/bar
}
