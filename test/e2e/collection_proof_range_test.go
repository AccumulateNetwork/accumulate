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
		"activated": ExecutorVersionLatest,
		"fallback":  ExecutorVersionV2Tanegashima,
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

		if version.V2KourouEnabled() {
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

// TestAnchorRangeRecovery drops every copy of the directory's first anchor
// and verifies the destinations recover it with a proof-authorized anchor
// (#4056) — no signature quorum is re-gathered. The network runs THREE
// validators per partition, which signature-quorum anchor healing cannot
// handle (see TestMissingDirectoryAnchorTxn's single-validator restriction):
// quorum healing needs 2f+1 validators to each independently re-sign and
// resubmit the missed anchor, while a collection proof only needs the current
// directory root, which every synced node already has.
func TestAnchorRangeRecovery(t *testing.T) {
	var timestamp uint64

	// dropped counts copies of anchor #1 that were dropped. recovered counts
	// proof-authorized anchors — only the range-recovery path produces those.
	var dropped, recovered atomic.Int32

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.GenesisWith(GenesisTime, globals),

		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
			messages, err := env.Normalize()
			if err != nil {
				return false, err
			}

			var drop bool
			for _, msg := range messages {
				anchor, ok := msg.(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				if anchor.Proof != nil {
					// A proof-authorized recovery — count it, never drop it
					recovered.Add(1)
					continue
				}
				seq, ok := anchor.Anchor.(*messaging.SequencedMessage)
				if !ok {
					continue
				}
				txn, ok := seq.Message.(*messaging.TransactionMessage)
				if !ok {
					continue
				}
				// Drop every copy of the directory's first anchor, from every
				// validator to every destination
				if txn.Transaction.Body.Type() == TransactionTypeDirectoryAnchor && seq.Number == 1 {
					dropped.Add(1)
					drop = true
				}
			}
			return !drop, nil
		}),
	)

	// Alice and Bob must live on different partitions so the deposit needs
	// cross-partition anchoring to complete.
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

	// Execute a cross-partition transfer. Its deposit cannot be delivered
	// until the destination has the directory anchors covering it, so this
	// completes only if the dropped anchor is recovered.
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(aliceUrl).
			SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
			SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))

	sim.StepUntil(True(func(*Harness) bool { return dropped.Load() > 0 }))

	sim.StepUntilN(400,
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	// The token arrived and at least one anchor was recovered by proof
	lta := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(bobUrl), bobUrl)
	require.Equal(t, int(protocol.AcmePrecision), int(lta.Balance.Uint64()))
	require.Greater(t, int(recovered.Load()), 0,
		"expected the dropped anchor to be recovered with a collection proof")
}
