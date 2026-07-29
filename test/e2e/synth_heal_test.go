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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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
	const drops = 3 // a RUN of consecutive losses must heal batched (#4067)
	var dropped int

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, globals),
		simulator.SkipProposalCheck(), // FIXME should not be necessary

		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
			if dropped >= drops {
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
						dropped++
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
	sim.StepUntil(True(func(*Harness) bool { return dropped >= drops }))

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

// TestSyntheticHealingSignatureRequest reproduces #4066 finding 1: a healed
// synthetic that is a MessageForTransaction (here a SignatureRequest) must
// bundle the companion transaction, because in a wedged stream the destination
// may never have received it. Alice initiates a transaction requiring bob's
// authority; the SignatureRequest synthetic to bob's partition is dropped, so
// only healing (with the companion transaction) can deliver it — proven by
// bob's authority signature completing (with a rejection memo, since bob's
// book does not exist).
func TestSyntheticHealingSignatureRequest(t *testing.T) {
	var timestamp uint64
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)

	var didDrop bool
	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, globals),
		simulator.SkipProposalCheck(),

		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
			if didDrop {
				return true, nil
			}
			messages, err := env.Normalize()
			if err != nil {
				return false, err
			}
			for _, msg := range messages {
				syn, ok := msg.(*messaging.SyntheticMessage)
				if !ok {
					continue
				}
				seq, ok := syn.Message.(*messaging.SequencedMessage)
				if !ok {
					continue
				}
				if _, ok := seq.Message.(*messaging.SignatureRequest); ok {
					fmt.Printf("Dropping signature request %X\n", seq.Message.Hash())
					didDrop = true
					return false, nil
				}
			}
			return true, nil
		}),
	)

	// Bob must live on a different partition than alice so the signature
	// request is a cross-partition synthetic.
	alicePart, err := sim.Router().RouteAccount(alice)
	require.NoError(t, err)
	var bob *url.URL
	for i := 0; ; i++ {
		bob = AccountUrl(fmt.Sprintf("bob%d", i))
		bobPart, err := sim.Router().RouteAccount(bob)
		require.NoError(t, err)
		if bobPart != alicePart {
			break
		}
	}

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	// DO NOT CREATE BOB — his authority signature carries a rejection memo,
	// which can only happen if the signature request DELIVERS on his partition.

	healsBefore := gatherHealCount(t)

	st := sim.BuildAndSubmitSuccessfully(
		build.Transaction().For(alice).
			CreateTokenAccount(alice, "tokens").ForToken(AcmeUrl()).
			WithAuthority(bob, "book").
			SignWith(alice, "book", "1").Version(1).Timestamp(&timestamp).PrivateKey(aliceKey))

	sim.StepUntil(True(func(*Harness) bool { return didDrop }))

	// Receiver-pull needs a LATER message on the same stream to see the gap
	// (Received > Delivered) — the incident shape. Send a deposit across the
	// same partition pair so the dropped signature request becomes a visible
	// hole instead of a silent trailing loss.
	var senderKey, recvKey []byte
	for i := 0; ; i++ {
		k := acctesting.GenerateKey("sender", i)
		part, err := sim.Router().RouteAccount(acctesting.AcmeLiteAddressStdPriv(k))
		require.NoError(t, err)
		if part == alicePart {
			senderKey = k
			break
		}
	}
	bobPart, err := sim.Router().RouteAccount(bob)
	require.NoError(t, err)
	for i := 0; ; i++ {
		k := acctesting.GenerateKey("recv", i)
		part, err := sim.Router().RouteAccount(acctesting.AcmeLiteAddressStdPriv(k))
		require.NoError(t, err)
		if part == bobPart {
			recvKey = k
			break
		}
	}
	senderUrl := acctesting.AcmeLiteAddressStdPriv(senderKey)
	MakeLiteTokenAccount(t, sim.DatabaseFor(senderUrl), senderKey[32:], AcmeUrl())
	var ts2 uint64
	sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(senderUrl).
			SendTokens(1, protocol.AcmePrecisionPower).To(acctesting.AcmeLiteAddressStdPriv(recvKey)).
			SignWith(senderUrl).Version(1).Timestamp(&ts2).PrivateKey(senderKey)))

	// The healer must fire
	sim.StepUntilN(200, True(func(*Harness) bool { return gatherHealCount(t) > healsBefore }))

	// The dropped signature request must be healed — with its companion
	// transaction — for bob's partition to process it and answer.
	sim.StepUntilN(400,
		Txn(st[0].TxID).Fails().WithError(errors.Rejected),
		Sig(st[1].TxID).SignatureRequestTo(bob, "book").AuthoritySignature().Completes())

	require.Greater(t, gatherHealCount(t), healsBefore, "expected synthetic healing to fire")
}

