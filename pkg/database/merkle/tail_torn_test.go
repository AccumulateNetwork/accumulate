// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package merkle

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// frozenStore serves a chosen set of keys as they were when frozen, and
// everything else live: a batch on a store that gives it no snapshot (the
// memory store) can read the head from before a commit and a tail chunk
// from after it, or the reverse, and this is that batch.
type frozenStore struct {
	keyvalue.ChangeSet
	frozen map[[32]byte][]byte
}

func (s *frozenStore) Get(key *record.Key) ([]byte, error) {
	if v, ok := s.frozen[key.Hash()]; ok {
		return v, nil
	}
	return s.ChangeSet.Get(key)
}

func freeze(t *testing.T, kv keyvalue.ChangeSet, keys ...*record.Key) *frozenStore {
	f := &frozenStore{ChangeSet: kv, frozen: map[[32]byte][]byte{}}
	for _, k := range keys {
		v, err := kv.Get(k)
		require.NoError(t, err)
		f.frozen[k.Hash()] = v
	}
	return f
}

// A torn read -- the head and a tail chunk from either side of a commit --
// does not fail an append: the head is the authority and the elements are
// the record, so a chunk that runs past the head is cut back to it and one
// that falls short is refilled from the elements. Validation batches on the
// memory store read such pairs, and are discarded; before this they were
// refused.
func TestTailToleratesATornRead(t *testing.T) {
	var rh common.RandHash
	kv := memory.New(nil).Begin(nil, true)
	key := record.NewKey("try")
	m := testChain(keyvalue.RecordStore{Store: kv}, 4, "try")
	for i := 0; i < 3; i++ {
		require.NoError(t, m.AddEntry(rh.NextList(), false))
	}
	require.NoError(t, m.Commit())
	chunk0 := key.Append("Tail", uint64(0))
	head := key.Append("Head")

	// Chunk from before the commit, head from after: the chunk is behind
	behind := freeze(t, kv, chunk0)
	// Head from before the commit, chunk from after: the chunk is ahead
	ahead := freeze(t, kv, head)

	m = testChain(keyvalue.RecordStore{Store: kv}, 4, "try")
	for i := 3; i < 5; i++ {
		require.NoError(t, m.AddEntry(rh.NextList(), false))
	}
	require.NoError(t, m.Commit())

	// Behind: head says 5, chunk holds 3; entries 3 and 4 come from Element
	next := rh.NextList()
	b := testChain(keyvalue.RecordStore{Store: behind}, 4, "try")
	require.NoError(t, b.AddEntry(next, false))
	c0, err := b.Tail(0).Get()
	require.NoError(t, err)
	require.Equal(t, rh.List[:6], c0.Hashes)
	st, err := b.StateAt(5)
	require.NoError(t, err)
	want := new(State)
	for _, h := range rh.List[:6] {
		want.AddEntry(h)
	}
	require.Equal(t, want.Anchor(), st.Anchor())

	// Ahead: head says 3, chunk holds 5; the chunk is cut back to the head
	other := rh.NextList()
	a := testChain(keyvalue.RecordStore{Store: ahead}, 4, "try")
	require.NoError(t, a.AddEntry(other, false))
	c0, err = a.Tail(0).Get()
	require.NoError(t, err)
	require.Equal(t, append(append([][]byte{}, rh.List[:3]...), other), c0.Hashes)
	h, err := a.Entry(3)
	require.NoError(t, err)
	require.Equal(t, other, h)
}
