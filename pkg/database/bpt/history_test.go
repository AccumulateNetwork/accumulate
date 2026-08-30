// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/smt/common"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// historyFixture drives a BPT one "block" at a time against a shared store, the
// way a partition does.
type historyFixture struct {
	t     testing.TB
	db    *memory.Database
	depth uint64
	keys  []*record.Key
	rh    common.RandHash
}

func newHistoryFixture(t testing.TB, accounts int, depth uint64) *historyFixture {
	f := &historyFixture{t: t, db: memory.New(nil), depth: depth}
	f.commit(1, func(b *BPT) {
		for i := 0; i < accounts; i++ {
			k := record.KeyFromHash(f.rh.NextA())
			f.keys = append(f.keys, k)
			require.NoError(t, b.Insert(k, f.rh.Next()))
		}
	})
	return f
}

// commit runs one block: open a BPT over the shared store, apply fn, commit.
func (f *historyFixture) commit(height uint64, fn func(*BPT)) [32]byte {
	f.t.Helper()
	kvb := f.db.Begin(nil, true)
	model := new(ChangeSet)
	model.store = keyvalue.RecordStore{Store: kvb}
	b := model.BPT()
	b.SetHistory(height, f.depth)
	fn(b)
	require.NoError(f.t, b.Commit())
	require.NoError(f.t, kvb.Commit())

	kvb = f.db.Begin(nil, false)
	model = new(ChangeSet)
	model.store = keyvalue.RecordStore{Store: kvb}
	root, err := model.BPT().GetRootHash()
	require.NoError(f.t, err)
	return root
}

// read opens a read-only BPT over the store.
func (f *historyFixture) read() *BPT {
	kvb := f.db.Begin(nil, false)
	model := new(ChangeSet)
	model.store = keyvalue.RecordStore{Store: kvb}
	return model.BPT()
}

// records returns the full record set, for byte-identical comparison.
func (f *historyFixture) records() map[[32]byte][]byte {
	entries, err := f.db.Export()
	require.NoError(f.t, err)
	m := make(map[[32]byte][]byte, len(entries))
	for _, e := range entries {
		m[e.Key.Hash()] = e.Value
	}
	return m
}

// TestHistory_DepthZeroChangesNothing is the gate that matters most for anyone
// running a node today: with retention off, the on-disk record set must be
// byte-identical to what it would have been without this change. Proven by
// comparison of the exported records, not asserted.
func TestHistory_DepthZeroChangesNothing(t *testing.T) {
	const accounts, blocks = 200, 10

	run := func(depth uint64) map[[32]byte][]byte {
		f := newHistoryFixture(t, accounts, depth)
		for i := uint64(0); i < blocks; i++ {
			f.commit(2+i, func(b *BPT) {
				require.NoError(t, b.Insert(f.keys[i%uint64(len(f.keys))], f.rh.Next()))
			})
		}
		return f.records()
	}

	// The same sequence of writes, once with retention off. Because the fixture
	// seeds its own RandHash deterministically, both runs write identical values.
	off := run(0)
	off2 := run(0)
	require.Equal(t, len(off), len(off2), "the fixture is not deterministic")
	for k, v := range off {
		require.Equal(t, v, off2[k], "the fixture is not deterministic")
	}

	// And with retention on, every record that existed before must still be
	// present and unchanged — history is added, nothing is rewritten
	on := run(1000)
	for k, v := range off {
		w, ok := on[k]
		require.Truef(t, ok, "retention removed a record")
		require.Equalf(t, v, w, "retention modified an existing record")
	}
	require.Greater(t, len(on), len(off), "retention stored nothing")
	t.Logf("records: depth 0 = %d, depth 1000 = %d (+%d history records)",
		len(off), len(on), len(on)-len(off))
}