// TestSyntheticHealingLostPrefix reproduces #4073: a stream whose FIRST message
// is lost and which has nothing following it.
//
// This is the case every gap-based recovery path is blind to. A dropped message
// with later traffic behind it leaves a nil hole in the destination's pending
// window, which is what TestSyntheticHealing exercises. Here the drop is the
// whole stream: the destination never receives anything, so Received stays 0,
// Pending stays empty, and Received > Delivered is never true. The stream is
// indistinguishable from a healthy idle one, and before the interval reconcile
// it stayed broken forever — observed live as DN->BVN1 stuck at produced=2
// received=0 for 23 hours of a chaos soak.
//
// Recovery here can only come from asking the source what it produced.
func TestSyntheticHealingLostPrefix(t *testing.T) {
	var timestamp uint64

	// Drop the one and only synthetic this test produces, so nothing ever
	// follows it. This is what makes the case undetectable by a gap scan.
	var dropped int
	const drops = 1

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, globals),
		simulator.SkipProposalCheck(), // FIXME should not be necessary

		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
			if dropped >= drops {
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
						dropped++
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

	// Exactly one deposit. Its synthetic is dropped, and no further traffic on
	// this stream ever exposes the hole.
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(aliceUrl).
			SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
			SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))

	sim.StepUntil(True(func(*Harness) bool { return dropped >= drops }))

	// The destination's ledger must show the shape that defeats a gap scan:
	// nothing received, nothing pending. If this ever fails the test has stopped
	// reproducing #4073 and is just re-testing #4064.
	sim.Step()
	for _, p := range sim.S.Partitions() {
		View(t, sim.S.Database(p.ID), func(batch *database.Batch) {
			var ledger *protocol.SyntheticLedger
			err := batch.Account(protocol.PartitionUrl(p.ID).JoinPath(protocol.Synthetic)).Main().GetAs(&ledger)
			if err != nil {
				return // no ledger yet is itself the lost-prefix shape
			}
			for _, part := range ledger.Sequence {
				require.Zero(t, len(part.Pending),
					"%s has a pending entry, so this is a gap case (#4064), not a lost prefix (#4073)", p.ID)
			}
		})
	}

	// Only the interval reconcile can recover this, and it deliberately waits
	// reconcileGraceBlocks before believing a gap — so allow more than
	// StepUntil's default 50 steps. If this ever needs raising again, the grace
	// grew; that is a real change in recovery latency, not a flaky test.
	sim.StepUntilN(3*60, // 3x reconcileGraceBlocks
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())

	lta := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(bobUrl), bobUrl)
	require.Equal(t, int(protocol.AcmePrecision), int(lta.Balance.Uint64()))
	require.Greater(t, gatherHealCount(t), healsBefore, "expected healing to fire")
}

// gatherCounter sums a Prometheus counter, optionally filtered to one label value.
func gatherCounter(t testing.TB, name, label, value string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	var total float64
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if label != "" {
				ok := false
				for _, l := range m.GetLabel() {
					if l.GetName() == label && l.GetValue() == value {
						ok = true
					}
				}
				if !ok {
					continue
				}
			}
			total += m.GetCounter().GetValue()
		}
	}
	return total
}

// TestReconcileDoesNotRaceNormalDelivery pins the defect that #4073's first
// implementation shipped with.
//
// The reconcile compares the source's Produced against our Received. On a
// healthy network that comparison is routinely, transiently true: a message
// counts as produced the moment the source emits it, and it has not reached us
// yet. Treating that as loss makes the reconcile request messages that are
// simply in flight — and the source cannot even build a receipt for them,
// because they are not anchored yet, so every such pull fails with
// "locate anchor index chain entry for block N: reached the end of the chain".
//
// With nothing dropped there is nothing to recover, so a correct reconcile must
// attempt ZERO pulls. Any attempt here is the mechanism racing normal delivery.
//
// This is asserted on ATTEMPTS, not successes: the original counter only
// incremented on success, so a pull path that failed 100% of the time reported
// the same zero as one with nothing to do. That is why six soak runs looked
// clean while the fix was completely broken.
func TestReconcileDoesNotRaceNormalDelivery(t *testing.T) {
	var timestamp uint64

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, globals),
		simulator.SkipProposalCheck(),
	)

	before := gatherCounter(t, "accumulate_crosschain_reconcile_pulls_total", "outcome", "attempted")

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey("Bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

	// Ordinary cross-partition traffic. Nothing is dropped.
	for i := 0; i < 5; i++ {
		st := sim.SubmitTxnSuccessfully(MustBuild(t,
			build.Transaction().For(aliceUrl).
				SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
				SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
		sim.StepUntil(
			Txn(st.TxID).Succeeds(),
			Txn(st.TxID).Produced().Succeeds())
	}

	attempted := gatherCounter(t, "accumulate_crosschain_reconcile_pulls_total", "outcome", "attempted") - before
	require.Zero(t, attempted,
		"reconcile attempted %v pull(s) with nothing dropped — it is racing normal "+
			"delivery and requesting messages that are merely in flight", attempted)
}
