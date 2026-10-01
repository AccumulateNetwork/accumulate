// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bpt

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// A joining node takes the tree by its top and loads its sections as they are
// needed (executor.md, "Sync", §3a; #4485). A section is the subtree under one
// SectionBits-bit prefix of the key hash. A section the node has not loaded is
// held in its tree as one marker leaf under the section's prefix, whose value
// is the section's hash: a branch with one non-empty side passes that side's
// hash up unchanged, so the root with the marker is the root with the
// section's leaves.

// SectionBits is the depth sections are cut at: 4,096 sections.
const SectionBits = 12

// Sections is how many sections the tree is cut into.
const Sections = 1 << SectionBits

// Section is one section, by its prefix read as a number.
type Section uint16

// sectionTag ends a marker's key hash. With the depth after it, no account's
// key hash -- a SHA-256 output -- is one.
var sectionTag = [7]byte{'b', 'p', 't', 's', 'e', 'c', 't'}

// SectionOf is the section a key hash falls in.
func SectionOf(kh [32]byte) Section {
	return Section(uint16(kh[0])<<4 | uint16(kh[1])>>4)
}

// Marker is the key hash of the section's marker leaf: the section's prefix,
// zeros, and the tag and depth in the last eight bytes.
func (s Section) Marker() [32]byte {
	var k [32]byte
	k[0] = byte(s >> 4)
	k[1] = byte(s << 4)
	copy(k[24:31], sectionTag[:])
	k[31] = SectionBits
	return k
}

// MarkerKey is the record key of the section's marker leaf: a bare key hash.
func (s Section) MarkerKey() *record.Key {
	return record.KeyFromHash(s.Marker())
}

// IsSectionMarker reports whether a key hash is a section's marker.
func IsSectionMarker(kh [32]byte) bool {
	return kh == SectionOf(kh).Marker()
}

// ScanStart is the key a page of the section's leaves starts from: the
// section's prefix followed by every bit set. A page runs in descending key
// order, so the section's leaves are the page's until a key leaves it.
func (s Section) ScanStart() [32]byte {
	var k [32]byte
	for i := range k {
		k[i] = 0xff
	}
	k[0] = byte(s >> 4)
	k[1] = byte(s<<4) | 0x0f
	return k
}

// A SectionLeaf is one leaf of a section: its key hash and its value.
type SectionLeaf struct {
	KeyHash [32]byte
	Hash    [32]byte
}

// SectionHash is the hash of the subtree holding exactly the given leaves, all
// of one section, by the tree's rule: a set bit goes left, two non-empty sides
// combine left then right, and one non-empty side passes its hash up. It
// reports false for no leaves.
func SectionHash(leaves []SectionLeaf) ([32]byte, bool) {
	return subtreeOf(leaves, SectionBits)
}

func subtreeOf(leaves []SectionLeaf, bits int) ([32]byte, bool) {
	switch len(leaves) {
	case 0:
		return [32]byte{}, false
	case 1:
		return leaves[0].Hash, true
	}
	if bits >= 256 {
		return [32]byte{}, false
	}
	var left, right []SectionLeaf
	m := byte(0x80) >> (bits & 7)
	for _, l := range leaves {
		if l.KeyHash[bits>>3]&m != 0 {
			left = append(left, l)
		} else {
			right = append(right, l)
		}
	}
	l, lok := subtreeOf(left, bits+1)
	r, rok := subtreeOf(right, bits+1)
	switch {
	case lok && rok:
		var b [64]byte
		copy(b[:], l[:])
		copy(b[32:], r[:])
		return sha256.Sum256(b[:]), true
	case lok:
		return l, true
	default:
		return r, rok
	}
}

// SectionHashes is the hash of every section, as the top of a tree says it;
// a section with Present false is empty.
type SectionHashes struct {
	Hash    [Sections][32]byte
	Present [Sections]bool
}

// Root is the root the section hashes combine to by the tree's rule.
func (h *SectionHashes) Root() [32]byte {
	level := make([]position, Sections)
	for i := range level {
		level[i] = position{h.Hash[i], h.Present[i]}
	}
	return combinePositions(level)
}

// A position is one node's hash, or nothing.
type position struct {
	h  [32]byte
	ok bool
}

// combinePositions combines 2ⁿ positions, indexed most significant bit first,
// into the hash of the node above them by the tree's rule: HashSlots for any
// depth. It combines in place.
func combinePositions(cur []position) [32]byte {
	for n := len(cur); n > 1; n /= 2 {
		for i := 0; i < n/2; i++ {
			l, r := cur[2*i+1], cur[2*i]
			switch {
			case l.ok && r.ok:
				var b [64]byte
				copy(b[:], l.h[:])
				copy(b[32:], r.h[:])
				cur[i] = position{sha256.Sum256(b[:]), true}
			case l.ok:
				cur[i] = l
			default:
				cur[i] = r
			}
		}
	}
	return cur[0].h
}

// SectionHashesFromBlocks computes every section's hash from a tree's stored
// blocks: the root's block, and the block under every position of it that is a
// branch, keyed by that position. Each block below the root must hash to its
// position in the root's block, and the root's block to root; anything else is
// refused, since the blocks are then not one tree's.
func SectionHashesFromBlocks(root [32]byte, rootSlots []BlockSlot, below map[byte][]BlockSlot) (*SectionHashes, error) {
	if h := HashSlots(rootSlots); h != root {
		return nil, errors.Conflict.WithFormat("the root's block hashes to %x, not the root %x", h[:8], root[:8])
	}
	out := new(SectionHashes)
	for _, s := range rootSlots {
		if !s.Branch {
			// One account under the whole position: its hash is its
			// section's, and the position's other sections are empty.
			if s.KeyHash[0] != s.Index {
				return nil, errors.Conflict.WithFormat("the leaf at position %d has key %x", s.Index, s.KeyHash[:4])
			}
			sec := SectionOf(s.KeyHash)
			out.Hash[sec], out.Present[sec] = s.Hash, true
			continue
		}
		sub, ok := below[s.Index]
		if !ok {
			return nil, errors.Conflict.WithFormat("no block under position %d", s.Index)
		}
		if h := HashSlots(sub); h != s.Hash {
			return nil, errors.Conflict.WithFormat("the block under position %d hashes to %x, not %x", s.Index, h[:8], s.Hash[:8])
		}
		var nibbles [16][16]position
		for _, t := range sub {
			if !t.Branch && (t.KeyHash[0] != s.Index || t.KeyHash[1] != t.Index) {
				return nil, errors.Conflict.WithFormat("the leaf at position %d.%d has key %x", s.Index, t.Index, t.KeyHash[:4])
			}
			nibbles[t.Index>>4][t.Index&0x0f] = position{t.Hash, true}
		}
		for n := range nibbles {
			any := false
			for _, p := range nibbles[n] {
				any = any || p.ok
			}
			if !any {
				continue
			}
			sec := Section(uint16(s.Index)<<4 | uint16(n))
			out.Hash[sec], out.Present[sec] = combinePositions(nibbles[n][:]), true
		}
	}
	return out, nil
}
