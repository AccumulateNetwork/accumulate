// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// The stored-block answer (executor.md, Sync, "Two mismatches" 1; #4441): as
// of block B, the 256 positions eight levels below a prefix. These tests check
// it against a model of the tree built from the leaves alone, never against the
// BPT's own code, so a walk that is wrong in the same way on both sides cannot
// agree with itself.

// modelTree is the BPT's content as a map from key hash to leaf hash. The BPT
// is a binary trie on the key hash, most significant bit first, where a set bit
// goes left; a branch with one non-empty side carries that side's hash, and a
// branch with two combines them left then right. Every subtree's hash, and so
// every slot of every stored block, follows from the leaves by that rule alone.
type modelTree map[[32]byte][32]byte

func (m modelTree) clone() modelTree {
	n := make(modelTree, len(m))
	for k, v := range m {
		n[k] = v
	}
	return n
}

// under returns the keys whose first `bits` bits equal those of prefix.
func under(keys [][32]byte, prefix [32]byte, bits int) [][32]byte {
	var out [][32]byte
	for _, k := range keys {
		if sharesBits(k, prefix, bits) {
			out = append(out, k)
		}
	}
	return out
}

func sharesBits(a, b [32]byte, bits int) bool {
	for i := 0; i < bits; i++ {
		m := byte(0x80) >> (i & 7)
		if a[i>>3]&m != b[i>>3]&m {
			return false
		}
	}
	return true
}

// subtreeHash is the hash of the subtree holding exactly keys, which all share
// their first `bits` bits.
func (m modelTree) subtreeHash(keys [][32]byte, bits int) ([32]byte, bool) {
	switch len(keys) {
	case 0:
		return [32]byte{}, false
	case 1:
		return m[keys[0]], true
	}
	var left, right [][32]byte
	m8 := byte(0x80) >> (bits & 7)
	for _, k := range keys {
		if k[bits>>3]&m8 != 0 {
			left = append(left, k)
		} else {
			right = append(right, k)
		}
	}
	l, lok := m.subtreeHash(left, bits+1)
	r, rok := m.subtreeHash(right, bits+1)
	switch {
	case lok && rok:
		return sha256.Sum256(append(l[:], r[:]...)), true
	case lok:
		return l, true
	default:
		return r, rok
	}
}

// block is what the model says the stored block under prefix holds: one slot
// per non-empty position eight levels down, ascending.
func (m modelTree) block(prefix []byte) []BlockSlot {
	var all [][32]byte
	for k := range m {
		all = append(all, k)
	}
	var p [32]byte
	copy(p[:], prefix)
	bits := 8 * len(prefix)
	mine := under(all, p, bits)

	var slots []BlockSlot
	for i := 0; i < 256; i++ {
		p[len(prefix)] = byte(i)
		keys := under(mine, p, bits+8)
		if len(keys) == 0 {
			continue
		}
		h, _ := m.subtreeHash(keys, bits+8)
		s := BlockSlot{Index: byte(i), Hash: h}
		if len(keys) == 1 {
			s.KeyHash = keys[0]
		} else {
			s.Branch = true
		}
		slots = append(slots, s)
	}
	return slots
}

// root is the model's root hash.
func (m modelTree) root() [32]byte {
	var all [][32]byte
	for k := range m {
		all = append(all, k)
	}
	h, _ := m.subtreeHash(all, 0)
	return h
}

// modelFixture is a historyFixture that also keeps the model of every block.
type modelFixture struct {
	*historyFixture
	models map[uint64]modelTree
	roots  map[uint64][32]byte
	cur    modelTree
}

func newModelFixture(t *testing.T, accounts int, depth uint64) *modelFixture {
	f := &modelFixture{
		historyFixture: &historyFixture{t: t, db: memory.New(nil), depth: depth},
		models:         map[uint64]modelTree{},
		roots:          map[uint64][32]byte{},
		cur:            modelTree{},
	}
	f.block(1, func(put func(*record.Key)) {
		for i := 0; i < accounts; i++ {
			k := record.KeyFromHash(f.rh.NextA())
			f.keys = append(f.keys, k)
			put(k)
		}
	})
	return f
}

// block commits one block whose writes are named by fn; each write gives the
// key a fresh value, in the tree and in the model.
func (f *modelFixture) block(height uint64, fn func(put func(*record.Key))) {
	var writes []*record.Key
	fn(func(k *record.Key) { writes = append(writes, k) })
	root := f.commit(height, func(b *BPT) {
		for _, k := range writes {
			v := f.rh.NextA()
			require.NoError(f.t, b.Insert(k, v[:]))
			f.cur[k.Hash()] = v
		}
	})
	f.models[height] = f.cur.clone()
	f.roots[height] = root
	require.Equal(f.t, f.cur.root(), root, "the model disagrees with the BPT about block %d's root", height)
}

