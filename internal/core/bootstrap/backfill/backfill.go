// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package backfill is the bootstrap-v3 ACTIVE → COMPLETE history
// backfiller (#3992).
//
// After bootstrap reaches ACTIVE, the local DB has authoritative
// state (head + chain heads) for every account that was either
// touched during steady-state ingestion or pulled in spine mode.
// The long-tail of accounts (the BPT carries their leaf hash but
// not their entries) is the gap COMPLETE closes.
//
// This package walks every account in the local BPT and, for each,
// pulls full chain entries via pull.Account(ModeFullSpine). When the
// pass completes, it promotes the bound nodestate.Machine from
// ACTIVE to COMPLETE.
//
// Status: minimal first-cut. Caveats:
//   - Re-running against an already-backfilled account double-applies
//     entries (merkle.AddEntry is append-only). Idempotent backfill
//     is a follow-up. For now, run once.
//   - No rate limiting beyond context cancellation. The caller is
//     responsible for scheduling around live traffic.
//   - HistoryDepth is hard-coded to 0 (unlimited). Configurable
//     retention windows are a follow-up.
package backfill

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Options configures Run.
type Options struct {
	// PageSize for paginated chain entry pulls. Default 256.
	PageSize uint64

	// OnAccount, if non-nil, is invoked once per account processed.
	// status is "ok" on success or "skip: <reason>" / "err: <msg>".
	OnAccount func(u *url.URL, status string)
}

// Result reports the outcome of a Run.
type Result struct {
	// Walked is the number of BPT entries iterated.
	Walked int

	// Pulled is the number of accounts that successfully had full
	// chain entries pulled.
	Pulled int

	// Skipped is the number of accounts that were iterated but not
	// pulled (e.g., system accounts already pulled in spine).
	Skipped int

	// Errored is the number of accounts that errored during pull.
	// On error, Run continues with the next account (best-effort).
	Errored int
}

// Run iterates every account in db's BPT, pulls full chain history
// for each, and on a clean pass promotes machine from StateActive to
// StateComplete. Returns Result and any non-recoverable error.
//
// Returns ctx.Err() if context is canceled mid-walk.
func Run(
	ctx context.Context,
	src pull.Source,
	db *database.Database,
	machine *nodestate.Machine,
	opts Options,
) (*Result, error) {
	if src == nil {
		return nil, fmt.Errorf("backfill.Run: src required")
	}
	if db == nil {
		return nil, fmt.Errorf("backfill.Run: db required")
	}
	if machine == nil {
		return nil, fmt.Errorf("backfill.Run: machine required")
	}
	if machine.State() != nodestate.StateActive {
		return nil, fmt.Errorf("backfill.Run: machine must be StateActive, got %v", machine.State())
	}

	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = 256
	}

	res := &Result{}

	// Iterate accounts. The iterator yields accounts in BPT order.
	roBatch := db.Begin(false)
	urls, err := collectAccountURLs(roBatch)
	roBatch.Discard()
	if err != nil {
		return res, fmt.Errorf("collect accounts: %w", err)
	}

	for _, u := range urls {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		res.Walked++

		batch := db.Begin(true)
		err := pull.Account(ctx, src, batch, u, pull.Options{
			Mode:     pull.ModeFullSpine,
			PageSize: pageSize,
		})
		if err != nil {
			batch.Discard()
			res.Errored++
			report(opts.OnAccount, u, "err: "+err.Error())
			continue
		}
		if err := batch.UpdateBPT(); err != nil {
			batch.Discard()
			res.Errored++
			report(opts.OnAccount, u, "err: update BPT: "+err.Error())
			continue
		}
		if err := batch.Commit(); err != nil {
			res.Errored++
			report(opts.OnAccount, u, "err: commit: "+err.Error())
			continue
		}
		res.Pulled++
		report(opts.OnAccount, u, "ok")
	}

	// Promote to COMPLETE on a clean pass with no errors. Errors
	// during walk leave the machine in ACTIVE so the operator can
	// inspect and retry.
	if res.Errored == 0 {
		machine.PromoteToComplete(0 /* unlimited history */, machine.Get().SinceBlock)
	}
	return res, nil
}

// collectAccountURLs walks the BPT and returns every account URL.
// We snapshot to a slice up front rather than streaming so the
// outer loop can safely open one write batch per account without
// holding the iterator's read batch open.
func collectAccountURLs(batch *database.Batch) ([]*url.URL, error) {
	out := make([]*url.URL, 0, 256)
	it := batch.IterateAccounts()
	for it.Next() {
		acct := it.Value()
		if acct == nil {
			continue
		}
		out = append(out, acct.Url())
	}
	if err := it.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func report(fn func(*url.URL, string), u *url.URL, status string) {
	if fn != nil {
		fn(u, status)
	}
}