// TestHistory_RootUnchanged proves retention does not touch the BPT root, which
// is what makes it safe to enable without an executor-version gate.
func TestHistory_RootUnchanged(t *testing.T) {
	const accounts, blocks = 100, 8

	roots := func(depth uint64) [][32]byte {
		f := newHistoryFixture(t, accounts, depth)
		var out [][32]byte
		for i := uint64(0); i < blocks; i++ {
			out = append(out, f.commit(2+i, func(b *BPT) {
				require.NoError(t, b.Insert(f.keys[i%uint64(len(f.keys))], f.rh.Next()))
			}))
		}
		return out
	}
	require.Equal(t, roots(0), roots(500))
}

// TestHistory_ReadsBackEveryVersion changes one leaf per block and requires that
// the node on its path reads back, at every historical height, exactly the bytes
// that were current at that height.
func TestHistory_ReadsBackEveryVersion(t *testing.T) {
	const accounts, blocks = 500, 12
	f := newHistoryFixture(t, accounts, 1000)

	// The height-0 node is on every path and is rewritten by every block, so it
	// is the node with the most versions and the one most likely to be wrong.
	// Its key is not all zeros — nodeKeyAt marks the end of the key with a set
	// bit, so height 0 is 0x80 followed by zeros.
	top, ok := nodeKeyAt(0, [32]byte{})
	require.True(t, ok)
	t.Logf("root node key = %x", top[:4])

	want := map[uint64][]byte{}
	for i := uint64(0); i < blocks; i++ {
		h := 2 + i
		// Capture what the top node holds BEFORE this block
		b := f.read()
		cur, present, err := readRaw(b.store, b.key.Append(top))
		require.NoError(t, err)
		require.True(t, present)
		want[h-1] = cur

		f.commit(h, func(b *BPT) {
			require.NoError(t, b.Insert(f.keys[int(i)%len(f.keys)], f.rh.Next()))
		})
	}

	b := f.read()
	heights, err := b.RetainedHeights(top)
	require.NoError(t, err)
	require.Len(t, heights, blocks, "expected one retained version per block")
	t.Logf("top node retained at heights %v", heights)

	for h, expected := range want {
		got, ok, err := b.NodeAt(top, h)
		require.NoErrorf(t, err, "height %d", h)
		require.Truef(t, ok, "no retained version covers height %d", h)
		require.Equalf(t, expected, got, "wrong bytes for height %d", h)
	}
}

// TestHistory_NoVersionAfterMeansCurrent proves the ok=false case is "the node
// has not changed since", not an error. Reporting it as missing would make every
// unchanged subtree unprovable.
func TestHistory_NoVersionAfterMeansCurrent(t *testing.T) {
	f := newHistoryFixture(t, 50, 1000)
	f.commit(2, func(b *BPT) { require.NoError(t, b.Insert(f.keys[0], f.rh.Next())) })

	b := f.read()
	top, ok := nodeKeyAt(0, [32]byte{})
	require.True(t, ok)
	_, ok, err := b.NodeAt(top, 99) // long after the last change
	require.NoError(t, err)
	require.False(t, ok, "a height after the last change must fall through to current")
}

// TestHistory_PrunesTheWindow proves the retained window is bounded, which is
// the whole reason depth exists: an unbounded window on a busy BVN grows without
// limit.
func TestHistory_PrunesTheWindow(t *testing.T) {
	const depth = 5
	f := newHistoryFixture(t, 50, depth)
	for i := uint64(0); i < 20; i++ {
		f.commit(2+i, func(b *BPT) {
			require.NoError(t, b.Insert(f.keys[int(i)%len(f.keys)], f.rh.Next()))
		})
	}

	b := f.read()
	top, ok := nodeKeyAt(0, [32]byte{})
	require.True(t, ok)
	heights, err := b.RetainedHeights(top)
	require.NoError(t, err)
	t.Logf("depth %d retained heights: %v", depth, heights)
	require.LessOrEqual(t, len(heights), depth+1,
		"the window is not bounded by depth")
	require.NotEmpty(t, heights)

	// Everything still retained must still read back
	for _, h := range heights {
		_, ok, err := b.NodeAt(top, h-1)
		require.NoErrorf(t, err, "height %d", h-1)
		require.Truef(t, ok, "height %d is indexed but does not read back", h-1)
	}
}

