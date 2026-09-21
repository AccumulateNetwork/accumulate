// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package bptproof returns paginated chunks of the local BPT, for a node that
// is pulling the state (executor.md, "Sync", step 3).
//
// A page is a run of (KeyHash, ValueHash) pairs in BPT key order plus the BPT
// root the page is consistent with. A page carries no per-leaf proof: the
// pages say which accounts exist and what their leaves hash to, and each
// account's state is verified on its own, when it is pulled, against the root
// the Directory anchored for the block it was served at (see package pull).
//
// Ported from bootstrap-v3 (issue #4293). Changed on this line: the doc
// comment, which described the rejected "match the whole BPT root and trust
// the pages" model. The code is unchanged.
package bptproof

import (
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// LeafSummary is one BPT leaf — the account-key hash, its value hash, and the
// account URL. The URL is what the puller needs to ask for the account;
// without it it would have to keep its own key-hash to URL map.
type LeafSummary struct {
	KeyHash   [32]byte
	ValueHash [32]byte
	Account   *url.URL
}

// Page is one paginated chunk of the BPT.
type Page struct {
	// Entries are the leaves in this page, in BPT key order.
	Entries []LeafSummary

	// NextStart is the key to pass as the StartHash of the next page request.
	// Undefined when Done.
	NextStart [32]byte

	// Done reports whether this page exhausts the BPT.
	Done bool

	// BptRoot is the root the entries in this page are consistent with. It
	// moves across pages on a live network; that is expected, and it is not
	// what a pulled account is verified against.
	BptRoot [32]byte
}

// Tree is the BPT a page is read from, at whatever version the caller means:
// the node's current tree, or the tree as of an anchored block. It is an
// argument because both shapes serve the same page from the same leaves, and
// the mapping from leaf to summary was written twice before this
// (internal/api/v3/querier.go, #4361).
type Tree interface {
	// Root is what the entries of a page read from this tree are consistent
	// with.
	Root() ([32]byte, error)

	// Range is the next pageSize entries after startKey, in BPT key order.
	Range(startKey [32]byte, pageSize int) (entries []bpt.KeyValuePair, nextStart [32]byte, err error)
}

// Current is the node's BPT as it stands.
func Current(batch *database.Batch) Tree { return currentTree{batch} }

type currentTree struct{ batch *database.Batch }

func (t currentTree) Root() ([32]byte, error) { return t.batch.GetBptRootHash() }
func (t currentTree) Range(startKey [32]byte, pageSize int) ([]bpt.KeyValuePair, [32]byte, error) {
	return t.batch.BPT().GetRange(startKey, pageSize)
}

// AsOf is the node's BPT as of an anchored block, whose root the caller has
// already resolved. The walk is the current path's with the node loads
// redirected to retained versions (#4361).
func AsOf(batch *database.Batch, block uint64, root [32]byte) Tree {
	return retainedTree{batch, block, root}
}

type retainedTree struct {
	batch *database.Batch
	block uint64
	root  [32]byte
}

func (t retainedTree) Root() ([32]byte, error) { return t.root, nil }
func (t retainedTree) Range(startKey [32]byte, pageSize int) ([]bpt.KeyValuePair, [32]byte, error) {
	return t.batch.BPT().GetRangeAt(t.block, t.root, startKey, pageSize)
}

// GetPage returns the next pageSize entries of tree starting after startKey.
// Use FullScanStart as the initial startKey to begin a fresh scan.
//
// pageSize must be > 0. The caller caps it for untrusted clients.
func GetPage(tree Tree, startKey [32]byte, pageSize int) (*Page, error) {
	if pageSize <= 0 {
		return nil, fmt.Errorf("bptproof.GetPage: pageSize must be > 0, got %d", pageSize)
	}

	rootHash, err := tree.Root()
	if err != nil {
		return nil, fmt.Errorf("read BPT root: %w", err)
	}

	pairs, nextStart, err := tree.Range(startKey, pageSize)
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
		// Account leaves are keyed ("Account", *url.URL). Other record types
		// may be in the BPT, so this is best-effort.
		if p.Key.Len() >= 2 {
			if u, ok := p.Key.Get(1).(*url.URL); ok {
				out.Entries[i].Account = u
			}
		}
	}
	return out, nil
}

// FullScanStart returns the all-FF key that callers pass as the initial
// startKey for a fresh scan.
func FullScanStart() [32]byte {
	var k [32]byte
	for i := range k {
		k[i] = 0xff
	}
	return k
}
