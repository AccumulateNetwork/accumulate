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
// account, verifies it against the root the Directory anchored, and writes it
// — and only then does a leaf enter the local BPT, derived from the state.
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

// Source is the read-only surface Run needs from the network.
// Production wraps an api.Querier2; tests use a fake.
type Source interface {
	QueryBptPage(ctx context.Context, scope *url.URL, query *api.BptPageQuery) (*api.BptPageRecord, error)
}

// The v3 client is a Source as it stands.
var _ Source = api.Querier2{}

// Result reports the outcome of a Run.
type Result struct {
	// PagesPulled counts the BptPageQuery requests issued.
	PagesPulled int

	// LeavesSeen counts the (KeyHash, ValueHash) pairs the peer served.
	LeavesSeen int

	// LastBptRoot is the BptRoot from the last received page, for
	// progress reporting. It is the peer's word for its own root, so it
	// is not a thing to verify against.
	LastBptRoot [32]byte

	// Accounts is every account the scan named.
	Accounts []*url.URL

	// Stale is the accounts whose leaf the node does not hold, or holds and
	// does not agree with. Those are the accounts to pull.
	Stale []*url.URL
}

// Options configures Run.
type Options struct {
	// PageSize is the per-request leaf count. Default 256.
	PageSize uint64

	// OnPage, if non-nil, is invoked after each page completes.
	// Lets the caller observe progress (e.g., update a tracker
	// or log).
	OnPage func(pageNum int, page *api.BptPageRecord)
}

// Run paginates the partition at scope and reports what the peer holds and
// how it differs from what the node holds. The batch is read, never written.
//
// It is what a node restarting with its store intact asks for: its leaves are
// the state as of its last block, so only the leaves that moved since come
// back stale. A fresh node holds no leaves, so every account is stale, which
// is the same answer.
//
// On error, Run returns the partial Result so the caller can decide
// whether to resume from the last good page.
func Run(
	ctx context.Context,
	src Source,
	scope *url.URL,
	batch *database.Batch,
	opts Options,
) (*Result, error) {
	if src == nil {
		return nil, errors.BadRequest.With("enumerate.Run: Source required")
	}
	if scope == nil {
		return nil, errors.BadRequest.With("enumerate.Run: scope required")
	}
	if batch == nil {
		return nil, errors.BadRequest.With("enumerate.Run: batch required")
	}
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = 256
	}

	res := &Result{}
	var start [32]byte // zero = begin a fresh scan from the highest BPT key

	for {
		if err := ctx.Err(); err != nil {
			return res, err
		}

		page, err := ReadPage(ctx, src, scope, batch, start, pageSize)
		if err != nil {
			return res, errors.UnknownError.WithFormat("page %d: %w", res.PagesPulled+1, err)
		}
		res.PagesPulled++
		res.LeavesSeen += page.Leaves
		res.Accounts = append(res.Accounts, page.Accounts...)
		res.Stale = append(res.Stale, page.Stale...)
		res.LastBptRoot = page.Record.BptRoot

		if opts.OnPage != nil {
			opts.OnPage(res.PagesPulled, page.Record)
		}

		if page.Record.Done {
			return res, nil
		}
		start = page.Record.NextStart
	}
}

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
// read, never written. It is one step of Run, for a caller that walks a page
// at a time: the join, which processes block-ledger records between pages
// (executor spec, "Sync", "The algorithm", step 1).
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
