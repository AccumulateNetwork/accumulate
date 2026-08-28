// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bsn

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A chain that admits duplicate entries indexes a hash at its FIRST
// position, and the BSN's indexer must agree with the node's own
// AddEntry on that. Rewriting ElementIndex(H) with a later position is a
// different value under the same key, which the BlockchainDB backend's
// permanent layer refuses (#4174).
func TestChainElemIndexer_DuplicateKeepsFirstIndex(t *testing.T) {
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	chain := batch.Account(protocol.AccountUrl("alice")).MainChain()
	h := make([]byte, 32)
	h[0] = 0xAA
	other := make([]byte, 32)
	other[0] = 0xBB

	// The chain as the summary delivered it: h at 0, other at 1, h again
	// at 2 -- the node's AddEntry keeps h indexed at 0.
	inner, err := chain.Get()
	require.NoError(t, err)
	require.NoError(t, inner.AddEntry(h, false))
	require.NoError(t, inner.AddEntry(other, false))
	require.NoError(t, inner.AddEntry(h, false))

	// The BSN indexes from where it stood; the third entry is a duplicate
	// and must not move h's index.
	idx := &chainElemIndexer{key: record.NewKey("Account", protocol.AccountUrl("alice"), "MainChain"), oldHeight: 0}
	require.NoError(t, idx.Apply(nil, nil, chain))

	i, err := chain.IndexOf(h)
	require.NoError(t, err)
	require.Equal(t, int64(0), i, "a duplicate hash keeps its first index")
	i, err = chain.IndexOf(other)
	require.NoError(t, err)
	require.Equal(t, int64(1), i)
}
