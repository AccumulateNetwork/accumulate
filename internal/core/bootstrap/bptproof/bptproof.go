// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package bptproof returns paginated chunks of the local BPT for
// the bootstrap launcher's enumeration phase.
//
// A page is a slice of (KeyHash, ValueHash) pairs in BPT key order
// plus the BPT root the page is consistent with. Per-leaf Merkle
// proofs are NOT included — the protocol assumes the launcher
// builds a complete local BPT from these pages and matches its
// root against a trusted current StateTreeAnchor (obtained via the
// node's signed major-block anchors). Mismatch on the final root
// is the whole-page consistency check; per-leaf proofs would be
// log-N each and add no security on top of root-match.
//
// Server side: this runs against the local DB. Clients call via
// the v3 BptPageQuery (issue #3969-equivalent on bootstrap-v3).
package bptproof

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// LeafSummary is one BPT leaf — the account-key hash plus its
// value hash. The value hash is the BPT leaf's content.
type LeafSummary struct {
	KeyHash   [32]byte
	ValueHash [32]byte
}

// Page is one paginated chunk of the BPT.
type Page struct {
	// Entries are the leaves in this page, in BPT key order.
	Entries []LeafSummary

	// NextStart is the key to pass as the StartHash of the next
	// page request. When Done is true, NextStart is undefined.
	NextStart [32]byte

	// Done reports whether this page exhausts the BPT.
	Done bool

	// BptRoot is the root the entries in this page are consistent
	// with. May differ across pages on a live network if the BPT
	// advances between requests; the launcher reconciles via
	// gossip.
	BptRoot [32]byte
}

// GetPage returns the next pageSize entries of the BPT starting
// after startKey. Use the all-FF [32]byte as the initial startKey
// to begin a fresh scan.
//
// pageSize must be > 0. The implementation does not enforce an
// upper bound; callers that fan out to untrusted clients should
// cap it.
func GetPage(batch *database.Batch, startKey [32]byte, pageSize int) (*Page, error) {
	if pageSize <= 0 {
		return nil, fmt.Errorf("bptproof.GetPage: pageSize must be > 0, got %d", pageSize)
	}

	rootHash, err := batch.GetBptRootHash()
	if err != nil {
		return nil, fmt.Errorf("read BPT root: %w", err)
	}

	pairs, nextStart, err := batch.BPT().GetRange(startKey, pageSize)
	if err != nil {
		return nil, fmt.Errorf("BPT range from %x: %w", startKey[:8], err)
	}

	out := &Page{
		Entries:   make([]LeafSummary, len(pairs)),
		NextStart: nextStart,
		Done:      len(pairs) < pageSize,
		BptRoot:   rootHash,
	}
	for i, p := range pairs {
		out.Entries[i].KeyHash = p.Key.Hash()
		if len(p.Value) != 32 {
			return nil, fmt.Errorf("BPT leaf at %x has %d-byte value, want 32",
				p.Key.Hash(), len(p.Value))
		}
		copy(out.Entries[i].ValueHash[:], p.Value)
	}
	return out, nil
}

// FullScanStart returns the all-FF [32]byte that callers pass as
// the initial startKey for a fresh scan.
func FullScanStart() [32]byte {
	var k [32]byte
	for i := range k {
		k[i] = 0xff
	}
	return k
}
