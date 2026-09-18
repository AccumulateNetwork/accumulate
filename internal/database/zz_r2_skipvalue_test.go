// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestR2_WhatSkipWouldWrite reports the VALUE a skip-the-entry policy produces,
// rather than only that it produces one. Run against the skip mutation it must
// print 290 (the later occurrence), which is the wrong-value bug the
// first-occurrence rule exists to prevent.
func TestR2_WhatSkipWouldWrite(t *testing.T) {
	const n = 300
	const early, late = 5, 290

	u := protocol.AccountUrl("foo.acme", "tokens")
	built := database.OpenInMemory(nil)
	batch := built.Begin(true)
	acct := new(protocol.TokenAccount)
	acct.Url = u
	acct.TokenUrl = protocol.AcmeUrl()
	acct.Balance = *big.NewInt(1000)
	require.NoError(t, batch.Account(u).Main().Put(acct))
	c, err := batch.Account(u).MainChain().Get()
	require.NoError(t, err)
	var hashes [][]byte
	for i := 0; i < n; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("entry %d", i)))
		hashes = append(hashes, h[:])
		if i == late {
			require.NoError(t, c.AddEntry(hashes[early], false))
			hashes[i] = hashes[early]
			continue
		}
		require.NoError(t, c.AddEntry(h[:], false))
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	batch.Discard()

	snap := collectSnapshot(t, built)

	restored := database.OpenInMemory(nil)
	require.NoError(t, database.Restore(restored, ioutil.NewBuffer(snap), &database.RestoreOptions{
		SkipHashCheck: true,
		Predicate: func(e *snapshot.RecordEntry) (bool, error) {
			if e.Key.Get(0) != "Account" || !u.Equal(e.Key.Get(1).(*url.URL)) {
				return false, nil
			}
			if e.Key.Get(2) == "MainChain" && e.Key.Len() > 3 && e.Key.Get(3) == "States" {
				return false, nil
			}
			return true, nil
		},
	}))

	rc := restored.Begin(false)
	defer rc.Discard()
	c2, err := rc.Account(u).MainChain().Get()
	require.NoError(t, err)

	h, err := c2.HeightOf(hashes[early])
	t.Logf("RESTORED HeightOf(entry %d's hash) = %d, err = %v (builder says %d)", early, h, err, early)

	// Also report how much of the chain got indexed
	var indexed int
	for i := 0; i < n; i++ {
		if _, e := c2.HeightOf(hashes[i]); e == nil {
			indexed++
		}
	}
	t.Logf("RESTORED indexed %d of %d entries", indexed, n)
}
