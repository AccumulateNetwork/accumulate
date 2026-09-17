// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Nothing enters the generator's model because a transaction was submitted.
// Submitting is an intention; until the effect can be read back the account
// holds nothing and the key is not on the page. Acting on the intention
// produces transactions that cannot succeed: in the 10 tps run behind #4271,
// a third of every transfer failed "insufficient balance: have 0" because the
// destination was advertised as a funding SOURCE the instant a transfer was
// submitted toward it, and one premature mark seeded the next.
//
// The model already knows the rule. liteAccount.ready is earned by
// observation — promoteLites queries the account, buys credits, and only then
// marks it ready — and the growth path learned the same lesson as #4130:
// "fund it BEFORE advertising it, and wait for the money to land ... existence
// is not solvency". This file holds the passes that make every other fact in
// the model earn its place the same way.
//
// None of it blocks the pacer: actions submit and move on, and these passes
// run on their own goroutine. None of it reads a transaction status either —
// ids come back nil on the DAG-BFT line (#4131), so state is the only common
// denominator, and state is what the model actually needs to know.

const (
	// confirmPoll is how often the passes run. A fraction of the block time on
	// either line, so a fact is usable within a block or so of becoming true.
	confirmPoll = 3 * time.Second

	// retireSample bounds the cost of looking for accounts that have gone dry.
	// Re-reading every funded account every pass would grow without limit;
	// sampling retires them within a few passes at constant cost (#4256).
	retireSample = 16

	// pageSyncSample bounds how many identities have their pages re-read per
	// pass, for the same reason.
	pageSyncSample = 4

	// confirmSettle is how long a balance read is distrusted after the
	// generator itself has spent from the account. A read taken while its own
	// send is still in flight shows the money still there, and promoting on
	// that number spends it twice — the same mistake as trusting a submission,
	// one step further along.
	confirmSettle = 8 * time.Second
)

// sendUnits is one transfer, in the smallest ACME unit.
var sendUnits = int64(math.Round(sendAmount * protocol.AcmePrecision))

// observeFunds records what the network showed the account holding, and makes
// it a source if that covers a transfer.
//
// spendable only ever RISES from an observation and only ever falls by what
// the generator itself has sent. The local mirror cannot be used for this: it
// credits a recipient when a transfer is SUBMITTED, so deposits that never
// landed inflate it — and an account funded only by failed sends looks solvent
// forever. That is the same error as #4271 one level down, and it is why the
// number an account is trusted for comes from the chain and nowhere else.
func (e *env) observeFunds(l *liteAccount, units int64) {
	e.u.mu.Lock()
	defer e.u.mu.Unlock()
	if time.Since(l.lastSpend) < confirmSettle {
		return // its own send is still in flight; the read is stale
	}
	if units > l.spendable {
		l.spendable = units
	}
	l.funded = l.spendable >= sendUnits
}

// claimSend reserves one transfer against what the account was last observed
// to hold, and stops it being drawn as a source when nothing is left. The
// account stays in the universe: confirmFunding promotes it again if a later
// deposit lands.
func (e *env) claimSend(l *liteAccount) bool {
	e.u.mu.Lock()
	defer e.u.mu.Unlock()
	if l.spendable < sendUnits {
		l.funded = false
		return false
	}
	l.spendable -= sendUnits
	l.lastSpend = time.Now()
	if l.spendable < sendUnits {
		l.funded = false
	}
	return true
}

// confirmLoop keeps the model's facts tied to what the network shows.
func (e *env) confirmLoop(ctx context.Context) {
	for ctx.Err() == nil {
		e.confirmFunding(ctx)
		e.retireDryLites(ctx)
		e.syncPages(ctx)

		select {
		case <-ctx.Done():
			return
		case <-time.After(confirmPoll):
		}
	}
}

// confirmFunding promotes a lite to a distribution source once the network
// shows it holding tokens — never because a transfer was aimed at it.
func (e *env) confirmFunding(ctx context.Context) {
	e.u.mu.Lock()
	var cand []*liteAccount
	for _, l := range e.u.lites {
		if !l.funded {
			cand = append(cand, l)
		}
	}
	e.u.mu.Unlock()

	for _, l := range cand {
		if ctx.Err() != nil {
			return
		}
		if units, ok := e.tokenUnits(ctx, l.acct); ok {
			e.observeFunds(l, units)
		}
	}
}

// retireDryLites demotes a funded lite the network shows empty, so the
// generator stops drawing a source that can no longer pay (#4256). Sampled,
// because the funded population only grows.
func (e *env) retireDryLites(ctx context.Context) {
	e.u.mu.Lock()
	var cand []*liteAccount
	for _, l := range e.u.lites {
		if l.funded {
			cand = append(cand, l)
		}
	}
	e.u.mu.Unlock()

	for i := 0; i < retireSample && len(cand) > 0; i++ {
		if ctx.Err() != nil {
			return
		}
		l := cand[e.u.intn(len(cand))]
		units, ok := e.tokenUnits(ctx, l.acct)
		if !ok {
			units = 0
		}
		// Re-anchor on the chain's number: dead reckoning drifts, and this is
		// the only place the figure is corrected downward. Skipped while the
		// account's own send is in flight, for the same reason as above.
		e.u.mu.Lock()
		if time.Since(l.lastSpend) >= confirmSettle {
			l.spendable = units
			l.funded = units >= sendUnits
		}
		e.u.mu.Unlock()
	}
}