// branchPrefixes returns the prefixes of every stored block one level below the
// given answer.
func branchPrefixes(prefix []byte, b *StoredBlock) [][]byte {
	var out [][]byte
	for _, s := range b.Slots {
		if s.Branch {
			out = append(out, append(append([]byte{}, prefix...), s.Index))
		}
	}
	return out
}

// TestStoredBlockAsOfAnEarlierBlockIsThatBlocksTree is the first claim of
// #4441: the answer as of B equals the tree as of B, computed locally from the
// leaves, after later blocks have changed it.
func TestStoredBlockAsOfAnEarlierBlockIsThatBlocksTree(t *testing.T) {
	// 3000 accounts put ~12 under each root slot, so the root's slots are
	// branches and their blocks hold leaves and a few deeper branches.
	f := newModelFixture(t, 3000, 1000)

	const B = 3
	for h := uint64(2); h <= 6; h++ {
		h := h
		f.block(h, func(put func(*record.Key)) {
			// Change a spread of existing leaves and add new ones, so every
			// block after B moves the root and many of its stored blocks.
			for i := int(h); i < len(f.keys); i += 37 {
				put(f.keys[i])
			}
			for i := 0; i < 40; i++ {
				k := record.KeyFromHash(f.rh.NextA())
				f.keys = append(f.keys, k)
				put(k)
			}
		})
	}

	b := f.read()
	now, err := b.GetStoredBlock(nil)
	require.NoError(t, err)

	checked, differs := 0, 0
	queue := [][]byte{nil}
	for len(queue) > 0 {
		prefix := queue[0]
		queue = queue[1:]

		got, err := f.read().GetStoredBlockAt(B, f.roots[B], prefix)
		require.NoErrorf(t, err, "prefix %x", prefix)
		want := f.models[B].block(prefix)
		require.Equalf(t, want, got.Slots, "the answer for prefix %x as of block %d is not that block's tree", prefix, B)
		require.Truef(t, bytes.Equal(prefix, got.Prefix), "the answer does not name its prefix")
		require.Equalf(t, got.Hash, HashSlots(got.Slots), "prefix %x: the slots do not hash to the block's hash", prefix)
		checked++

		if len(prefix) == 0 {
			require.Equal(t, f.roots[B], got.Hash, "the root's block does not hash to block %d's root", B)
			require.NotEqual(t, now.Hash, got.Hash, "the tree did not move after block %d; the test proves nothing", B)
		}
		cur, err := f.read().GetStoredBlock(prefix)
		if err != nil || cur.Hash != got.Hash {
			differs++
		}
		queue = append(queue, branchPrefixes(prefix, got)...)
	}
	require.Greater(t, checked, 200, "only %d stored blocks were compared", checked)
	require.Greater(t, differs, 100, "only %d of %d stored blocks changed after block %d; the historical path is barely exercised", differs, checked, B)
	t.Logf("compared %d stored blocks as of block %d; %d have changed since", checked, B, differs)

	// And the current answer is the current model's.
	require.Equal(t, f.cur.block(nil), now.Slots)
}

// TestStoredBlockOutsideRetentionIsRefused is the second claim: a block the
// node keeps no history for is refused, never answered wrongly.
func TestStoredBlockOutsideRetentionIsRefused(t *testing.T) {
	const depth = 3
	f := newModelFixture(t, 2000, depth)
	for h := uint64(2); h <= 12; h++ {
		h := h
		f.block(h, func(put func(*record.Key)) {
			for i := int(h); i < len(f.keys); i += 11 {
				put(f.keys[i])
			}
		})
	}

	b := f.read()
	earliest, ok, err := b.EarliestRetained()
	require.NoError(t, err)
	require.True(t, ok)
	require.Greater(t, earliest, uint64(2), "the window never moved; the test proves nothing")

	// Inside the window: answered, and right.
	got, err := b.GetStoredBlockAt(earliest, f.roots[earliest], nil)
	require.NoError(t, err)
	require.Equal(t, f.models[earliest].block(nil), got.Slots)

	// Below it: refused as a capability limit, for every prefix, with the
	// window named.
	for h := uint64(1); h < earliest; h++ {
		for _, prefix := range [][]byte{nil, {f.keys[0].Hash()[0]}} {
			got, err := f.read().GetStoredBlockAt(h, f.roots[h], prefix)
			require.Errorf(t, err, "block %d, prefix %x, is below the retained window %d and was answered", h, prefix, earliest)
			require.Nil(t, got)
			require.Equalf(t, errors.IncompleteChain, errors.Code(err),
				"block %d: a block outside retention must be IncompleteChain, not %v (%v)", h, errors.Code(err), err)
			require.Contains(t, err.Error(), "retained")
		}
	}

	// A node that retains nothing refuses everything historical.
	g := newModelFixture(t, 200, 0)
	g.block(2, func(put func(*record.Key)) { put(g.keys[0]) })
	_, err = g.read().GetStoredBlockAt(1, g.roots[1], nil)
	require.Error(t, err)
	require.Equal(t, errors.IncompleteChain, errors.Code(err))
}

