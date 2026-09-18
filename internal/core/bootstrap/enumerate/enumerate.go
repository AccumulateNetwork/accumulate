// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package enumerate walks a partition's BPT page by page through the v3
// BptPageQuery and inserts every (KeyHash, ValueHash) leaf locally, for a node
// that is pulling the state (executor.md, "Sync", step 3).
//
// The pages say which accounts exist and what their leaves hash to. They are
// not themselves trusted: nothing is believed until each account is pulled and
// verified against the root the Directory anchored (package pull). A fresh
// node enumerates once to learn the set; a node restarting with its store
// intact does not enumerate at all — the blocks it buffers name what changed.
//
// The per-page BptRoot moves while the scan runs on a live network. That is
// expected and is not an error: the scan is a list of names, and convergence
// is decided later, by the tracker.
//
// Ported from bootstrap-v3 (issue #4293). Changed on this line: the doc
// comment. The code is unchanged.
package enumerate

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Source is the read-only surface Run needs from the network.
// Production wraps an api.Querier2; tests use a fake.
type Source interface {
	QueryBptPage(ctx context.Context, scope *url.URL, query *api.BptPageQuery) (*api.BptPageRecord, error)
}

// Result reports the outcome of a Run.
type Result struct {
	// PagesPulled counts the BptPageQuery requests issued.
	PagesPulled int

	// LeavesInserted counts the (KeyHash, ValueHash) pairs written
	// to the local BPT.
	LeavesInserted int

	// LastBptRoot is the BptRoot from the last received page, for
	// progress reporting. It is the peer's word for its own root, so it
	// is not a thing to verify against.
	LastBptRoot [32]byte

	// Accounts is the list of account URLs the scan named. The caller
	// pulls each one whose leaf it does not already hold.
	Accounts []*url.URL
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

// Run paginates the partition at scope, inserting every received
// leaf into the local BPT under batch. Returns when the source
// reports Done=true.
//
// The batch is the launcher's local DB write batch. The caller is
// responsible for committing it after Run returns (and after any
// concurrent gossip processing has finished).
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
		return nil, fmt.Errorf("enumerate.Run: Source required")
	}
	if scope == nil {
		return nil, fmt.Errorf("enumerate.Run: scope required")
	}
	if batch == nil {
		return nil, fmt.Errorf("enumerate.Run: batch required")
	}
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = 256
	}

	res := &Result{}
	var start [32]byte // zero = begin a fresh scan from the highest BPT key

	for {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}

		page, err := src.QueryBptPage(ctx, scope, &api.BptPageQuery{
			StartHash: start,
			Count:     pageSize,
		})
		if err != nil {
			return res, fmt.Errorf("page %d: %w", res.PagesPulled+1, err)
		}
		res.PagesPulled++

		for _, e := range page.Entries {
			if e == nil {
				continue
			}
			if err := batch.BPT().Insert(record.KeyFromHash(e.KeyHash), e.ValueHash[:]); err != nil {
				return res, fmt.Errorf("insert leaf %x at page %d: %w",
					e.KeyHash[:8], res.PagesPulled, err)
			}
			res.LeavesInserted++
			if e.Account != nil {
				res.Accounts = append(res.Accounts, e.Account)
			}
		}
		res.LastBptRoot = page.BptRoot

		if opts.OnPage != nil {
			opts.OnPage(res.PagesPulled, page)
		}

		if page.Done {
			return res, nil
		}
		start = page.NextStart
	}
}
