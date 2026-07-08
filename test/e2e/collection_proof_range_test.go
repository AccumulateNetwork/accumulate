// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
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

// TestRangeRecovery drops a run of consecutive synthetic deposits between two
// partitions and verifies the destination recovers the whole run with a
// single SequenceRange call carrying ONE shared collection proof (#4048).
// Pre-activation (before VNext) the same scenario must recover through the
// per-message healing path, without any collection proofs on the wire.
func TestRangeRecovery(t *testing.T) {
	Run(t, map[string]ExecutorVersion{
		"activated": ExecutorVersionVNext,
		"fallback":  ExecutorVersionLatest,
	}, func(t *testing.T, version ExecutorVersion) {
		var timestamp uint64
		const transfers = 5
		const drops = 3

		// dropped counts synthetic-deposit envelopes deliberately dropped.
		// recovered counts synthetic messages that carry a collection proof
		// (ReceiptList) — only the range-recovery path produces those.
		var dropped, recovered atomic.Int32

		globals := new(core.GlobalValues)
		globals.ExecutorVersion = version
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 3, 1),
			simulator.GenesisWith(GenesisTime, globals),

			simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
				messages, err := env.Normalize()
				if err != nil {
					return false, err
				}

				for _, msg := range messages {
					// Recovered messages are counted, never dropped.
					if syn, ok := msg.(*messaging.SyntheticMessage); ok &&
						syn.Proof != nil && syn.Proof.ReceiptList != nil {
						recovered.Add(1)
						return true, nil
					}
				}

				if dropped.Load() >= drops {
					return true, nil
				}
				for _, msg := range messages {
				again:
					switch m := msg.(type) {
					case interface{ Unwrap() messaging.Message }:
						msg = m.Unwrap()
						goto again
					case messaging.MessageWithTransaction:
						if m.GetTransaction().Body.Type() == TransactionTypeSyntheticDepositTokens {
							dropped.Add(1)
							return false, nil
						}
					}
				}
				return true, nil
			}),
		)

		// Alice and Bob must live on different partitions so the deposits are
		// cross-partition synthetic messages.
		alice := acctesting.GenerateKey("Alice")
		aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
		alicePart, err := sim.Router().RouteAccount(aliceUrl)
		require.NoError(t, err)
		var bobUrl *url.URL
		for i := 0; ; i++ {
			bob := acctesting.GenerateKey("Bob", i)
			bobUrl = acctesting.AcmeLiteAddressStdPriv(bob)
			bobPart, err := sim.Router().RouteAccount(bobUrl)
			require.NoError(t, err)
			if alicePart != bobPart {
				break
			}
		}
		MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

		// Execute. The first `drops` deposits are dropped, creating a run of
		// consecutive missing sequence numbers at Bob's partition.
		st := make([]*protocol.TransactionStatus, transfers)
		for i := range st {
			st[i] = sim.SubmitTxnSuccessfully(MustBuild(t,
				build.Transaction().For(aliceUrl).
					SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
					SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
		}
		sim.StepUntil(True(func(*Harness) bool { return dropped.Load() >= drops }))

		// Healing must recover the dropped run — every deposit executes.
		for _, st := range st {
			sim.StepUntilN(200,
				Txn(st.TxID).Succeeds(),
				Txn(st.TxID).Produced().Succeeds())
		}

		// Verify every token arrived exactly once
		lta := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(bobUrl), bobUrl)
		require.Equal(t, transfers*int(protocol.AcmePrecision), int(lta.Balance.Uint64()))

		if version.VNextEnabled() {
			// The dropped run must have been recovered via the range path:
			// each resubmitted message carries the shared collection proof.
			require.GreaterOrEqual(t, int(recovered.Load()), drops,
				"expected the dropped run to be recovered with collection proofs")
		} else {
			// Pre-activation there must be no collection proofs on the wire.
			require.Zero(t, recovered.Load(),
				"collection proofs must not be used before activation")
		}
	})
}