// syncPages replaces what the model believes about a key page with what the
// page holds. A key the chain does not list is dropped, so remove-page-key
// cannot ask for an entry that was never added, and set-threshold cannot ask
// for more signatures than there are keys — both of which the run behind
// #4271 produced. The page's own version is adopted at the same time, so a
// signature carries the version the network will accept.
func (e *env) syncPages(ctx context.Context) {
	e.u.mu.Lock()
	var cand []*identity
	cand = append(cand, e.u.adis...)
	e.u.mu.Unlock()

	for i := 0; i < pageSyncSample && len(cand) > 0; i++ {
		if ctx.Err() != nil {
			return
		}
		a := cand[e.u.intn(len(cand))]

		e.u.mu.Lock()
		var pages []*keyPage
		for _, b := range a.books {
			pages = append(pages, b.pages...)
		}
		e.u.mu.Unlock()

		for _, p := range pages {
			if ctx.Err() != nil {
				return
			}
			e.syncPage(ctx, p)
		}
	}
}

func (e *env) syncPage(ctx context.Context, p *keyPage) {
	r, err := e.Q.QueryAccount(ctx, p.url, nil)
	if err != nil {
		return
	}
	onChain, ok := r.Account.(*protocol.KeyPage)
	if !ok {
		return
	}

	e.u.mu.Lock()
	defer e.u.mu.Unlock()

	// Keep only the keys the page actually lists. The generator holds private
	// keys; the page holds their hashes, and EntryByKey does that comparison.
	kept := p.keys[:0]
	for _, k := range p.keys {
		if _, _, found := onChain.EntryByKey(k[32:]); found {
			kept = append(kept, k)
		}
	}
	p.keys = kept

	// Adopt the page's own numbers rather than the ones the model counted.
	p.version = onChain.Version
	p.threshold = onChain.AcceptThreshold
}

// --- observers ------------------------------------------------------------

// tokenUnits reports the account's balance in the smallest ACME unit, and
// whether it could be read at all.
func (e *env) tokenUnits(ctx context.Context, u *url.URL) (int64, bool) {
	r, err := e.Q.QueryAccount(ctx, u, nil)
	if err != nil {
		return 0, false
	}
	a, ok := r.Account.(protocol.AccountWithTokens)
	if !ok {
		return 0, false
	}
	b := a.TokenBalance()
	if b == nil {
		return 0, true
	}
	if !b.IsInt64() {
		return math.MaxInt64, true
	}
	return b.Int64(), true
}

// --- outcomes -------------------------------------------------------------

// reportOutcomes says how the followed transactions actually ENDED.
//
// The delivery report cannot answer that. It asks whether a transaction was
// delivered, and protocol.TransactionStatus.Delivered() is defined as
// "Code == Delivered OR Failed()" — a FAILED transaction is delivered. It
// means "reached a final state", not "worked". So the run behind #4271
// printed "OK: every followed transaction was delivered" over 736 transfers
// while the node refused 741 of them for want of funds, and the same green
// line appears on both lines: on the DAG-BFT line because nothing can be
// followed at all (#4131 returns nil ids), and on the CometBFT line because
// everything followed was "delivered".
//
// A load generator that cannot say what failed cannot be used to judge a
// change, which is what #4256 asks for as offered / accepted / failed.
func (e *env) reportOutcomes(ctx context.Context) {
	e.track.mu.Lock()
	roots := map[string][]*url.TxID{}
	total := 0
	for n, ids := range e.track.roots {
		roots[n] = ids
		total += len(ids)
	}
	e.track.mu.Unlock()

	fmt.Println()
	fmt.Println("== execution outcomes ==")
	if total == 0 {
		fmt.Println("  nothing could be followed: submission returned no transaction ids (#4131),")
		fmt.Println("  so this run cannot say what succeeded. Read the node's executor log.")
		return
	}

	type outcome struct {
		ok, failed, unknown int
		example             string
	}
	var mu sync.Mutex
	out := map[string]*outcome{}

	names := make([]string, 0, len(roots))
	for n := range roots {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, n := range names {
		o := new(outcome)
		out[n] = o

		ids := roots[n]
		ch := make(chan *url.TxID)
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for id := range ch {
					r, err := e.Q.QueryMessage(ctx, id, nil)
					mu.Lock()
					switch {
					case err != nil:
						o.unknown++
					case !r.Status.Success():
						// Status.Delivered() would be TRUE here: it is
						// "reached a final state", and an error code is final.
						// Success() is the question that was never asked.
						o.failed++
						if o.example == "" && r.Error != nil {
							o.example = r.Error.Message
						}
					default:
						o.ok++
					}
					mu.Unlock()
				}
			}()
		}
		for _, id := range ids {
			if ctx.Err() != nil {
				break
			}
			ch <- id
		}
		close(ch)
		wg.Wait()
	}

	totalFailed := 0
	for _, n := range names {
		o := out[n]
		totalFailed += o.failed
		mark := " "
		if o.failed > 0 {
			mark = "!"
		}
		line := fmt.Sprintf(" %s %-28s ok=%-6d failed=%-6d unknown=%d", mark, n, o.ok, o.failed, o.unknown)
		if o.example != "" {
			line += "  e.g. " + o.example
		}
		fmt.Println(line)
	}
	fmt.Printf("  %d of %d followed transactions FAILED after being accepted\n", totalFailed, total)
}
