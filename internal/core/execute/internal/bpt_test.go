// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package internal

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestAccountState verifies that adding an entry to an ADI's directory listing changes the account's BPT entry.
func TestAccountState(t *testing.T) {
	db := database.OpenInMemory(nil)
	db.SetObserver(databaseObserver{})
	batch := db.Begin(true)
	defer batch.Discard()
	adiurl := url.MustParse("acc://testadi1.acme")
	bookurl := url.MustParse("acc://testadi1.acme/testbook1")
	testurl := url.MustParse("acc://testurl")
	a := batch.Account(adiurl)
	acc := &protocol.ADI{Url: adiurl, AccountAuth: protocol.AccountAuth{Authorities: []protocol.AuthorityEntry{{Url: bookurl}}}}
	err := a.Main().Put(acc)
	require.NoError(t, err)
	h1, err := (&observedAccount{a, batch}).hashState()
	require.NoError(t, err)
	err = a.Directory().Add(testurl)
	require.NoError(t, err)
	h2, err := (&observedAccount{a, batch}).hashState()
	require.NoError(t, err)
	require.NotEqual(t, h1.MerkleHash(), h2.MerkleHash())
}
