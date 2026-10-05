// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// pull-probe pulls one account from one node as a joining node would, into
// a scratch store, and says what it cost: every call by kind, how many, how
// long, and how many ran at once (#4529, #4531).
//
//	pull-probe <endpoint> <account> <mode: state|whole|window> [window]
package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

type counting struct {
	api.Querier
	mu             sync.Mutex
	n              map[string]int
	d              map[string]time.Duration
	inflight, most int
	first, last    time.Time
}

func (c *counting) Query(ctx context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	kind := q.QueryType().String()
	if cq, ok := q.(*api.ChainQuery); ok {
		switch {
		case cq.Range != nil && cq.Range.Expand != nil && *cq.Range.Expand:
			kind += " entries+messages"
		case cq.Range != nil:
			kind += " entries"
		case cq.Name != "":
			kind += " head"
		default:
			kind += " list"
		}
	}
	c.mu.Lock()
	c.inflight++
	c.most = max(c.most, c.inflight)
	if c.first.IsZero() {
		c.first = time.Now()
	}
	c.mu.Unlock()
	start := time.Now()
	r, err := c.Querier.Query(ctx, scope, q)
	took := time.Since(start)
	c.mu.Lock()
	c.inflight--
	c.n[kind]++
	c.d[kind] += took
	c.last = time.Now()
	c.mu.Unlock()
	return r, err
}

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: pull-probe <endpoint> <account> <state|whole|window> [window]")
		os.Exit(2)
	}
	u := url.MustParse(os.Args[2])
	part := u.RootIdentity()
	opts := pull.Options{WithReceipt: true, Partition: part}
	switch os.Args[3] {
	case "state":
		opts.Mode = pull.ModeStateOnly
	case "whole":
		opts.Mode = pull.ModeFullSpine
	case "window":
		opts.Mode = pull.ModeWindow
		opts.Window = pull.AnchorPoolWindow
		if len(os.Args) > 4 {
			w, _ := strconv.Atoi(os.Args[4])
			opts.Window = uint64(w)
		}
	}
	dir, err := os.MkdirTemp("", "pull-probe")
	check(err)
	defer os.RemoveAll(dir)
	db, err := database.OpenLevelDB(dir, nil)
	check(err)
	defer db.Close()
	opts.Store = db

	cq := &counting{Querier: jsonrpc.NewClient(os.Args[1]), n: map[string]int{}, d: map[string]time.Duration{}}
	src := api.Querier2{Querier: cq}
	start := time.Now()
	err = pull.Account(context.Background(), src, nil, u, opts)
	total := time.Since(start)
	if err != nil {
		fmt.Printf("pull failed after %v: %v\n", total.Round(time.Millisecond), err)
	}
	var kinds []string
	var calls int
	var inCalls time.Duration
	for k := range cq.n {
		kinds = append(kinds, k)
		calls += cq.n[k]
		inCalls += cq.d[k]
	}
	sort.Slice(kinds, func(i, j int) bool { return cq.d[kinds[i]] > cq.d[kinds[j]] })
	fmt.Printf("%s %s from %s: %v wall, %d calls, %v in calls (%.1f calls in flight at most)\n", u, os.Args[3], os.Args[1], total.Round(time.Millisecond), calls, inCalls.Round(time.Millisecond), float64(cq.most))
	for _, k := range kinds {
		fmt.Printf("  %-36s %6d calls %9v  avg %v\n", k, cq.n[k], cq.d[k].Round(time.Millisecond), (cq.d[k] / time.Duration(cq.n[k])).Round(time.Millisecond))
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
