// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

const (
	// subTreasuryAcme is the ACME funded into each bootstrap sub-treasury. Large
	// enough to relay millions of dust transfers (sendAmount) and buy its own
	// credits for the length of a run, so it acts as an independent funding
	// source rather than a one-shot recipient. 100 * this is a rounding error
	// against the genesis faucet's 200M ACME.
	subTreasuryAcme = 1000.0
	// subTreasuryCredits is the credit grant per sub-treasury so it can sign from
	// the first block without waiting on promoteLites.
	subTreasuryCredits = 5000
)

// bootstrapSubTreasuries seeds n well-funded, ready lite accounts BEFORE the
// workload starts, so the mix draws its funding sources from a base spread
// across every BVN from the very first transaction.
//
// Why: a lite account's key is random and routes ~evenly across the BVNs, so n
// sources land ≈ n/#BVNs per partition. The workload's random source selection
// then originates cross-partition synthetics from every BVN. Without this the
// treasury seeds a single lite on one partition and everything cascades from
// there, concentrating ~80% of synthetic production on one BVN (the funding-star
// bug) — which is what let one hot stream outrun healing under load.
//
// These sub-treasuries then fund everything else via the normal cascade, so the
// faucet/treasury stops being a per-account bottleneck. Growth continues
// concurrently after this returns: generate() runs the mix against the base
// while growAsync/promoteLites keep enlarging the set — the "both paths" the
// design calls for.
func (e *env) bootstrapSubTreasuries(ctx context.Context, n int) error {
	if n <= 0 {
		return nil
	}
	log.Printf("== bootstrapping %d sub-treasuries (%.0f ACME + %d credits each) ==",
		n, subTreasuryAcme, subTreasuryCredits)

	lites := make([]*liteAccount, n)
	for i := range lites {
		lites[i] = newLiteAccount(e.u.rng)
	}

	// Phase A — fund each from the treasury. Treasury-signed, so they serialize
	// on the treasury's signer (monotonic timestamps) but pipeline into the
	// mempool; the barrier waits for all deposits to land once, not one-by-one.
	// Keep the txid of each deposit. When one never lands, the useful question
	// is what happened to THAT transaction — and until now the timeout said
	// only "never funded", naming the account and nothing else. Every run of
	// the past week has hit this and none of them could say why.
	deposit := make(map[string]*url.TxID, len(lites))
	for _, l := range lites {
		l := l
		ids, err := e.submitAsTreasury(ctx, func() txBuilder {
			return e.build(e.treasury).
				SendTokens(subTreasuryAcme, protocol.AcmePrecisionPower).To(l.acct).
				SignWith(e.treasury.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(e.treasury.key)
		})
		if err != nil {
			return fmt.Errorf("fund sub-treasury %v: %w", l.acct, err)
		}
		if len(ids) > 0 {
			deposit[l.acct.String()] = ids[0]
		}
	}

	funded, stuck := 0, 0
	phaseA := time.Now()
	// ONE deadline for the whole phase, not one per account.
	//
	// Reporting every straggler instead of failing on the first is the right
	// behaviour, but with a 5-minute timeout EACH it turns 90 stuck deposits
	// into seven hours of bootstrap. Past the deadline, still check each
	// account — a quick look, so the count is accurate — but stop waiting.
	phaseDeadline := phaseA.Add(5 * time.Minute)
	for _, l := range lites {
		wait := time.Until(phaseDeadline)
		if wait < 2*time.Second {
			wait = 2 * time.Second
		}
		if err := e.awaitAccount(ctx, l.acct, wait); err != nil {
			stuck++
			// Say what was actually seen: which transaction, and what the
			// network says about it now.
			// Explain the first one in full; after that just count, or the
			// log becomes 90 copies of the same paragraph.
			if stuck == 1 {
				log.Printf("bootstrap: sub-treasury %v never funded after %v: %v (deposit %v: %s)",
					l.acct, time.Since(phaseA).Round(time.Second), err,
					deposit[l.acct.String()], e.describeTx(ctx, deposit[l.acct.String()]))
			}
			continue
		}
		funded++
	}
	log.Printf("bootstrap: phase A complete in %v — %d/%d sub-treasuries funded, %d never landed",
		time.Since(phaseA).Round(time.Second), funded, len(lites), stuck)
	if funded == 0 {
		return fmt.Errorf("no sub-treasury was funded (%d attempted)", len(lites))
	}

	// Phase B — buy credits for each so it can sign. A brand-new lite cannot pay
	// for its own first AddCredits (it holds no credits yet), so the treasury
	// buys them; from then on the sub-treasury buys its own from its ACME,
	// sourced from its own BVN — which is what spreads credit-burn traffic to the
	// DN across all BVNs.
	for _, l := range lites {
		l := l
		if _, err := e.submitAsTreasury(ctx, func() txBuilder {
			return e.build(e.treasury).
				AddCredits().WithOracle(e.oracle).Purchase(subTreasuryCredits).To(l.id).
				SignWith(e.treasury.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(e.treasury.key)
		}); err != nil {
			return fmt.Errorf("credit sub-treasury %v: %w", l.id, err)
		}
	}

	// Barrier + register: a sub-treasury joins the source pool only once it holds
	// credits (ready) and ACME (funded). A straggler — e.g. its credit deposit
	// was dropped and is still healing — is skipped, not fatal; the run just
	// starts with a few fewer sources.
	ready := 0
	for _, l := range lites {
		if err := e.awaitCredits(ctx, l.id, 5*time.Minute); err != nil {
			log.Printf("bootstrap: sub-treasury %v not ready (%v) — skipping", l.id, err)
			continue
		}
		e.u.addLite(l)
		e.u.markFunded(l)
		e.u.markReady(l)
		ready++
	}
	log.Printf("== %d/%d sub-treasuries ready as funding sources ==", ready, n)
	if ready == 0 {
		return fmt.Errorf("no sub-treasuries became ready")
	}
	return nil
}