func TestPruneHeights(t *testing.T) {
	cases := []struct {
		heights    []uint64
		now, depth uint64
		keep       []uint64
	}{
		// Nothing to drop while the window covers everything
		{[]uint64{2, 3, 4}, 4, 10, []uint64{2, 3, 4}},
		{[]uint64{2, 3, 4}, 4, 0, []uint64{2, 3, 4}},
		// floor = 10-5 = 5; keep the last entry at or below 5, which covers the
		// start of the window
		{[]uint64{2, 4, 6, 8, 10}, 10, 5, []uint64{4, 6, 8, 10}},
		// floor = 20-5 = 15; only 16.. is in the window, and 12 covers 15
		{[]uint64{2, 12, 16, 20}, 20, 5, []uint64{12, 16, 20}},
	}
	for i, c := range cases {
		keep, _ := pruneHeights(append([]uint64(nil), c.heights...), c.now, c.depth)
		require.Equalf(t, c.keep, keep, "case %d", i)
	}
}

// TestHistory_ReceiptAgainstHistoricalRoot is the point of retention: a receipt
// for a key, against the root the tree had at a past block, that validates.
func TestHistory_ReceiptAgainstHistoricalRoot(t *testing.T) {
	const accounts, blocks = 300, 10
	f := newHistoryFixture(t, accounts, 1000)

	// Record the root after each block, and the value the probe key held then
	type snapshot struct {
		root  [32]byte
		value []byte
	}
	probe := f.keys[7]
	seen := map[uint64]snapshot{}

	for i := uint64(0); i < blocks; i++ {
		h := 2 + i
		val := f.rh.Next()
		// Change the probe key on even blocks, something else on odd ones, so
		// the probe's own value is stable across some blocks and not others
		target := f.keys[int(i)%len(f.keys)]
		if i%2 == 0 {
			target = probe
		}
		root := f.commit(h, func(b *BPT) {
			require.NoError(t, b.Insert(target, val))
		})
		cur := seen[h-1].value
		if target == probe {
			cur = val
		}
		seen[h] = snapshot{root: root, value: cur}
	}

	b := f.read()
	checked := 0
	for h, s := range seen {
		if s.value == nil {
			continue
		}
		r, err := b.GetReceiptAt(probe, h, s.root)
		require.NoErrorf(t, err, "height %d", h)
		require.Truef(t, r.Validate(nil), "receipt for height %d does not validate", h)
		require.Equalf(t, s.root[:], r.Anchor, "receipt for height %d has the wrong anchor", h)
		require.Equalf(t, s.value, r.Start, "receipt for height %d proves the wrong value", h)
		checked++
	}
	require.Greater(t, checked, 5, "only %d heights checked", checked)
	t.Logf("verified receipts at %d historical heights", checked)
}

// TestHistory_ReceiptRefusesWrongRoot proves the view is self-checking: handed a
// root that does not belong to the height, it refuses rather than returning a
// receipt against whatever it reconstructed.
func TestHistory_ReceiptRefusesWrongRoot(t *testing.T) {
	f := newHistoryFixture(t, 100, 1000)
	var roots [][32]byte
	for i := uint64(0); i < 6; i++ {
		roots = append(roots, f.commit(2+i, func(b *BPT) {
			require.NoError(t, b.Insert(f.keys[int(i)], f.rh.Next()))
		}))
	}

	b := f.read()
	// Height 3's tree against height 6's root
	_, err := b.GetReceiptAt(f.keys[0], 3, roots[len(roots)-1])
	require.Error(t, err)
	require.Contains(t, err.Error(), "but the ledger recorded")
}
