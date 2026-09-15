// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database_test

import (
	"crypto/sha256"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// recordingStore records every key the outermost batch writes to the store.
type recordingStore struct {
	inner keyvalue.Beginner
	mu    sync.Mutex
	puts  map[string]int
}

type recordingChangeSet struct {
	keyvalue.ChangeSet
	r *recordingStore
}

func (r *recordingStore) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	return &recordingChangeSet{r.inner.Begin(prefix, writable), r}
}

func (r *recordingStore) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.puts = map[string]int{}
}

func (r *recordingStore) writes(key *record.Key) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.puts[key.String()]
}

func (s *recordingChangeSet) Put(key *record.Key, value []byte) error {
	s.r.mu.Lock()
	if s.r.puts == nil {
		s.r.puts = map[string]int{}
	}
	s.r.puts[key.String()]++
	s.r.mu.Unlock()
	return s.ChangeSet.Put(key, value)
}

func (s *recordingChangeSet) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	return &recordingChangeSet{s.ChangeSet.Begin(prefix, writable), s.r}
}

// Account.Chains is the index of an account's chains. It is written when a
// chain the index does not hold is appended to, and not otherwise: before
// #4244 every dirty account rewrote it every block (~270 per block, ~1 GB/h
// of dynamic-layer churn) to add chains it already listed.
func TestChainsIndexIsWrittenOnlyForNewChains(t *testing.T) {
	store := &recordingStore{inner: memory.New(nil)}
	db := database.New(store, nil)
	alice := url.MustParse("alice.acme")
	chainsKey := record.NewKey("Account", alice, "Chains")
	hash := func(i byte) []byte { h := sha256.Sum256([]byte{i}); return h[:] }

	// The account is created and its main chain appended to: the index is
	// written. (Update commits dirty records twice -- once for the BPT, once
	// for the batch -- so the count is "written", not "once".)
	require.NoError(t, db.Update(func(batch *database.Batch) error {
		a := batch.Account(alice)
		if err := a.Main().Put(&protocol.LiteIdentity{Url: alice}); err != nil {
			return err
		}
		return a.MainChain().Inner().AddEntry(hash(1), false)
	}))
	require.Positive(t, store.writes(chainsKey))

	// The same chain again: the index already lists it
	store.reset()
	require.NoError(t, db.Update(func(batch *database.Batch) error {
		return batch.Account(alice).MainChain().Inner().AddEntry(hash(2), false)
	}))
	require.Zero(t, store.writes(chainsKey), "no new chain, so no Chains record")
	require.Positive(t, store.writes(record.NewKey("Account", alice, "MainChain", "Head")), "the chain itself was written")

	// A chain the index does not hold: written, and both are listed
	store.reset()
	require.NoError(t, db.Update(func(batch *database.Batch) error {
		return batch.Account(alice).SignatureChain().Inner().AddEntry(hash(3), false)
	}))
	require.Positive(t, store.writes(chainsKey))
	require.NoError(t, db.View(func(batch *database.Batch) error {
		chains, err := batch.Account(alice).Chains().Get()
		require.NoError(t, err)
		var names []string
		for _, c := range chains {
			names = append(names, c.Name)
		}
		require.ElementsMatch(t, []string{"main", "signature"}, names)
		return nil
	}))
}
