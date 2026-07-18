// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestSyntheticHealing reproduces a wedged synthetic stream — a single dropped
// synthetic message strands every later message behind it — and verifies that
// receiver-pull healing (#4064) recovers it: the destination detects the gap,
// pulls the missing message from the source partition, resubmits it, and the
// pending tail drains via the normal cascade.
func TestSyntheticHealing(t *testing.T) {
	var timestamp uint64

	// Drop the first synthetic deposit exactly once. Every later deposit's
	// synthetic will be received but stuck pending behind the missing one.
	var didDrop bool

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, globals),
		simulator.SkipProposalCheck(), // FIXME should not be necessary
		simulator.EnableSyntheticHealing(),

		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
			if didDrop {
				return true, nil
			}

			messages, err := env.Normalize()
			if err != nil {
				return false, err
			}

			for _, msg := range messages {
			again:
				switch m := msg.(type) {
				case interface{ Unwrap() messaging.Message }:
					msg = m.Unwrap()
					goto again
				case messaging.MessageWithTransaction:
					if m.GetTransaction().Body.Type() == TransactionTypeSyntheticDepositTokens {
						fmt.Printf("Dropping synthetic %X\n", m.GetTransaction().GetHash()[:4])
						didDrop = true
						return false, nil
					}
				}
			}
			return true, nil
		}),
	)

	healsBefore := gatherHealCount(t)

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey("Bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

	// Submit several deposits so that later synthetics pile up behind the
	// dropped one.
	st := make([]*protocol.TransactionStatus, 5)
	for i := range st {
		st[i] = sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(aliceUrl).
				SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
				SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
	}

	// Confirm the wedge actually formed.
	sim.StepUntil(True(func(*Harness) bool { return didDrop }))

	// All deposits — including the one whose synthetic was dropped — must
	// eventually deliver once healing pulls the missing message.
	for _, st := range st {
		sim.StepUntil(
			Txn(st.TxID).Succeeds(),
			Txn(st.TxID).Produced().Succeeds())
	}

	// The recipient received every deposit.
	lta := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(bobUrl), bobUrl)
	require.Equal(t, len(st)*protocol.AcmePrecision, int(lta.Balance.Uint64()))

	// And the recovery went through the receiver-pull healer (proving the fix
	// ran, not some other retry path).
	require.Greater(t, gatherHealCount(t), healsBefore, "expected synthetic healing to fire")
}

// gatherHealCount sums the accumulate_crosschain_heals_total counter across all
// label sets from the default Prometheus registry.
func gatherHealCount(t testing.TB) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	var total float64
	for _, mf := range mfs {
		if mf.GetName() != "accumulate_crosschain_heals_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			total += m.GetCounter().GetValue()
		}
	}
	return total
}
