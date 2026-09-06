// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bcdb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A batch begun through Unisolated() -- what CheckTx validates against --
// pins no version on a store that keeps isolation by pre-image: a block
// committed while it is open reads nothing to keep it isolated, and the
// batch sees the block.
func TestUnisolatedBatchPinsNoVersion(t *testing.T) {
	store, err := bcdb.Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()
	db := database.New(store, nil)

	alice := url.MustParse("alice.acme")
	writeBalance := func(n uint64) {
		require.NoError(t, db.Update(func(batch *database.Batch) error {
			acct := &protocol.LiteIdentity{Url: alice, CreditBalance: n}
			return batch.Account(alice).Main().Put(acct)
		}))
	}
	writeBalance(1)

	// CheckTx's batch, open across a block
	checkTx := db.Unisolated().Begin(false)
	defer checkTx.Discard()
	_, err = checkTx.Account(alice).Main().Get()
	require.NoError(t, err)

	before := store.PreImageReads()
	writeBalance(2)
	require.Equal(t, before, store.PreImageReads(), "a commit under an unisolated reader takes no pre-images")

	// A fresh read through the same batch sees the block. The batch's
	// record layer caches what it has loaded, so ask for a record it has not.
	var acct *protocol.LiteIdentity
	require.NoError(t, db.Unisolated().View(func(batch *database.Batch) error {
		return batch.Account(alice).Main().GetAs(&acct)
	}))
	require.Equal(t, uint64(2), acct.CreditBalance)

	// The contrast: an ordinary batch is kept at its version, at a cost
	pinned := db.Begin(false)
	defer pinned.Discard()
	writeBalance(3)
	require.Greater(t, store.PreImageReads(), before, "a pinned reader is what pre-images are read for")
	require.NoError(t, pinned.Account(alice).Main().GetAs(&acct))
	require.Equal(t, uint64(2), acct.CreditBalance)
}

// On a store that isolates for free, Unisolated() is the same database.
func TestUnisolatedOnAMemoryStoreIsOrdinary(t *testing.T) {
	db := database.New(memory.New(nil), nil)
	alice := url.MustParse("alice.acme")
	require.NoError(t, db.Unisolated().Update(func(batch *database.Batch) error {
		return batch.Account(alice).Main().Put(&protocol.LiteIdentity{Url: alice})
	}))
	require.NoError(t, db.View(func(batch *database.Batch) error {
		_, err := batch.Account(alice).Main().Get()
		return err
	}))
}
