// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// tracker follows a sample of the generated transactions so the run can report
// what was actually delivered rather than only what was accepted.
type tracker struct {
	Q api.Querier2

	mu       sync.Mutex
	gen      map[string]int         // action name -> transactions generated (every one, not sampled)
	roots    map[string][]*url.TxID // action name -> followed transaction IDs
	failures map[string]int         // action name -> submissions the network rejected
	skips    map[string]int         // action name -> times preconditions were not met
}

func newTracker(q api.Querier2) *tracker {
	return &tracker{Q: q, gen: map[string]int{}, roots: map[string][]*url.TxID{}, failures: map[string]int{}, skips: map[string]int{}}
}

// generated records that a transaction of this type was submitted. Unlike
// follow, which keeps only a sample for the delivery report, this counts every
// one, so the live per-type mix is exact rather than 1-in-N.
func (t *tracker) generated(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.gen[name]++
}

// typeStat is one row of the live per-type mix.
type typeStat struct {
	Generated int `json:"generated"`
	Rejected  int `json:"rejected"`
	Skipped   int `json:"skipped"`
}

// snapshot returns the current per-type counts and their totals, for the live
// stats file. Names present in any of the three maps appear.
func (t *tracker) snapshot() (perType map[string]typeStat, totGen, totRej, totSkip int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	perType = map[string]typeStat{}
	add := func(name string) {
		if _, ok := perType[name]; ok {
			return
		}
		s := typeStat{Generated: t.gen[name], Rejected: t.failures[name], Skipped: t.skips[name]}
		perType[name] = s
		totGen += s.Generated
		totRej += s.Rejected
		totSkip += s.Skipped
	}
	for n := range t.gen {
		add(n)
	}
	for n := range t.failures {
		add(n)
	}
	for n := range t.skips {
		add(n)
	}
	return perType, totGen, totRej, totSkip
}

func (t *tracker) follow(name string, ids []*url.TxID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.roots[name] = append(t.roots[name], ids...)
}

// track counts a transaction in the live mix and follows it for the delivery
// report. The growth sequences use it because, unlike the main loop, they are
// not sampled — every growth transaction is both counted and followed.
func (t *tracker) track(name string, ids []*url.TxID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.gen[name]++
	t.roots[name] = append(t.roots[name], ids...)
}

// failed records a submission the network rejected — a real problem.
func (t *tracker) failed(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failures[name]++
}

// skipped records an action that declined to run because the state it needs
// does not exist yet. That is normal, especially early in a run, and must not
// be reported as a failure: a generator that conflates the two hides real
// rejections in a haze of expected ones.
func (t *tracker) skipped(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.skips[name]++
}

// settle waits until every followed transaction is delivered, or the grace
// period expires. It returns how many never landed.
//
// A transaction that is still pending after the grace period is worth
// surfacing precisely because it did not fail: a rejected transaction is
// visible immediately, but one whose signature was refused sits pending with
// no signature recorded and no error anywhere. Left unreported it looks like
// success.
func (t *tracker) settle(ctx context.Context, grace time.Duration) int {
	deadline := time.Now().Add(grace)
	for {
		pending := t.sweep(ctx)
		if pending == 0 {
			return 0
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			log.Printf("%d messages still undelivered after the grace period", pending)
			t.reportStuck(ctx)
			return pending
		}
		log.Printf("waiting on %d undelivered messages...", pending)
		time.Sleep(15 * time.Second)
	}
}

// sweep counts how many followed messages are not yet delivered.
func (t *tracker) sweep(ctx context.Context) int {
	t.mu.Lock()
	var all []*url.TxID
	for _, ids := range t.roots {
		all = append(all, ids...)
	}
	t.mu.Unlock()

	const workers = 8
	ch := make(chan *url.TxID)
	var wg sync.WaitGroup
	var mu sync.Mutex
	pending := 0

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range ch {
				if !t.delivered(ctx, id) {
					mu.Lock()
					pending++
					mu.Unlock()
				}
			}
		}()
	}
	for _, id := range all {
		if ctx.Err() != nil {
			break
		}
		ch <- id
	}
	close(ch)
	wg.Wait()
	return pending
}

// reportStuck names the messages that never landed, and why, so a stuck run is
// diagnosable from its own output instead of by hand-querying afterwards.
func (t *tracker) reportStuck(ctx context.Context) {
	t.mu.Lock()
	roots := map[string][]*url.TxID{}
	for n, ids := range t.roots {
		roots[n] = ids
	}
	t.mu.Unlock()

	fmt.Println()
	fmt.Println("== undelivered messages ==")
	shown := 0
	for name, ids := range roots {
		for _, id := range ids {
			if shown >= 20 {
				fmt.Println("  ... (truncated)")
				return
			}
			r, err := t.Q.QueryMessage(ctx, id, nil)
			switch {
			case err != nil:
				fmt.Printf("  %-24s %v: not found (%v)\n", name, id, err)
			case r.Status.Delivered():
				continue
			default:
				why := r.Status.String()
				if r.Error != nil {
					why = r.Error.Error()
				}
				fmt.Printf("  %-24s %v: %s\n", name, id, why)
			}
			shown++
		}
	}
	if shown == 0 {
		fmt.Println("  (none)")
	}
}

