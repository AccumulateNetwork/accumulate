// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/values"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// countingStore counts every read the batches make of the store below them.
type countingStore struct {
	inner keyvalue.Beginner
	gets  atomic.Int64
	mu    sync.Mutex
	keys  []string
}

type countingChangeSet struct {
	keyvalue.ChangeSet
	c *countingStore
}

func (c *countingStore) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	return &countingChangeSet{c.inner.Begin(prefix, writable), c}
}

func (s *countingChangeSet) Get(key *record.Key) ([]byte, error) {
	s.c.gets.Add(1)
	s.c.mu.Lock()
	s.c.keys = append(s.c.keys, key.String())
	s.c.mu.Unlock()
	return s.ChangeSet.Get(key)
}

func (s *countingChangeSet) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	return &countingChangeSet{s.ChangeSet.Begin(prefix, writable), s.c}
}

// A first write does not read the store to learn the key is absent (database
// spec, "Duplicates are caught at entry"): the version it needs is the parent
// batch's bookkeeping, and the outermost batch has no parent.
func TestPut_FirstWriteReadsNothing(t *testing.T) {
	store := &countingStore{inner: memory.New(nil)}
	db := database.New(store, nil)
	batch := db.Begin(true)
	defer batch.Discard()

	foo := protocol.AccountUrl("foo")
	require.NoError(t, batch.Account(foo).Main().Put(&protocol.UnknownSigner{Url: foo, Version: 1}))
	require.NoError(t, batch.Account(foo).MainChain().Inner().Element(0).Put([]byte{1}))
	require.Zero(t, store.gets.Load(), "two first writes, no reads")

	// A set is read to be merged into, which is not an existence check: one
	// read, of a mutable record
	require.NoError(t, batch.Account(foo).Pending().Add(foo.WithTxID([32]byte{1})))
	require.Equal(t, int64(1), store.gets.Load(), "the merge reads the set once")

	// A child's first write asks its parent for the version, in memory
	child := batch.Begin(true)
	require.NoError(t, child.Account(foo).Main().Put(&protocol.UnknownSigner{Url: foo, Version: 2}))
	require.Equal(t, int64(1), store.gets.Load(), "the child's first write read nothing: %v", store.keys)
	// Committing the child merges the account's chain set: one more merge
	// read, not an existence check
	require.NoError(t, child.Commit())
	require.Equal(t, int64(2), store.gets.Load(), "reads: %v", store.keys)
}

// The conflict check between concurrent children still works without the
// read: a child writing a key its parent wrote earlier in the block commits
// (this is the case a naive skip would break, raising a spurious conflict
// that poisons the block), siblings writing the same key conflict, and the
// version carries through three levels.
func TestVersion_WithoutTheRead(t *testing.T) {
	foo := protocol.AccountUrl("foo")
	signer := func(v uint64) *protocol.UnknownSigner { return &protocol.UnknownSigner{Url: foo, Version: v} }
	get := func(t *testing.T, batch *database.Batch) uint64 {
		var a *protocol.UnknownSigner
		require.NoError(t, batch.Account(foo).Main().GetAs(&a))
		return a.Version
	}

	t.Run("child after parent", func(t *testing.T) {
		db := database.OpenInMemory(nil)
		root := db.Begin(true)
		defer root.Discard()
		require.NoError(t, root.Account(foo).Main().Put(signer(1)))
		child := root.Begin(true)
		require.NoError(t, child.Account(foo).Main().Put(signer(2)), "blind put, no read")
		require.NoError(t, child.Commit())
		require.Equal(t, uint64(2), get(t, root))
	})

	t.Run("siblings conflict", func(t *testing.T) {
		db := database.OpenInMemory(nil)
		root := db.Begin(true)
		defer root.Discard()
		a, b := root.Begin(true), root.Begin(true)
		require.NoError(t, a.Account(foo).Main().Put(signer(1)))
		require.NoError(t, b.Account(foo).Main().Put(signer(2)))
		require.NoError(t, a.Commit())
		err := b.Commit()
		require.Error(t, err)
		require.ErrorIs(t, err, errors.Conflict)
	})

	t.Run("three levels", func(t *testing.T) {
		db := database.OpenInMemory(nil)
		root := db.Begin(true)
		defer root.Discard()
		require.NoError(t, root.Account(foo).Main().Put(signer(1)))
		child := root.Begin(true)
		require.NoError(t, child.Account(foo).Main().Put(signer(2)))
		grand := child.Begin(true)
		require.NoError(t, grand.Account(foo).Main().Put(signer(3)))
		require.NoError(t, grand.Commit())
		require.NoError(t, child.Commit())
		require.Equal(t, uint64(3), get(t, root))
	})

	t.Run("shards after parent", func(t *testing.T) {
		db := database.OpenInMemory(nil)
		root := db.Begin(true)
		defer root.Discard()
		bar := protocol.AccountUrl("bar")
		require.NoError(t, root.Account(foo).Main().Put(signer(1)))
		var mu sync.Mutex
		s1, s2 := root.BeginConcurrent(&mu, true), root.BeginConcurrent(&mu, true)
		require.NoError(t, s1.Account(foo).Main().Put(signer(2)))
		require.NoError(t, s2.Account(bar).Main().Put(&protocol.UnknownSigner{Url: bar, Version: 9}))
		require.NoError(t, s1.Commit())
		require.NoError(t, s2.Commit())
		require.Equal(t, uint64(2), get(t, root))
	})
}

