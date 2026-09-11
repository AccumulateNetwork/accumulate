// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
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
)

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
		if e.holdsTokens(ctx, l.acct) {
			e.u.markFunded(l)
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
		if !e.holdsTokens(ctx, l.acct) {
			e.u.mu.Lock()
			l.funded = false
			e.u.mu.Unlock()
		}
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

// holdsTokens reports whether the account exists and holds a positive balance.
func (e *env) holdsTokens(ctx context.Context, u *url.URL) bool {
	r, err := e.Q.QueryAccount(ctx, u, nil)
	if err != nil {
		return false
	}
	a, ok := r.Account.(protocol.AccountWithTokens)
	if !ok {
		return false
	}
	b := a.TokenBalance()
	return b != nil && b.Sign() > 0
}

// accountHasAuthority reports whether the account carries auth in its OWN
// authority set. An account created without one inherits its identity's
// authority and carries none of its own, which is why update-account-auth
// against it is refused as "not an authority" (#4271).
func (e *env) accountHasAuthority(ctx context.Context, u, auth *url.URL) bool {
	r, err := e.Q.QueryAccount(ctx, u, nil)
	if err != nil {
		return false
	}
	f, ok := r.Account.(protocol.FullAccount)
	if !ok {
		return false
	}
	_, found := f.GetAuth().GetAuthority(auth)
	return found
}
