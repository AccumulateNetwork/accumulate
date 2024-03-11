// Copyright 2024 The Accumulate Authors
// 
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	. "gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

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
	db.SetObserver(acctesting.NullObserver{})
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
