// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestSyntheticStuckWhenRouteDead reproduces #4047 (reliable cross-partition
// synthetic delivery).
//
// Accumulate ALREADY auto-re-dispatches a transiently-dropped synthetic — see
// TestDropDeposit, which drops a deposit once and still ends with the recipient
// credited in full. So the reliability gap is NOT "no retry".
//
// The gap is the absence of a FALLBACK ROUTE. When the route to the destination
// partition is persistently unavailable — exactly what churn causes when a
// producer's known peers are stale/gone and discovery hasn't found live ones —
// re-dispatch retries the same dead route forever with nowhere else to go. The
// tokens are debited on the source and never credited on the destination: stuck
// in limbo, recoverable only by manual healing.
//
// This test models the dead route by persistently dropping the cross-partition
// SyntheticDepositTokens, then asserts the DESIRED behaviour (the deposit is
// eventually delivered by SOME path). It therefore FAILS on current code — that
// failure is the reproduction / baseline. It should pass once #4047 adds the
// DN-relay fallback + durable cross-route retry.
func TestSyntheticStuckWhenRouteDead(t *testing.T) {
	var timestamp uint64

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest

	// Persistently drop every synthetic token deposit: the destination
	// partition's route is dead from the producer's perspective. Anchors and
	// everything else flow normally.
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 1),
		simulator.GenesisWith(GenesisTime, globals),
		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
			msgs, err := env.Normalize()
			if err != nil {
				return false, err
			}
			for _, msg := range msgs {
			again:
				switch m := msg.(type) {
				case interface{ Unwrap() messaging.Message }:
					msg = m.Unwrap()
					goto again
				case messaging.MessageWithTransaction:
					if m.GetTransaction().Body.Type() == TransactionTypeSyntheticDepositTokens {
						return false, nil // drop — the route to the destination is dead
					}
				}
			}
			return true, nil
		}),
	)

	// Place alice and bob on DIFFERENT BVNs so the send produces a
	// cross-partition synthetic deposit.
	var alice, bob ed25519.PrivateKey
	var aliceUrl, bobUrl *url.URL
	for i := 0; ; i++ {
		require.Less(t, i, 100, "could not place alice and bob on different BVNs")
		alice = acctesting.GenerateKey("Alice", i)
		bob = acctesting.GenerateKey("Bob", i)
		aliceUrl = acctesting.AcmeLiteAddressStdPriv(alice)
		bobUrl = acctesting.AcmeLiteAddressStdPriv(bob)
		pa, _ := sim.Router().RouteAccount(aliceUrl)
		pb, _ := sim.Router().RouteAccount(bobUrl)
		if pa != pb {
			t.Logf("cross-partition: alice->%s bob->%s", pa, pb)
			break
		}
	}
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

	// alice sends to bob: the source transaction succeeds (alice is debited) and
	// produces a synthetic deposit destined for bob's BVN.
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(aliceUrl).
			SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
			SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	// Give the network ample time for any fallback path to deliver.
	sim.StepN(200)

	// DESIRED: bob received the deposit. On current code this FAILS — the deposit
	// has no route and no fallback, so bob's account is never even created (a
	// lite account springs into existence on first deposit) while alice was
	// already debited. Tokens stuck in limbo (#4047).
	var bal uint64
	View(t, sim.DatabaseFor(bobUrl), func(batch *database.Batch) {
		var acct *LiteTokenAccount
		if err := batch.Account(bobUrl).Main().GetAs(&acct); err == nil {
			bal = acct.Balance.Uint64()
		}
	})
	require.NotZero(t, bal,
		"synthetic deposit never delivered: no fallback route to an unreachable destination (#4047)")
}
