// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package enumerate walks a partition's BPT page by page through the v3
// BptPageQuery and reports which accounts the peer holds and which of them the
// node does not agree with, for a node that is pulling the state
// (executor.md, "Sync", step 3).
//
// **Nothing is written.** The local BPT may only hold leaves derived from
// state this node holds and has verified; a leaf taken from a peer's word
// would make the local root the peer's root, and the local root is exactly
// what the tracker matches against an anchored root to decide the node is
// caught up. Enumeration therefore learns the peer's key set and its claimed
// value hashes and hands the difference to package pull, which fetches each
// account and writes it — and only then does a leaf enter the local BPT,
// derived from the state.
//
// The per-page BptRoot moves while the scan runs on a live network. That is
// expected and is not an error: the scan is a list of names, and convergence
// is decided later, by the tracker.
//
// Ported from bootstrap-v3 (issue #4293). Changed on this line: bootstrap-v3
// inserted the peer's leaves into the local BPT, which is safe only under the
// rejected model where matching the peer's whole root was the proof.
package enumerate

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Source is the read-only surface a walk needs from the network.
// Production wraps an api.Querier2; tests use a fake.
type Source interface {
	QueryBptPage(ctx context.Context, scope *url.URL, query *api.BptPageQuery) (*api.BptPageRecord, error)
}

// The v3 client is a Source as it stands.
var _ Source = api.Querier2{}

// A Page is one page of a walk: what the peer served, the accounts it named,
// and those whose leaf this node does not hold or holds and does not agree
// with.
type Page struct {
	Record   *api.BptPageRecord
	Leaves   int
	Accounts []*url.URL
	Stale    []*url.URL
}

// ReadPage reads the page of the peer's BPT that starts at start -- zero
// starts a walk -- and compares its leaves with the batch's. The batch is
// read, never written. The join walks a page at a time, and processes
// block-ledger records between pages (executor spec, "Sync", "The
// algorithm", step 1).
func ReadPage(ctx context.Context, src Source, scope *url.URL, batch *database.Batch, start [32]byte, count uint64) (*Page, error) {
	if count == 0 {
		count = 256
	}
	rec, err := src.QueryBptPage(ctx, scope, &api.BptPageQuery{StartHash: start, Count: count})
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}
	page := &Page{Record: rec}
	for _, e := range rec.Entries {
		if e == nil {
			continue
		}
		page.Leaves++
		if e.Account == nil {
			// A leaf that names no account cannot be pulled, so there is
			// nothing the node can do about it either way.
			continue
		}
		page.Accounts = append(page.Accounts, e.Account)

		local, err := batch.BPT().Get(record.KeyFromHash(e.KeyHash))
		switch {
		case err == nil && len(local) == 32 && [32]byte(local) == e.ValueHash:
			// Held, and it agrees
		case err == nil, errors.Is(err, errors.NotFound):
			page.Stale = append(page.Stale, e.Account)
		default:
			return nil, errors.UnknownError.WithFormat("read local leaf %x: %w", e.KeyHash[:8], err)
		}
	}
	return page, nil
}
