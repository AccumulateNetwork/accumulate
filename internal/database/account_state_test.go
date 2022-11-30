// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestAccountState verifies that adding an entry to an ADI's directory listing changes the account's BPT entry.
func TestAccountState(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()
	adiurl := url.MustParse("acc://testadi1.acme")
	bookurl := url.MustParse("acc://testadi1.acme/testbook1")
	testurl := url.MustParse("acc://testurl")
	a := batch.Account(adiurl)
	acc := &protocol.ADI{Url: adiurl, AccountAuth: protocol.AccountAuth{Authorities: []protocol.AuthorityEntry{{Url: bookurl}}}}
	err := a.PutState(acc)
	require.NoError(t, err)
	h1, err := a.hashState()
	require.NoError(t, err)
	err = a.Directory().Add(testurl)
	require.NoError(t, err)
	h2, err := a.hashState()
	require.NoError(t, err)
	require.NotEqual(t, h1.MerkleHash(), h2.MerkleHash())
}

func TestStripUrl_Get(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(false)
	defer batch.Discard()

	foo := protocol.AccountUrl("foo")
	require.Empty(t, batch.Account(foo.WithQuery("bar")).Url().Query)
	require.Empty(t, batch.Account(foo.WithFragment("bar")).Url().Fragment)
	require.Empty(t, batch.Account(foo.WithUserInfo("bar")).Url().UserInfo)
}

func TestStripUrl_Put(t *testing.T) {
	db := OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	foo := protocol.AccountUrl("foo")
	err := batch.Account(foo).Main().Put(&protocol.UnknownAccount{Url: foo.WithQuery("bar")})
	require.NoError(t, err)
	require.NoError(t, batch.Commit())

	batch = db.Begin(false)
	defer batch.Discard()
	a, err := batch.Account(foo).Main().Get()
	require.NoError(t, err)
	require.Empty(t, a.GetUrl().Query)
}
