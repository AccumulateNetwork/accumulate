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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// The persisted head is Count and Pending. It used to carry the current
// mark set too -- up to markFreq hashes, rewritten on every append -- which
// was 85% of the bytes written to the dynamic layer (#4234). The mark set
// lives in the Tail records now, so the head's size follows log(Count), not
// how far into the mark set the chain is.
func TestHeadSizeIsIndependentOfTheMarkSetFill(t *testing.T) {
	var rh common.RandHash
	store := begin()
	m := testChain(store, 8, "try") // 256 hashes per mark set

	const n = 300
	var largest int
	for i := 0; i < n; i++ {
		require.NoError(t, m.AddEntry(rh.NextList(), false))
		head, err := m.Head().Get()
		require.NoError(t, err)
		require.Empty(t, head.HashList, "the head carries no hash list")
		b, err := head.MarshalBinary()
		require.NoError(t, err)
		if len(b) > largest {
			largest = len(b)
		}
	}
	// Pending is at most one hash per set bit of Count (9 bits for 300), so
	// the head is a few hundred bytes; 255 hashes of mark set would be 8 KB.
	require.Less(t, largest, 400, "head size (bytes)")

	// The mark point closed at 255 still holds the whole set, as before
	mp, err := m.States(255).Get()
	require.NoError(t, err)
	require.Len(t, mp.HashList, 256)
	require.Equal(t, rh.List[:256], mp.HashList)
}

// The Tail records serve every reader that used to read the head's hash
// list, without touching Element: the element within the open mark set, the
// state at any index, and a range that runs into the tail. Element is
// deleted here so a read that fell through to it would fail.
func TestTailServesTheOpenMarkSet(t *testing.T) {
	var rh common.RandHash
	kv := memory.New(nil).Begin(nil, true)
	store := keyvalue.RecordStore{Store: kv}
	m := testChain(store, 4, "try") // 16 hashes per mark set, chunks of 8

	const n = 37 // two full mark sets and five in the open one
	var states []*State
	for i := 0; i < n; i++ {
		require.NoError(t, m.AddEntry(rh.NextList(), false))
		head, err := m.Head().Get()
		require.NoError(t, err)
		states = append(states, head.Copy())
	}
	require.NoError(t, m.Commit())
	for i := 0; i < n; i++ {
		require.NoError(t, kv.Delete(m.key.Append("Element", uint64(i))))
	}
	m = testChain(store, 4, "try") // nothing cached from the appends

	for i := int64(0); i < n; i++ {
		h, err := m.Entry(i)
		require.NoError(t, err, "entry %d", i)
		require.Equal(t, rh.List[i], h, "entry %d", i)

		st, err := m.StateAt(i)
		require.NoError(t, err, "state at %d", i)
		require.Equal(t, states[i].Count, st.Count)
		require.Equal(t, states[i].Anchor(), st.Anchor(), "state at %d", i)
	}
	hashes, err := m.Entries(30, n)
	require.NoError(t, err)
	require.Equal(t, rh.List[30:n], hashes)

	// The chunks are what the tail is: 5 hashes in the open set -> one chunk
	c0, err := m.Tail(0).Get()
	require.NoError(t, err)
	require.Equal(t, uint64(32), c0.Index)
	require.Len(t, c0.Hashes, 5)
	c1, err := m.Tail(1).Get()
	require.NoError(t, err)
	require.NotEqual(t, uint64(40), c1.Index, "chunk 1 still holds the previous set's hashes; it is stale by its index")
}

// A chain written before #4234 carries its open mark set in the head. Its
// first append moves the set into the Tail records, so the mark point that
// closes the set is whole and every reader keeps working.
func TestLegacyHeadIsMigratedOnAppend(t *testing.T) {
	var rh common.RandHash
	store := begin()
	m := testChain(store, 3, "legacy") // 8 per mark set

	// Written the old way: a mark point, then a head that carries the hashes
	// since it, and every element.
	st := new(State)
	for st.Count < 8 {
		st.AddEntry(rh.NextList())
	}
	require.NoError(t, m.States(7).Put(st.Copy()))
	st.HashList = st.HashList[:0]
	for st.Count < 13 {
		st.AddEntry(rh.NextList())
	}
	require.NoError(t, m.Head().Put(st.Copy()))
	for i, h := range rh.List {
		require.NoError(t, m.Element(uint64(i)).Put(h))
		require.NoError(t, m.ElementIndex(h).Put(uint64(i)))
	}

	// Readers work on the legacy head as they always did
	h, err := m.Entry(10)
	require.NoError(t, err)
	require.Equal(t, rh.List[10], h)

	// Append through the mark point
	for st.Count < 20 {
		require.NoError(t, m.AddEntry(rh.NextList(), false))
		st.AddEntry(rh.List[len(rh.List)-1])
	}
	head, err := m.Head().Get()
	require.NoError(t, err)
	require.Empty(t, head.HashList, "migrated: the head no longer carries the set")
	require.Equal(t, st.Anchor(), head.Anchor())

	mp, err := m.States(15).Get()
	require.NoError(t, err)
	require.Equal(t, rh.List[8:16], mp.HashList, "the mark point holds the hashes from the legacy head and the new ones")

	for i := int64(0); i < 20; i++ {
		h, err := m.Entry(i)
		require.NoError(t, err, "entry %d", i)
		require.Equal(t, rh.List[i], h)
	}
	hashes, err := m.Entries(5, 20)
	require.NoError(t, err)
	require.Equal(t, rh.List[5:20], hashes)
}

// A tail chunk whose index says it belongs to this mark set but whose length
// does not is corruption, and the append says so rather than closing a mark
// point with the wrong hashes.
func TestInconsistentTailChunkIsAnError(t *testing.T) {
	var rh common.RandHash
	store := begin()
	m := testChain(store, 4, "try")
	for i := 0; i < 3; i++ {
		require.NoError(t, m.AddEntry(rh.NextList(), false))
	}
	c0, err := m.Tail(0).Get()
	require.NoError(t, err)
	c0.Hashes = c0.Hashes[:1]
	require.NoError(t, m.Tail(0).Put(c0))

	err = m.AddEntry(rh.NextList(), false)
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.InvalidRecord), "got %v", err)
}
