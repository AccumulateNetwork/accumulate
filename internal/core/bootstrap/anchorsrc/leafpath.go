// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package anchorsrc

import (
	"crypto/sha256"
	"math/bits"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
)

// leafPattern is the shape of the receipt from entry index of a chain to the
// chain's anchor when it holds height entries: for each step, whether the
// other hash is applied on the right.
//
// A chain of height entries is a row of perfect trees, one per set bit of
// height, largest first. An entry climbs its own tree, a step per level, its
// sibling on the right where its position has a zero bit. The anchor then
// folds the trees smallest first: everything smaller than the entry's tree
// arrives as one hash on the right, and every larger tree as one on the left.
func leafPattern(index, height uint64) ([]bool, bool) {
	if index >= height {
		return nil, false
	}
	offset := uint64(0)
	for b := bits.Len64(height) - 1; b >= 0; b-- {
		size := uint64(1) << b
		if height&size == 0 {
			continue
		}
		if index >= offset+size {
			offset += size
			continue
		}

		pos := index - offset
		var p []bool
		for l := 0; l < b; l++ {
			p = append(p, pos>>l&1 == 0)
		}
		if height&(size-1) != 0 {
			p = append(p, true)
		}
		for n := bits.OnesCount64(height >> (b + 1)); n > 0; n-- {
			p = append(p, false)
		}
		return p, true
	}
	return nil, false
}

// leafOf reads a receipt's shape backwards: the index of the entry of a chain
// of height entries whose receipt has exactly this shape.
func leafOf(path []*merkle.ReceiptEntry, height uint64) (uint64, bool) {
	offset := uint64(0)
	for b := bits.Len64(height) - 1; b >= 0; b-- {
		size := uint64(1) << b
		if height&size == 0 {
			continue
		}
		smaller := 0
		if height&(size-1) != 0 {
			smaller = 1
		}
		if len(path) != b+smaller+bits.OnesCount64(height>>(b+1)) {
			offset += size
			continue
		}
		pos := uint64(0)
		for l := 0; l < b; l++ {
			if !path[l].Right {
				pos |= 1 << l
			}
		}
		// Trees of different sizes can have paths of one length; the fold's
		// steps tell them apart.
		want, _ := leafPattern(offset+pos, height)
		match := true
		for i, right := range want {
			match = match && path[i].Right == right
		}
		if match {
			return offset + pos, true
		}
		offset += size
	}
	return 0, false
}

// splitAtLeaf finds where a receipt enters a chain of height entries as one of
// its ENTRIES: the step at which what remains is exactly the receipt of a
// leaf. It returns that step, the hash the receipt holds there, and the leaf's
// index.
//
// There is at most one such step. The remainder from any step after it is the
// path of an interior node, and from any step before it a path longer than the
// leaf's own, and neither has a leaf's shape: where a leaf of another tree
// would need the fold's one right-hand hash, this path has a left-hand one, or
// the reverse. So height -- which a verified anchor signs -- decides which
// hash on the path is the chain's entry, and no peer's word enters into it.
func splitAtLeaf(r *merkle.Receipt, height uint64) (step int, hash []byte, index uint64, ok bool) {
	working := r.Start
	found := false
	for k := 0; ; k++ {
		if j, is := leafOf(r.Entries[k:], height); is {
			if found {
				return 0, nil, 0, false
			}
			found, step, hash, index = true, k, working, j
		}
		if k == len(r.Entries) {
			break
		}
		working = applyEntry(r.Entries[k], working)
	}
	return step, hash, index, found
}

func applyEntry(e *merkle.ReceiptEntry, working []byte) []byte {
	h := sha256.New()
	if e.Right {
		h.Write(working)
		h.Write(e.Hash)
	} else {
		h.Write(e.Hash)
		h.Write(working)
	}
	return h.Sum(nil)
}
