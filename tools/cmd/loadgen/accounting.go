// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"log"
	"math/big"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// acct is the loadgen's local mirror of one on-chain account.
//
//   - tokens is dead-reckoned EXACTLY: the generator owns every amount it moves
//     and token transfers carry no token-denominated fee, so deposits minus sends
//     is exact.
//   - credits is dead-reckoned from the protocol fee schedule (ComputeTransactionFee)
//     and drifts — refunds on failure, oracle moves, held/rejected txns — so it is
//     periodically reconciled against the chain.
//   - txns counts how many transactions this account was the principal of.
type acct struct {
	kind    string
	tokens  *big.Int // smallest ACME unit
	credits int64    // credit-units (may drift; reconciled)
	txns    uint64
}

// ledger mirrors every account the loadgen touches. It is OBSERVE-ONLY: it
// records and reconciles but does not (yet) drive selection, so a bug here can
// only skew a report, never break the workload — record() is panic-guarded.
type ledger struct {
	mu   sync.Mutex
	m    map[string]*acct
	fees *protocol.FeeSchedule
	// maxDrift is the largest credit discrepancy reconciliation has corrected,
	// so a growing number flags that dead reckoning is losing accuracy.
	maxDrift int64
}

func newLedger(fees *protocol.FeeSchedule) *ledger {
	return &ledger{m: map[string]*acct{}, fees: fees}
}

// get returns the entry for u, creating it as `kind` if absent. Caller holds mu.
func (l *ledger) get(u *url.URL, kind string) *acct {
	if u == nil {
		return nil
	}
	k := u.String()
	a := l.m[k]
	if a == nil {
		a = &acct{kind: kind, tokens: new(big.Int)}
		l.m[k] = a
	}
	return a
}

// record folds a just-submitted envelope into the local mirror: it counts the
// transaction, debits the signer's credits by the computed fee, and applies the
// value movement of the value-moving bodies. Best-effort and panic-safe: any
// surprise (new body shape, nil field) is swallowed so accounting never affects
// the load.
func (l *ledger) record(env *messaging.Envelope) {
	if l == nil || env == nil {
		return
	}
	defer func() { _ = recover() }()

	l.mu.Lock()
	defer l.mu.Unlock()

	// The signer pays the fee. For the loadgen's transactions there is one
	// signer per envelope; attributing the whole fee to it is exact for lite
	// accounts and close for pages (reconciliation corrects the rest).
	var signer *url.URL
	if len(env.Signatures) > 0 {
		signer = env.Signatures[0].GetSigner()
	}

	for _, tx := range env.Transaction {
		if tx == nil || tx.Body == nil {
			continue
		}
		principal := tx.Header.Principal
		if p := l.get(principal, "account"); p != nil {
			p.txns++
		}

		if l.fees != nil {
			if fee, err := l.fees.ComputeTransactionFee(tx); err == nil && fee > 0 {
				payer := signer
				if payer == nil {
					payer = principal
				}
				if a := l.get(payer, "signer"); a != nil {
					a.credits -= int64(fee)
				}
			}
		}

		switch body := tx.Body.(type) {
		case *protocol.SendTokens:
			from := l.get(principal, "token")
			for _, to := range body.To {
				if to == nil {
					continue
				}
				if from != nil {
					from.tokens.Sub(from.tokens, &to.Amount)
				}
				if d := l.get(to.Url, "token"); d != nil {
					d.tokens.Add(d.tokens, &to.Amount)
				}
			}
		case *protocol.AddCredits:
			// ACME is burned from the principal; credits land on the recipient.
			if from := l.get(principal, "token"); from != nil {
				from.tokens.Sub(from.tokens, &body.Amount)
			}
			// credit-units ≈ acme-units * oracle / oracle-precision.
			if body.Oracle > 0 {
				cu := new(big.Int).Mul(&body.Amount, big.NewInt(int64(body.Oracle)))
				cu.Div(cu, big.NewInt(protocol.AcmeOraclePrecision))
				if r := l.get(body.Recipient, "credits"); r != nil {
					r.credits += cu.Int64()
				}
			}
		case *protocol.BurnTokens:
			if from := l.get(principal, "token"); from != nil {
				from.tokens.Sub(from.tokens, &body.Amount)
			}
		}
	}
}

// summary returns aggregate mirror state for the report/stats.
func (l *ledger) summary() (accounts int, totalTxns uint64, maxDrift int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, a := range l.m {
		totalTxns += a.txns
	}
	return len(l.m), totalTxns, l.maxDrift
}

// reconcile periodically samples known accounts, queries their real token and
// credit balances, and corrects the mirror — the "check every now and again to
// keep the credit math honest" pass. Rotating sample: cheap and steady.
func (e *env) reconcile(ctx context.Context, every time.Duration, sample int) {
	if e.led == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}

		// Snapshot a random sample of keys under the lock, then query without it.
		e.led.mu.Lock()
		keys := make([]string, 0, len(e.led.m))
		for k := range e.led.m {
			keys = append(keys, k)
		}
		e.led.mu.Unlock()
		if len(keys) == 0 {
			continue
		}
		for i := 0; i < sample && i < len(keys); i++ {
			k := keys[e.u.intn(len(keys))]
			u, err := url.Parse(k)
			if err != nil {
				continue
			}
			// Credits live on the identity; ACME on the token account. Query
			// whichever the real balance call returns and correct the mirror.
			real := e.creditBalance(ctx, u)
			e.led.mu.Lock()
			if a := e.led.m[k]; a != nil {
				drift := a.credits - int64(real)
				if drift < 0 {
					drift = -drift
				}
				if drift > e.led.maxDrift {
					e.led.maxDrift = drift
				}
				a.credits = int64(real) // trust the chain
			}
			e.led.mu.Unlock()
		}
		acc, txns, maxDrift := e.led.summary()
		log.Printf("reconcile: %d accounts tracked, %d principal-txns, max credit drift %d units",
			acc, txns, maxDrift)
	}
}