// The two write paths — read before a first write, and version-only — commit
// byte-identical state over a workload of accounts, chains, sets, child
// batches and concurrent shards, and the version-only path reads less.
func TestPut_DifferentialAgainstTheRead(t *testing.T) {
	run := func(readBeforeWrite bool) (map[[32]byte][]byte, [32]byte, int64) {
		values.ReadBeforeWrite = readBeforeWrite
		defer func() { values.ReadBeforeWrite = false }()

		store := &countingStore{inner: memory.New(nil)}
		db := database.New(store, nil)
		batch := db.Begin(true)

		accounts := []*url.URL{protocol.AccountUrl("a"), protocol.AccountUrl("b"), protocol.AccountUrl("c")}
		for i, u := range accounts {
			require.NoError(t, batch.Account(u).Main().Put(&protocol.UnknownSigner{Url: u, Version: uint64(i)}))
			for j := 0; j < 5; j++ {
				require.NoError(t, batch.Account(u).MainChain().Inner().AddEntry([]byte{byte(i), byte(j), 7, 7}, false))
			}
			require.NoError(t, batch.Account(u).Pending().Add(u.WithTxID([32]byte{byte(i)})))
		}

		// A child batch that rewrites an account and adds to its chain
		child := batch.Begin(true)
		require.NoError(t, child.Account(accounts[0]).Main().Put(&protocol.UnknownSigner{Url: accounts[0], Version: 10}))
		require.NoError(t, child.Account(accounts[0]).MainChain().Inner().AddEntry([]byte{9, 9, 9, 9}, false))
		require.NoError(t, child.Commit())

		// Two shards writing disjoint accounts, committed in order
		var mu sync.Mutex
		s1, s2 := batch.BeginConcurrent(&mu, true), batch.BeginConcurrent(&mu, true)
		require.NoError(t, s1.Account(accounts[1]).Main().Put(&protocol.UnknownSigner{Url: accounts[1], Version: 11}))
		require.NoError(t, s1.Account(accounts[1]).MainChain().Inner().AddEntry([]byte{8, 8, 8, 8}, false))
		require.NoError(t, s2.Account(accounts[2]).Pending().Add(accounts[2].WithTxID([32]byte{5})))
		require.NoError(t, s1.Commit())
		require.NoError(t, s2.Commit())

		require.NoError(t, batch.Commit())

		out := map[[32]byte][]byte{}
		view := store.inner.Begin(nil, false)
		require.NoError(t, view.ForEach(func(k *record.Key, v []byte) error {
			out[k.Hash()] = append([]byte(nil), v...)
			return nil
		}))
		view.Discard()
		root := db.Begin(false)
		defer root.Discard()
		hash, err := root.GetBptRootHash()
		require.NoError(t, err)
		return out, hash, store.gets.Load()
	}

	oldEntries, oldRoot, oldReads := run(true)
	newEntries, newRoot, newReads := run(false)
	require.Equal(t, oldRoot, newRoot, "state root")
	require.Equal(t, len(oldEntries), len(newEntries), "committed keys")
	for k, v := range oldEntries {
		require.Equal(t, v, newEntries[k], "committed bytes for %x", k[:4])
	}
	require.Less(t, newReads, oldReads, "the version-only path reads less (%d vs %d)", newReads, oldReads)
}
