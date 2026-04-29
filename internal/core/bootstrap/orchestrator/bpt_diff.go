// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package orchestrator

import (
	"context"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bptproof"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/enumerate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// BPTDiff walks both the source's BPT (via paginated BptPageQuery)
// and the launcher's local BPT, and reports leaf-level mismatches.
// Useful for diagnosing why the local BPT root doesn't equal the
// source's signed major-block anchor.
//
// Output is written to w: one line per mismatch, plus a final
// summary line. Returns the count of mismatches and the count of
// keys present only on one side.
type BPTDiffResult struct {
	SourceLeaves   int
	LocalLeaves    int
	Match          int
	ValueMismatch  int    // same key, different value-hash
	OnlyOnSource   int    // present on source, missing locally
	OnlyOnLocal    int    // present locally, missing on source
	SourceRoot     [32]byte
	LocalRoot      [32]byte
}

// RunBPTDiff scans the source's BPT and compares to the launcher's
// local BPT, emitting per-leaf mismatches to w.
func RunBPTDiff(
	ctx context.Context,
	src enumerate.Source,
	db *database.Database,
	scope *url.URL,
	pageSize uint64,
	w io.Writer,
) (*BPTDiffResult, error) {
	if pageSize == 0 {
		pageSize = 256
	}

	// Collect every leaf the source has into a map.
	sourceLeaves := make(map[[32]byte][32]byte)
	var sourceRoot [32]byte
	var startKey [32]byte
	for i := range startKey {
		startKey[i] = 0xff // FullScanStart
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := src.QueryBptPage(ctx, scope, &api.BptPageQuery{
			StartHash: startKey,
			Count:     pageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("source page: %w", err)
		}
		if page == nil || len(page.Entries) == 0 {
			break
		}
		for _, e := range page.Entries {
			if e == nil {
				continue
			}
			sourceLeaves[e.KeyHash] = e.ValueHash
		}
		sourceRoot = page.BptRoot
		if page.Done {
			break
		}
		startKey = page.NextStart
	}

	// Walk launcher's local BPT, compare each leaf.
	roBatch := db.Begin(false)
	defer roBatch.Discard()
	localRoot, err := roBatch.GetBptRootHash()
	if err != nil {
		return nil, fmt.Errorf("local root: %w", err)
	}

	res := &BPTDiffResult{
		SourceLeaves: len(sourceLeaves),
		SourceRoot:   sourceRoot,
		LocalRoot:    localRoot,
	}

	// Iterate local BPT page-by-page using the same primitive the
	// server uses.
	var local [32]byte
	for i := range local {
		local[i] = 0xff
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := bptproof.GetPage(roBatch, local, int(pageSize))
		if err != nil {
			return nil, fmt.Errorf("local page: %w", err)
		}
		if page == nil || len(page.Entries) == 0 {
			break
		}
		for _, e := range page.Entries {
			res.LocalLeaves++
			srcVal, ok := sourceLeaves[e.KeyHash]
			if !ok {
				res.OnlyOnLocal++
				fmt.Fprintf(w, "ONLY-LOCAL  key=%x  local=%x\n", e.KeyHash, e.ValueHash)
				continue
			}
			if srcVal == e.ValueHash {
				res.Match++
			} else {
				res.ValueMismatch++
				fmt.Fprintf(w, "MISMATCH    key=%x  src=%x  local=%x\n", e.KeyHash, srcVal, e.ValueHash)
			}
			delete(sourceLeaves, e.KeyHash) // mark seen
		}
		if page.Done {
			break
		}
		local = page.NextStart
	}
	// Anything left in sourceLeaves is only on source.
	for k, v := range sourceLeaves {
		res.OnlyOnSource++
		fmt.Fprintf(w, "ONLY-SOURCE key=%x  src=%x\n", k, v)
	}

	fmt.Fprintf(w, "--- bpt diff summary ---\n")
	fmt.Fprintf(w, "source leaves: %d  (root %x)\n", res.SourceLeaves, res.SourceRoot)
	fmt.Fprintf(w, "local leaves:  %d  (root %x)\n", res.LocalLeaves, res.LocalRoot)
	fmt.Fprintf(w, "match:         %d\n", res.Match)
	fmt.Fprintf(w, "value-mismatch:%d\n", res.ValueMismatch)
	fmt.Fprintf(w, "only-on-source:%d\n", res.OnlyOnSource)
	fmt.Fprintf(w, "only-on-local: %d\n", res.OnlyOnLocal)
	return res, nil
}

// localPageStart is the conventional starting key for a full BPT
// scan — see bptproof.FullScanStart.
var _ = record.Key{}
