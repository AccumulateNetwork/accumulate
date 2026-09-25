// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestForgetAccountRemovesTheWholeAccount — #4438 review F4. An account a
// joining node's own execution created, and no peer holds, is removed whole:
// no main state, no chain, no chain list, no directory, no pending entry, no
// leaf. Main state and leaf alone left an account with chains and no main
// state, which invariant 13 calls a defect.
func TestForgetAccountRemovesTheWholeAccount(t *testing.T) {
	db := OpenInMemory(nil)
	db.SetObserver(NewDatabaseObserver())
	u := url.MustParse("bob/extra")

	batch := db.Begin(true)
	require.NoError(t, batch.Account(u).Main().Put(&protocol.TokenAccount{Url: u, TokenUrl: protocol.AcmeUrl()}))
	c, err := batch.Account(u).ChainByName("main")
	require.NoError(t, err)
	require.NoError(t, c.Inner().AddEntry(make([]byte, 32), false))
	require.NoError(t, batch.Account(u).Directory().Add(u.JoinPath("child")))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	batch = db.Begin(true)
	require.NoError(t, batch.ForgetAccount(u))
	require.NoError(t, batch.Commit())

	batch = db.Begin(false)
	defer batch.Discard()
	_, err = batch.Account(u).Main().Get()
	require.ErrorIs(t, err, errors.NotFound, "main state")
	chains, err := batch.Account(u).Chains().Get()
	require.NoError(t, err)
	require.Empty(t, chains, "chain list")
	c, err = batch.Account(u).ChainByName("main")
	require.NoError(t, err)
	head, err := c.Inner().Head().Get()
	require.NoError(t, err)
	require.Zero(t, head.Count, "main chain")
	dir, err := batch.Account(u).Directory().Get()
	require.NoError(t, err)
	require.Empty(t, dir, "directory")
	_, err = batch.BPT().Get(record.NewKey("Account", u))
	require.ErrorIs(t, err, errors.NotFound, "leaf")
}