// TestAStoredBlockLocatesADifferingLeafInTwoAnswers is the third claim: a
// single leaf that differs between two trees is located to its 8-level branch
// by comparing two answers, the root's and the one under the root slot that
// differs.
func TestAStoredBlockLocatesADifferingLeafInTwoAnswers(t *testing.T) {
	// The peer: a tree answered as of block B after a later block changed it.
	peer := newModelFixture(t, 5000, 1000)
	const B = 2
	peer.block(B, func(put func(*record.Key)) { put(peer.keys[0]) })
	peer.block(3, func(put func(*record.Key)) {
		for i := 0; i < len(peer.keys); i += 7 {
			put(peer.keys[i])
		}
	})

	// The node: the same leaves as the peer at B, but one.
	node := &historyFixture{t: t, db: memory.New(nil)}
	var odd *record.Key
	for i := len(peer.keys) - 1; i >= 0; i-- {
		// A key whose root slot is a branch, so locating it takes two answers.
		k := peer.keys[i].Hash()
		n := 0
		for other := range peer.models[B] {
			if other[0] == k[0] {
				n++
			}
		}
		if n > 1 {
			odd = peer.keys[i]
			break
		}
	}
	require.NotNil(t, odd)
	node.commit(1, func(b *BPT) {
		for k, v := range peer.models[B] {
			v := v
			if k == odd.Hash() {
				v[0] ^= 0xff
			}
			require.NoError(t, b.Insert(record.KeyFromHash(k), v[:]))
		}
	})

	differing := func(a, b *StoredBlock) []BlockSlot {
		theirs := map[byte]BlockSlot{}
		for _, s := range b.Slots {
			theirs[s.Index] = s
		}
		var out []BlockSlot
		for _, s := range a.Slots {
			if theirs[s.Index] != s {
				out = append(out, s)
			}
			delete(theirs, s.Index)
		}
		for _, s := range theirs {
			out = append(out, s)
		}
		return out
	}

	// Answer one: the root.
	theirs, err := peer.read().GetStoredBlockAt(B, peer.roots[B], nil)
	require.NoError(t, err)
	mine, err := node.read().GetStoredBlock(nil)
	require.NoError(t, err)
	d := differing(theirs, mine)
	require.Len(t, d, 1, "one leaf differs, so one root slot must")
	require.Equal(t, odd.Hash()[0], d[0].Index)
	require.True(t, d[0].Branch, "the differing root slot is a branch; the next answer is the block below it")

	// Answer two: the block under the differing slot.
	prefix := []byte{d[0].Index}
	theirs, err = peer.read().GetStoredBlockAt(B, peer.roots[B], prefix)
	require.NoError(t, err)
	mine, err = node.read().GetStoredBlock(prefix)
	require.NoError(t, err)
	d = differing(theirs, mine)
	require.Len(t, d, 1, "one leaf differs, so one slot of the second answer must")
	require.Equal(t, odd.Hash()[1], d[0].Index, "the difference is not located to the leaf's 16-bit branch")
	oddHash := odd.Hash()
	t.Logf("located %x to branch %x in two answers", oddHash[:4], oddHash[:2])
}

// TestAStoredBlockMissingFromTheStoreIsRefused covers the fold check (#4441
// review F1). A stored block whose record is gone loads as an empty subtree;
// its positions then fold to nothing while the block above it records a hash
// for it. The answer must be refused, not served as an empty block that would
// send a joining node after every account under the prefix.
func TestAStoredBlockMissingFromTheStoreIsRefused(t *testing.T) {
	f := newModelFixture(t, 3000, 0)
	prefix := []byte{0x2b}

	// Undisturbed, the block is served and is the model's.
	got, err := f.read().GetStoredBlock(prefix)
	require.NoError(t, err)
	require.True(t, len(got.Slots) > 1, "prefix %x holds no stored block; the test proves nothing", prefix)
	require.Equal(t, f.cur.block(prefix), got.Slots)

	// Delete the block's record, leaving the root's block (which records the
	// block's hash) alone.
	var key [32]byte
	key[0] = prefix[0]
	nodeKey, ok := nodeKeyAt(8, key)
	require.True(t, ok)
	kvb := f.db.Begin(nil, true)
	require.NoError(t, kvb.Delete(new(ChangeSet).BPT().key.Append(nodeKey)))
	require.NoError(t, kvb.Commit())

	got, err = f.read().GetStoredBlock(prefix)
	require.Error(t, err, "a stored block missing from the store was served as %d positions", func() int {
		if got == nil {
			return 0
		}
		return len(got.Slots)
	}())
	require.Nil(t, got)
	require.Equalf(t, errors.InternalError, errors.Code(err), "%v", err)
	require.Contains(t, err.Error(), "but the block's hash is", "the refusal did not come from the fold check: %v", err)
}
