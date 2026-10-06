// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// bpt-page-probe times what a join pays per section of a partition's state
// tree against one node: a BptPage query of 256 leaves, as of the node's
// current tree and as of a block a few behind it, and the AccountStates query
// for the accounts one page names (#4529).
//
//	bpt-page-probe <endpoint> <partition-url> [pages]
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bptproof"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: bpt-page-probe <endpoint> <partition-url> [pages]")
		os.Exit(2)
	}
	pages := 20
	if len(os.Args) > 3 {
		pages, _ = strconv.Atoi(os.Args[3])
	}
	part := url.MustParse(os.Args[2])
	q := api.Querier2{Querier: jsonrpc.NewClient(os.Args[1])}
	ctx := context.Background()

	var ledger *protocol.SystemLedger
	if _, err := q.QueryAccountAs(ctx, part.JoinPath(protocol.Ledger), nil, &ledger); err != nil {
		fmt.Fprintln(os.Stderr, "ledger:", err)
		os.Exit(1)
	}
	fmt.Printf("%s is at block %d\n", part, ledger.Index)

	run := func(label string, height uint64) []*url.URL {
		start := bptproof.FullScanStart()
		var urls []*url.URL
		var total, slowest time.Duration
		n := 0
		for i := 0; i < pages; i++ {
			t := time.Now()
			page, err := q.QueryBptPage(ctx, part, &api.BptPageQuery{StartHash: start, Count: 256, ForHeight: height})
			took := time.Since(t)
			if err != nil {
				fmt.Printf("  %s page %d: %v after %v\n", label, i, err, took)
				return urls
			}
			n++
			total += took
			slowest = max(slowest, took)
			if urls == nil {
				for _, e := range page.Entries {
					if e != nil && e.Account != nil {
						urls = append(urls, e.Account)
					}
				}
			}
			if page.Done {
				break
			}
			start = page.NextStart
		}
		fmt.Printf("  %-28s %3d pages of 256: mean %7.1f ms, slowest %7.1f ms\n", label, n, float64(total.Milliseconds())/float64(max(n, 1)), float64(slowest.Milliseconds()))
		return urls
	}
	urls := run("BptPage current", 0)
	asOf := ledger.Index - 3
	run(fmt.Sprintf("BptPage as of %d", asOf), asOf)

	if len(urls) > 0 {
		for _, h := range []uint64{0, asOf} {
			t := time.Now()
			rec, err := q.QueryAccountStates(ctx, part, &api.AccountStatesQuery{Accounts: urls, ForHeight: h})
			took := time.Since(t)
			if err != nil {
				fmt.Printf("  AccountStates(%d) as of %d: %v after %v\n", len(urls), h, err, took)
				continue
			}
			fmt.Printf("  AccountStates(%d) as of %-6d  %7.1f ms (%d states)\n", len(urls), h, float64(took.Microseconds())/1000, len(rec.States))
		}
	}
}
