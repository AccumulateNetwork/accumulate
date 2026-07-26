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
	for _, l := range lites {
		l := l
		if _, err := e.submitAsTreasury(ctx, func() txBuilder {
			return e.build(e.treasury).
				SendTokens(subTreasuryAcme, protocol.AcmePrecisionPower).To(l.acct).
				SignWith(e.treasury.id).Version(1).Timestamp(e.nonce.next()).PrivateKey(e.treasury.key)
		}); err != nil {
			return fmt.Errorf("fund sub-treasury %v: %w", l.acct, err)
		}
	}
	for _, l := range lites {
		if err := e.awaitAccount(ctx, l.acct, 5*time.Minute); err != nil {
			return fmt.Errorf("sub-treasury %v never funded: %w", l.acct, err)
		}
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
