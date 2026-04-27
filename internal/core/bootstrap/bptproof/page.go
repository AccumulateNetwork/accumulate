// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bptproof

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// PageEntry is one (key_hash, value_hash) pair from a paginated BPT
// enumeration.
type PageEntry struct {
	KeyHash   [32]byte
	ValueHash [32]byte
}

// Page is one paginated chunk of BPT leaves consistent with the BPT root
// at the time of the call.
type Page struct {
	// Entries in this page, ordered from largest key to smallest (the BPT
	// iterator's natural order).
	Entries []PageEntry

	// NextStart is the cursor for the next page. Zero (i.e. all-zero hash)
	// when there are no more entries. Use it as the StartHash for the next
	// GetPage call.
	NextStart [32]byte

	// BptRoot is the BPT root the page is consistent with at the time of
	// the call.
	BptRoot [32]byte

	// Done is true when this page exhausted the keyspace.
	Done bool
}

// GetPage returns up to count BPT leaves with key hashes <= startHash.
// Pass a zero startHash to get the first (highest-hash) page; pass the
// previous page's NextStart for subsequent pages.
//
// This drives Phase 1 BPT-structure fill in the bootstrap design (#3953,
// #3969). The launcher iterates pages until Done is true, inserting each
// entry into its local BPT until the local root matches the network's
// committed StateTreeAnchor.
func GetPage(batch *database.Batch, startHash [32]byte, count int) (*Page, error) {
	if count <= 0 {
		return nil, fmt.Errorf("count must be positive, got %d", count)
	}

	rootHash, err := batch.GetBptRootHash()
	if err != nil {
		return nil, fmt.Errorf("get bpt root: %w", err)
	}

	// If the caller didn't supply a start hash, use the BPT's natural
	// starting position (all-ones).
	if isZero32(startHash) {
		for i := range startHash {
			startHash[i] = 0xff
		}
	}

	bpt := batch.BPT()
	it := bpt.Iterate(count)
	// We need to seed the iterator at startHash. The exported iterator
	// always starts at all-ones, so for non-default starts we do a single
	// pass and then prune. Future revision: add an exported method on BPT
	// that takes a start position directly (#3969 follow-up).
	page := &Page{BptRoot: rootHash}
	page.Entries = make([]PageEntry, 0, count)

	// Walk the iterator until we've collected `count` entries from
	// positions <= startHash.
	for it.Next() {
		for _, kv := range it.Value() {
			kh := kv.Key.Hash()
			if hashLess(startHash, kh) {
				// kh > startHash; not in the requested range yet
				continue
			}
			var entry PageEntry
			entry.KeyHash = kh
			if len(kv.Value) == 32 {
				copy(entry.ValueHash[:], kv.Value)
			}
			page.Entries = append(page.Entries, entry)
			if len(page.Entries) >= count {
				break
			}
		}
		if len(page.Entries) >= count {
			break
		}
	}
	if err := it.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}

	if len(page.Entries) == 0 {
		page.Done = true
		return page, nil
	}

	// NextStart = (smallest key in this page) - 1, or signal Done if we
	// hit the end. The BPT iterator returns entries from high to low; the
	// last collected entry has the smallest key in this page.
	last := page.Entries[len(page.Entries)-1].KeyHash
	if isZero32(last) {
		page.Done = true
		return page, nil
	}
	page.NextStart = decrementHash(last)
	return page, nil
}

func isZero32(h [32]byte) bool {
	for _, b := range h {
		if b != 0 {
			return false
		}
	}
	return true
}

// hashLess reports whether a < b in big-endian byte order.
func hashLess(a, b [32]byte) bool {
	for i := 0; i < 32; i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// decrementHash returns h-1 in big-endian. If h == 0 it returns 0
// (callers should check via isZero32 before calling).
func decrementHash(h [32]byte) [32]byte {
	for i := 31; i >= 0; i-- {
		if h[i] != 0 {
			h[i]--
			return h
		}
		h[i] = 0xff
	}
	return [32]byte{}
}
