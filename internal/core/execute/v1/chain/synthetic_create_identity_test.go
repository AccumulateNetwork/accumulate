// Copyright 2024 The Accumulate Authors
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func init() { acctesting.EnableDebugFeatures() }

func TestSyntheticCreateIdentity_MultiSlash(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(acctesting.NullObserver{})

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

	env :=
		MustBuild(t, build.Transaction().
			For(protocol.AccountUrl("foo")).
			Body(body).
			SignWith(protocol.FormatKeyPageUrl(book, 0)).Version(1).Timestamp(1).PrivateKey(fooKey).Type(protocol.SignatureTypeED25519))

	st, d := LoadStateManagerForTest(t, db, env)
	defer st.Discard()

	scc := SyntheticCreateIdentity{}
	_, err := scc.Validate(st, d)
	require.EqualError(t, err, `missing identity for acc://foo.acme/bar/baz`) // We created ADI acc://foo not acc://foo/bar
}

func TestSyntheticCreateIdentity_MultiSlash_SubADI(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(acctesting.NullObserver{})

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

	env :=
		MustBuild(t, build.Transaction().
			For(protocol.AccountUrl("foo")).
			Body(body).
			SignWith(protocol.FormatKeyPageUrl(book, 0)).Version(1).Timestamp(1).PrivateKey(fooKey).Type(protocol.SignatureTypeED25519))

	st, d := LoadStateManagerForTest(t, db, env)
	defer st.Discard()

	_, err := SyntheticCreateIdentity{}.Validate(st, d)
	require.NoError(t, err) // We created ADI acc://foo not acc://foo/bar
}
