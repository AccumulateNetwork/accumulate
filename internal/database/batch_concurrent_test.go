// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4145 hazard (iv): a Batch is not safe for concurrent use — even a READ
// from a child memoizes into the parent. BeginConcurrent children serialize
// every touch of the shared parent through one mutex, so sibling children
// can run concurrently. Run with -race: that is the assertion the mutex
// exists for.
func TestBeginConcurrent_SiblingsAreRaceFree(t *testing.T) {
	db := OpenInMemory(nil)

	// Seed accounts, including one SHARED account every child reads. Lite
	// accounts, so the observer's account validation is satisfied without a
	// key book.
	lid := func(n byte) *url.URL {
		return protocol.LiteAuthorityForKey([]byte{31: n}, protocol.SignatureTypeED25519)
	}
	batch := db.Begin(true)
	shared := lid(255)
	require.NoError(t, batch.Account(shared).Main().Put(&protocol.LiteIdentity{Url: shared}))
	const n = 8
	for i := 0; i < n; i++ {
		u := lid(byte(i)).JoinPath("ACME")
		require.NoError(t, batch.Account(lid(byte(i))).Main().Put(&protocol.LiteIdentity{Url: lid(byte(i))}))
		require.NoError(t, batch.Account(u).Main().Put(&protocol.LiteTokenAccount{Url: u, TokenUrl: protocol.AcmeUrl()}))
	}
	require.NoError(t, batch.Commit())

	// One block-level batch, n concurrent children: each reads the shared
	// account (pull-through memoization on the parent — the racy part) and
	// writes its own.
	block := db.Begin(true)
	defer block.Discard()

	var mu sync.Mutex
	children := make([]*Batch, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		children[i] = block.BeginConcurrent(&mu, true)
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := children[i]

			var l *protocol.LiteIdentity
			require.NoError(t, c.Account(shared).Main().GetAs(&l))

			u := lid(byte(i)).JoinPath("ACME")
			var acct *protocol.LiteTokenAccount
			require.NoError(t, c.Account(u).Main().GetAs(&acct))
			acct.Balance = *big.NewInt(int64(i + 1))
			require.NoError(t, c.Account(u).Main().Put(acct))
		}(i)
	}
	wg.Wait()

	// Commits are the caller's job to serialize — in a deterministic order.
	for _, c := range children {
		require.NoError(t, c.Commit())
	}
	require.NoError(t, block.Commit())

	// Every child's write landed.
	check := db.Begin(false)
	defer check.Discard()
	for i := 0; i < n; i++ {
		u := lid(byte(i)).JoinPath("ACME")
		var acct *protocol.LiteTokenAccount
		require.NoError(t, check.Account(u).Main().GetAs(&acct))
		assert.EqualValues(t, i+1, acct.Balance.Int64())
	}
}
