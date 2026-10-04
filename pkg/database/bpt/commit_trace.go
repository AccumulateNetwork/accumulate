// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

// BlockWrite is what one commit did to one stored block eight levels below the
// root: the hash the root's block records for it before the commit's first
// change under it and after the commit, and whether the commit wrote the
// block's own record. A block whose hash changed and whose record was not
// written -- or the reverse -- is the write that leaves the tree failing its
// own check (executor spec, "Sync", §3b).
type BlockWrite struct {
	Prefix  byte
	Before  [32]byte
	After   [32]byte
	Written bool
}

// commitTrace measures the stored blocks a commit changes, for the caller's
// write trail.
type commitTrace struct {
	fn     func([]BlockWrite)
	before map[byte][32]byte
}

// SetCommitTrace has every commit of this tree report the stored blocks eight
// levels below the root it changed. It is set on the tree that executes the
// commit -- a root batch's -- since a nested batch's commit only hands its
// changes up.
func (b *BPT) SetCommitTrace(fn func([]BlockWrite)) {
	if fn == nil {
		b.trace = nil
		return
	}
	b.trace = &commitTrace{fn: fn}
}

// blockUnder is the branch eight levels below the root on the way to key, or
// nil when there is none: fewer than two keys share the prefix.
func (b *BPT) blockUnder(prefix byte) *branch {
	var key [32]byte
	key[0] = prefix
	nodeKey, ok := nodeKeyAt(storedBlockLevels, key)
	if !ok {
		return nil
	}
	br, err := b.getRoot().getBranch(nodeKey)
	if err != nil {
		return nil
	}
	return br
}

// noteBefore records, once per commit, the hash the root's block holds for
// the stored block under the key's first byte, before a change is applied
// under it.
func (b *BPT) noteBefore(keyHash [32]byte) {
	t := b.trace
	if t == nil {
		return
	}
	p := keyHash[0]
	if _, ok := t.before[p]; ok {
		return
	}
	if t.before == nil {
		t.before = map[byte][32]byte{}
	}
	var h [32]byte
	if br := b.blockUnder(p); br != nil {
		h, _ = br.getHash()
	}
	t.before[p] = h
}

// measure is what the commit did to each stored block noted before it: called
// once the changes are applied and the hashes recomputed, before the records
// are written.
func (b *BPT) measure() []BlockWrite {
	t := b.trace
	if t == nil || len(t.before) == 0 {
		return nil
	}
	out := make([]BlockWrite, 0, len(t.before))
	for p, before := range t.before {
		w := BlockWrite{Prefix: p, Before: before}
		if br := b.blockUnder(p); br != nil {
			w.After, _ = br.getHash()
			w.Written = br.IsDirty()
		}
		out = append(out, w)
	}
	t.before = nil
	return out
}
