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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
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
	const drops = 3 // a RUN of consecutive losses must heal batched (#4067)
	var dropped int

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, globals),
		simulator.SkipProposalCheck(), // FIXME should not be necessary
		simulator.EnableSyntheticHealing(),

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

// TestSyntheticHeldForMissingAnchor reproduces the #4070 wedge: a synthetic that
// arrives before its proof anchor. Under network disruption the DN→BVN anchor is
// delayed, so the synthetic's DeliverTx anchor check fails. It must be HELD
// (recorded pending) and re-attempted IN PLACE when the anchor arrives — not
// terminally failed. Terminal failure wedges the stream permanently: the healer
// re-submits a byte-identical message, which is deduplicated and never
// re-processed. This restores the V1 hold-for-anchor behavior that V2 dropped.
func TestSyntheticHeldForMissingAnchor(t *testing.T) {
	var timestamp uint64
	var trap bool // while true, drop DN anchors destined to BVN1

	globals := new(core.GlobalValues)
	globals.ExecutorVersion = ExecutorVersionLatest // V2Jiuquan gates the hold
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 3, 1),
		simulator.GenesisWith(GenesisTime, globals),
		simulator.SkipProposalCheck(),

		// Drop directory anchors to BVN1 while trapping, so the deposit's synthetic
		// arrives at BVN1 with its proof anchor missing → the anchor check fails.
		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
			if !trap || len(env.Messages) != 1 {
				return true, nil
			}
			blk, ok := env.Messages[0].(*messaging.BlockAnchor)
			if !ok {
				return true, nil
			}
			seq, ok := blk.Anchor.(*messaging.SequencedMessage)
			if !ok {
				return true, nil
			}
			txn, ok := seq.Message.(*messaging.TransactionMessage)
			if !ok {
				return true, nil
			}
			if _, ok := txn.Transaction.Body.(*DirectoryAnchor); !ok {
				return true, nil
			}
			if seq.Destination != nil && seq.Destination.Equal(PartitionUrl("BVN1")) {
				return false, nil // drop
			}
			return true, nil
		}),
	)

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	bob := acctesting.GenerateKey("Bob")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
	sim.SetRoute(aliceUrl, "BVN0")
	sim.SetRoute(bobUrl, "BVN1")
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

	// Trap DN→BVN1 anchors, then send a cross-partition deposit BVN0→BVN1.
	trap = true
	st := sim.SubmitTxnSuccessfully(MustBuild(t,
		build.Transaction().For(aliceUrl).
			SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
			SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	// The deposit's synthetic reaches BVN1 but its anchor is trapped, so it is
	// held — the recipient account is not even created. (Without the fix the
	// synthetic would be terminally failed here and never recover.)
	sim.StepN(25)
	heldErr := sim.DatabaseFor(bobUrl).View(func(batch *database.Batch) error {
		_, err := batch.Account(bobUrl).Main().Get()
		return err
	})
	require.Error(t, heldErr, "deposit must be held while its anchor is missing (recipient not yet created)")

	// Release: stop trapping so anchor healing re-pushes the DN anchor. The held
	// synthetic re-attempts in place (no re-submission) and delivers.
	trap = false
	sim.StepUntil(
		Txn(st.TxID).Produced().Succeeds())

	lta := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(bobUrl), bobUrl)
	require.Equal(t, int(protocol.AcmePrecision), int(lta.Balance.Uint64()))
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
