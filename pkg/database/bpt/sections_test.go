// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// Sections (executor.md, "Sync", §3a; #4485) are checked against the model of
// the tree built from its leaves alone (modelTree), never against the BPT's
// own code.

// sectionsOfModel is every section's hash as the model says it.
func sectionsOfModel(m modelTree) *SectionHashes {
	bySection := map[Section][]SectionLeaf{}
	for k, v := range m {
		bySection[SectionOf(k)] = append(bySection[SectionOf(k)], SectionLeaf{KeyHash: k, Hash: v})
	}
	out := new(SectionHashes)
	for s, leaves := range bySection {
		var keys [][32]byte
		for _, l := range leaves {
			keys = append(keys, l.KeyHash)
		}
		out.Hash[s], out.Present[s] = m.subtreeHash(keys, SectionBits)
	}
	return out
}

// topOf reads a tree's root block and the block under each of its branches.
func topOf(t *testing.T, b *BPT) ([32]byte, []BlockSlot, map[byte][]BlockSlot) {
	root, err := b.GetRootHash()
	require.NoError(t, err)
	top, err := b.GetStoredBlock(nil)
	require.NoError(t, err)
	below := map[byte][]BlockSlot{}
	for _, s := range top.Slots {
		if !s.Branch {
			continue
		}
		sub, err := b.GetStoredBlock([]byte{s.Index})
		require.NoError(t, err)
		below[s.Index] = sub.Slots
	}
	return root, top.Slots, below
}

func TestSectionHashesAreTheModelsAndCombineToTheRoot(t *testing.T) {
	// Sizes on both sides of one leaf per section: a sparse tree's leaves sit
	// above depth 16, a dense one's below it.
	for _, n := range []int{1, 3, 300, 3000, 20000} {
		f := newModelFixture(t, n, 0)
		model := f.cur
		want := sectionsOfModel(model)

		require.Equal(t, model.root(), want.Root(), "%d leaves: the model's sections do not combine to its root", n)

		for s := range want.Hash {
			var leaves []SectionLeaf
			for k, v := range model {
				if SectionOf(k) == Section(s) {
					leaves = append(leaves, SectionLeaf{KeyHash: k, Hash: v})
				}
			}
			h, ok := SectionHash(leaves)
			require.Equal(t, want.Present[s], ok)
			require.Equal(t, want.Hash[s], h, "%d leaves: section %d", n, s)
		}

		root, top, below := topOf(t, f.read())
		require.Equal(t, model.root(), root)
		got, err := SectionHashesFromBlocks(root, top, below)
		require.NoError(t, err)
		require.Equal(t, want.Present, got.Present, "%d leaves: the stored blocks name other sections", n)
		require.Equal(t, want.Hash, got.Hash, "%d leaves: the stored blocks give other section hashes", n)
	}
}

func TestAMarkerInPlaceOfASectionKeepsTheRoot(t *testing.T) {
	f := newModelFixture(t, 3000, 0)
	want := sectionsOfModel(f.cur)
	before := f.cur.root()

	// Every third non-empty section is replaced by its marker: its leaves
	// deleted, the marker inserted with the section's hash.
	replaced := 0
	f.commit(2, func(b *BPT) {
		for s := 0; s < Sections; s += 3 {
			if !want.Present[s] {
				continue
			}
			replaced++
			for k := range f.cur {
				if SectionOf(k) == Section(s) {
					require.NoError(t, b.Delete(record.KeyFromHash(k)))
				}
			}
			h := want.Hash[s]
			require.NoError(t, b.Insert(Section(s).MarkerKey(), h[:]))
		}
	})
	require.NotZero(t, replaced)

	b := f.read()
	root, err := b.GetRootHash()
	require.NoError(t, err)
	require.Equal(t, before, root, "the root with markers is not the root with the sections' leaves")

	// The top read from the tree with markers is the same top.
	r, top, below := topOf(t, b)
	got, err := SectionHashesFromBlocks(r, top, below)
	require.NoError(t, err)
	require.Equal(t, want.Hash, got.Hash)
	require.Equal(t, want.Present, got.Present)

	// And each marker is found where it was put, with the section's hash.
	for s := 0; s < Sections; s += 3 {
		v, err := b.Get(Section(s).MarkerKey())
		if !want.Present[s] {
			require.ErrorIs(t, err, errors.NotFound)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, want.Hash[s][:], v)
	}
}

func TestATopWhoseBlocksDoNotAgreeIsRefused(t *testing.T) {
	f := newModelFixture(t, 3000, 0)
	root, top, below := topOf(t, f.read())

	// A block below the root that is not the one the root's block names.
	for i := range below {
		forged := append([]BlockSlot(nil), below[i]...)
		forged[0].Hash[0] ^= 1
		below[i] = forged
		break
	}
	_, err := SectionHashesFromBlocks(root, top, below)
	require.ErrorIs(t, err, errors.Conflict)

	// A root's block that does not hash to the root.
	root2, top2, below2 := topOf(t, f.read())
	root2[0] ^= 1
	_, err = SectionHashesFromBlocks(root2, top2, below2)
	require.ErrorIs(t, err, errors.Conflict)
}

func TestAMarkerIsNoAccountsKeyAndItsScanStartsInItsSection(t *testing.T) {
	f := newModelFixture(t, 2000, 0)
	for k := range f.cur {
		require.False(t, IsSectionMarker(k), "an account's key hash read as a marker")
	}
	for s := 0; s < Sections; s += 97 {
		sec := Section(s)
		require.True(t, IsSectionMarker(sec.Marker()))
		require.Equal(t, sec, SectionOf(sec.Marker()))
		require.Equal(t, sec, SectionOf(sec.ScanStart()))
		require.Equal(t, sec.Marker(), [32]byte(sec.MarkerKey().Hash()))
	}
}