func (t *tracker) delivered(ctx context.Context, id *url.TxID) bool {
	r, err := t.Q.QueryMessage(ctx, id, nil)
	if err != nil {
		return false
	}
	return r.Status.Delivered()
}

// reportMix is the primary report: what was generated, and did it land.
func (e *env) reportMix() {
	e.track.mu.Lock()
	names := make([]string, 0, len(e.track.roots))
	for n := range e.track.roots {
		names = append(names, n)
	}
	for n := range e.track.failures {
		if _, ok := e.track.roots[n]; !ok {
			names = append(names, n)
		}
	}
	for n := range e.track.skips {
		if _, ok := e.track.roots[n]; !ok {
			if _, ok := e.track.failures[n]; !ok {
				names = append(names, n)
			}
		}
	}
	e.track.mu.Unlock()
	sort.Strings(names)

	fmt.Println()
	fmt.Println("== transaction mix ==")
	for _, n := range names {
		e.track.mu.Lock()
		followed := len(e.track.roots[n])
		failed := e.track.failures[n]
		skipped := e.track.skips[n]
		e.track.mu.Unlock()
		mark := " "
		if failed > 0 {
			mark = "!"
		}
		fmt.Printf(" %s %-28s followed=%-6d rejected=%-5d skipped=%d\n", mark, n, followed, failed, skipped)
	}

	adis, books, pages, accounts, issuers := e.u.counts()
	fmt.Println()
	fmt.Println("== accounts created ==")
	fmt.Printf("  identities=%d key-books=%d key-pages=%d token-issuers=%d accounts=%d\n",
		adis, books, pages, issuers, accounts)
	e.reportPageShapes()
	e.reportTokens()
}

// reportPageShapes summarises the range of key counts and thresholds the run
// produced, which is the point of generating them.
func (e *env) reportPageShapes() {
	e.u.mu.Lock()
	defer e.u.mu.Unlock()

	byKeys := map[int]int{}
	byThreshold := map[uint64]int{}
	for _, a := range e.u.adis {
		for _, b := range a.books {
			for _, p := range b.pages {
				byKeys[len(p.keys)]++
				byThreshold[p.threshold]++
			}
		}
	}
	if len(byKeys) == 0 {
		return
	}
	fmt.Printf("  key-page shapes: %s\n", histogram(byKeys))
	fmt.Printf("  thresholds:      %s\n", histogramU(byThreshold))
}

// reportTokens summarises the custom tokens created, so "created a token" can
// never again mean "created and abandoned".
func (e *env) reportTokens() {
	e.u.mu.Lock()
	defer e.u.mu.Unlock()

	byPrecision := map[int]int{}
	total, limited, holders := 0, 0, 0
	for _, a := range e.u.adis {
		for _, t := range a.issuers {
			total++
			byPrecision[int(t.precision)]++
			holders += len(t.accounts)
			if t.limited {
				limited++
			}
		}
	}
	if total == 0 {
		return
	}
	fmt.Printf("  custom tokens:   %d (%d supply-limited), %d holding accounts\n", total, limited, holders)
	fmt.Printf("  token precision: %s\n", histogramPrec(byPrecision))

	// Name a sample of what was created. A run that reports counts but no URLs
	// cannot be checked against the chain by anyone but itself.
	shown := 0
	for _, a := range e.u.adis {
		for _, t := range a.issuers {
			if shown >= 2 {
				return
			}
			fmt.Printf("  e.g. %v (%s, precision %d) held by %v\n", t.url, t.symbol, t.precision, t.accounts)
			shown++
		}
	}
}

func histogramPrec(m map[int]int) string {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("p%d=%d", k, m[k]))
	}
	return strings.Join(parts, " ")
}

func histogram(m map[int]int) string {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%dkey=%d", k, m[k]))
	}
	return strings.Join(parts, " ")
}

func histogramU(m map[uint64]int) string {
	keys := make([]uint64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("t%d=%d", k, m[k]))
	}
	return strings.Join(parts, " ")
}

