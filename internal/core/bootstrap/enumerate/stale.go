// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enumerate

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Stale walks the peer's BPT and reports the accounts whose leaf the node does
// not hold or does not agree with. Those are the accounts to pull
// (executor.md, "Sync", step 3: every account behind a leaf that is missing or
// stale).
//
// It is what a node restarting with its store intact asks for: its leaves are
// the state as of its last block, so only the leaves that moved since come
// back. A fresh node gets every leaf, which is the same answer.
//
// Nothing is written. Stale names accounts; pull.Account fetches and verifies
// them.
func Stale(ctx context.Context, src Source, scope *url.URL, batch *database.Batch, opts Options) ([]*url.URL, error) {
	if src == nil {
		return nil, errors.BadRequest.With("enumerate.Stale: Source required")
	}
	if scope == nil {
		return nil, errors.BadRequest.With("enumerate.Stale: scope required")
	}
	if batch == nil {
		return nil, errors.BadRequest.With("enumerate.Stale: batch required")
	}
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = 256
	}

	var stale []*url.URL
	var start [32]byte
	pages := 0
	for {
		if err := ctx.Err(); err != nil {
			return stale, err
		}

		page, err := src.QueryBptPage(ctx, scope, &api.BptPageQuery{
			StartHash: start,
			Count:     pageSize,
		})
		if err != nil {
			return stale, errors.UnknownError.WithFormat("page %d: %w", pages+1, err)
		}
		pages++

		for _, e := range page.Entries {
			if e == nil || e.Account == nil {
				continue
			}
			local, err := batch.BPT().Get(record.KeyFromHash(e.KeyHash))
			switch {
			case err == nil && len(local) == 32 && [32]byte(local) == e.ValueHash:
				continue // Held, and it agrees
			case err == nil, errors.Is(err, errors.NotFound):
				stale = append(stale, e.Account)
			default:
				return stale, errors.UnknownError.WithFormat("read local leaf %x: %w", e.KeyHash[:8], err)
			}
		}

		if opts.OnPage != nil {
			opts.OnPage(pages, page)
		}
		if page.Done {
			return stale, nil
		}
		start = page.NextStart
	}
}