// reportPartitions is an observation, not an input: it shows where the
// generated accounts happened to land. Generation never consults this.
func (e *env) reportPartitions(ctx context.Context) {
	ns, err := e.c.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: "Directory"})
	if err != nil {
		log.Printf("partition report: network status: %v", err)
		return
	}
	tree, err := routing.NewRouteTree(ns.Routing)
	if err != nil {
		log.Printf("partition report: route tree: %v", err)
		return
	}

	counts := map[string]int{}
	route := func(u *url.URL) {
		if p, err := tree.Route(u); err == nil {
			counts[p]++
		}
	}

	e.u.mu.Lock()
	for _, l := range e.u.lites {
		route(l.acct)
	}
	for _, a := range e.u.adis {
		route(a.url)
		for _, u := range a.tokens {
			route(u)
		}
		for _, u := range a.data {
			route(u)
		}
	}
	e.u.mu.Unlock()

	fmt.Println()
	fmt.Println("== account distribution across partitions ==")
	parts := make([]string, 0, len(counts))
	for p := range counts {
		parts = append(parts, p)
	}
	sort.Strings(parts)
	total := 0
	for _, p := range parts {
		total += counts[p]
	}
	for _, p := range parts {
		pct := 0.0
		if total > 0 {
			pct = 100 * float64(counts[p]) / float64(total)
		}
		fmt.Printf("  %-12s %-6d (%.1f%%)\n", p, counts[p], pct)
	}
}

// reportHeals asks every validator for its own heal counters. This is why the
// tool talks to ConsensusStatus directly rather than leaving it to a shell
// pipeline: the counters are per node, per partition, and only the node itself
// can report its own.
func (e *env) reportHeals(ctx context.Context) {
	ns, err := e.c.NetworkStatus(ctx, api.NetworkStatusOptions{Partition: "Directory"})
	if err != nil {
		log.Printf("heal report: network status: %v", err)
		return
	}

	fmt.Println()
	fmt.Println("== heal counters ==")

	any := false
	for _, p := range ns.Network.Partitions {
		st, err := e.c.ConsensusStatus(ctx, api.ConsensusStatusOptions{Partition: p.ID})
		switch {
		case errors.Is(err, errors.NotFound):
			continue
		case err != nil:
			continue
		case st == nil:
			continue
		}
		if st.SyntheticHeals == 0 && st.AnchorHeals == 0 {
			continue
		}
		any = true
		fmt.Printf("  %-12s syntheticHeals=%-6d anchorHeals=%d\n", p.ID, st.SyntheticHeals, st.AnchorHeals)
	}
	if !any {
		fmt.Println("  (none recorded on the node this tool is talking to)")
	}
}

// --- waiting helpers ------------------------------------------------------

func (e *env) awaitDelivery(ctx context.Context, id *url.TxID, limit time.Duration) {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		r, err := e.Q.QueryMessage(ctx, id, nil)
		if err == nil && r.Status.Delivered() {
			return
		}
		time.Sleep(time.Second)
	}
}

func (e *env) awaitAll(ctx context.Context, ids []*url.TxID, limit time.Duration) {
	for _, id := range ids {
		e.awaitDelivery(ctx, id, limit)
	}
}

// awaitAccount blocks until the account exists. Account creation is the one
// place the generator has to wait, because everything built under an identity
// depends on the identity being there.
func (e *env) awaitAccount(ctx context.Context, u *url.URL, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, err := e.Q.QueryAccount(ctx, u, nil)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, errors.NotFound):
		default:
			return errors.UnknownError.WithFormat("query %v: %w", u, err)
		}
		time.Sleep(2 * time.Second)
	}
	return errors.NotReady.WithFormat("timed out waiting for %v", u)
}

// acmeBalance reports an account's ACME balance in whole tokens, or 0.
func (e *env) acmeBalance(ctx context.Context, u *url.URL) int64 {
	r, err := e.Q.QueryAccount(ctx, u, nil)
	if err != nil {
		return 0
	}
	a, ok := r.Account.(interface{ TokenBalance() *big.Int })
	if !ok {
		return 0
	}
	return new(big.Int).Div(a.TokenBalance(), big.NewInt(protocol.AcmePrecision)).Int64()
}

// creditBalance reports an account's credit balance, or 0.
func (e *env) creditBalance(ctx context.Context, u *url.URL) uint64 {
	r, err := e.Q.QueryAccount(ctx, u, nil)
	if err != nil {
		return 0
	}
	a, ok := r.Account.(interface{ GetCreditBalance() uint64 })
	if !ok {
		return 0
	}
	return a.GetCreditBalance()
}

// hasCredits is a single non-blocking check, for callers that poll on their
// own schedule rather than blocking.
func (e *env) hasCredits(ctx context.Context, u *url.URL) bool {
	r, err := e.Q.QueryAccount(ctx, u, nil)
	if err != nil {
		return false
	}
	a, ok := r.Account.(interface{ GetCreditBalance() uint64 })
	return ok && a.GetCreditBalance() > 0
}

func (e *env) awaitCredits(ctx context.Context, u *url.URL, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		r, err := e.Q.QueryAccount(ctx, u, nil)
		if err == nil {
			if a, ok := r.Account.(interface{ GetCreditBalance() uint64 }); ok && a.GetCreditBalance() > 0 {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return errors.NotReady.WithFormat("timed out waiting for %v to hold credits", u)
}
