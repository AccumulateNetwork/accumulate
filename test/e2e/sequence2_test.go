// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
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

func TestMissingSynthTxn(t *testing.T) {
	// This test was flaky because the simulator lost messages that healing
	// submitted from background tasks — fixed by the shared hub dispatcher
	// (#4048).
	Run(t, map[string]ExecutorVersion{
		"v1":     ExecutorVersionV1SignatureAnchoring,
		"latest": ExecutorVersionLatest,
	}, func(t *testing.T, version ExecutorVersion) {
		var timestamp uint64

		// The first time an envelope contains a deposit, drop the first deposit
		var didDrop bool

		// Initialize
		globals := new(core.GlobalValues)
		globals.ExecutorVersion = version
		sim := NewSim(t,
			simulator.SimpleNetwork(t.Name(), 3, 3),
			simulator.GenesisWith(GenesisTime, globals),
			simulator.SkipProposalCheck(), // FIXME should not be necessary

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
							fmt.Printf("Dropping %X\n", m.GetTransaction().GetHash()[:4])
							didDrop = true
							return false, nil
						}
					}
				}
				return true, nil
			}),
		)

		alice := acctesting.GenerateKey("Alice")
		aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
		bob := acctesting.GenerateKey("Bob")
		bobUrl := acctesting.AcmeLiteAddressStdPriv(bob)
		// The deposit this test drops only exists if the sender and the receiver
		// are on different partitions, so say so instead of leaving it to where
		// two hashes happen to land. Even bucket routing (#4136) moved them onto
		// the same BVN, and the drop hook then had nothing to drop.
		sim.SetRoute(aliceUrl, "BVN0")
		sim.SetRoute(bobUrl, "BVN1")
		MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())

		// Execute. Step between submissions so each block produces ONE
		// deposit: packaged dispatch (#4141) bundles a block's deposits for a
		// destination into a single envelope, and this test is about ONE
		// missing message whose gap is exposed by later arrivals — dropping a
		// whole package would instead leave a lost tail with nothing behind
		// it (TestRangeRecovery's territory).
		st := make([]*protocol.TransactionStatus, 5)
		for i := range st {
			st[i] = sim.SubmitTxnSuccessfully(MustBuild(t,
				build.Transaction().For(aliceUrl).
					SendTokens(1, protocol.AcmePrecisionPower).To(bobUrl).
					SignWith(aliceUrl).Version(1).Timestamp(&timestamp).PrivateKey(alice)))
			sim.StepN(2)
		}
		sim.StepUntil(True(func(*Harness) bool { return didDrop }))

		for _, st := range st {
			sim.StepUntil(
				Txn(st.TxID).Succeeds(),
				Txn(st.TxID).Produced().Succeeds())
		}

		// Verify
		lta := GetAccount[*LiteTokenAccount](t, sim.DatabaseFor(bobUrl), bobUrl)
		require.Equal(t, len(st)*protocol.AcmePrecision, int(lta.Balance.Uint64()))
	})
}

func TestMissingDirectoryAnchorTxn(t *testing.T) {
	// Drop the next directory anchor
	// The hook runs on every node's goroutine; the counter it reads and
	// increments is shared (#4171).
	var anchorsMu sync.Mutex
	var anchors int

	// Initialize
	const bvnCount, valCount = 1, 1 // Anchor healing doesn't work with more than one validator
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), bvnCount, valCount),
		simulator.Genesis(GenesisTime),

		simulator.CaptureDispatchedMessages(func(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
			anchorsMu.Lock()
			defer anchorsMu.Unlock()
			if anchors >= valCount*bvnCount {
				return true, nil
			}

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
				txn := anchor.Anchor.(*messaging.SequencedMessage).Message.(*messaging.TransactionMessage)
				if txn.Transaction.Body.Type() == TransactionTypeDirectoryAnchor {
					anchors++
					drop = true
				}
			}
			return !drop, nil
		}),
	)

	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	faucetKey := acctesting.GenerateKey("Faucet")
	faucet := acctesting.AcmeLiteAddressStdPriv(faucetKey)
	MakeLiteTokenAccount(t, sim.DatabaseFor(faucet), faucetKey[32:], AcmeUrl())

	sim.StepUntil(True(func(*Harness) bool { return anchors >= valCount*bvnCount }))

	// Cause a synthetic transaction
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(faucet).
			SendTokens(1, AcmeOraclePrecisionPower).To(lite).
			SignWith(faucet).Timestamp(1).Version(1).PrivateKey(faucetKey))

	// The lost anchor holes the Directory's anchor stream from that BVN, and
	// a stream executes in order with no gaps, so nothing that BVN produces
	// is anchored -- and no synthetic of its is provable -- until healing
	// fills the hole. Filling it means rebuilding the anchor's signature
	// QUORUM from answers: one node's answer carries one signature, so the
	// requester has to ask the source's validators one by one until it holds
	// enough distinct signers. Before it did, a completely lost anchor stayed
	// lost about one run in thirty -- the runs where the transport happened
	// to keep dialing the same source node. The budget is generous so that a
	// failure here means recovery stopped working, not that it was slow.
	sim.StepUntilN(recoverAnchorHoleBlocks,
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
}

// recoverAnchorHoleBlocks is how long this test waits for healing to fill a
// hole in an anchor stream. Well past what recovery needs: the point is to
// distinguish "recovery is broken" from "recovery is slow", and when the
// stillness gate was applied to anchor streams it was broken -- 600 blocks
// did not help (#4280).
const recoverAnchorHoleBlocks = 120

func TestMissingBlockValidatorAnchorTxn(t *testing.T) {
	// Initialize
	const bvnCount, valCount = 3, 3
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), bvnCount, valCount),
		simulator.Genesis(GenesisTime),
	)

	liteKey := acctesting.GenerateKey("Lite")
	lite := acctesting.AcmeLiteAddressStdPriv(liteKey)
	alice := AccountUrl("alice")
	aliceKey := acctesting.GenerateKey(alice)
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	UpdateAccount(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), func(p *KeyPage) { p.CreditBalance = 1e9 })

	faucetKey := acctesting.GenerateKey("Faucet")
	faucet := acctesting.AcmeLiteAddressStdPriv(faucetKey)
	MakeLiteTokenAccount(t, sim.DatabaseFor(faucet), faucetKey[32:], AcmeUrl())

	// Lose ONE block validator anchor completely: every copy of a single
	// (source, number), for a fixed number of blocks, and nothing else.
	//
	// The hook is called per node, and it edits shared envelopes (#4171), so
	// a hook that drops whatever it sees until a shared counter trips drops a
	// schedule-dependent amount: measured, this one removed eighty-one
	// messages -- every copy of anchor 1 from all three BVNs -- where the
	// counter said three. Two different faults, chosen by timing, and the
	// test failed about half the time because only one of them was
	// recoverable. Pinning the target to the first (source, number) seen
	// makes the fault the one the test names.
	lostAnchorSource := PartitionUrl("BVN0")
	var dropMu sync.Mutex
	var target *url.URL
	var targetNum uint64
	var dropUntil uint64
	var dropped int
	sim.SetBlockHook(Directory, func(params execute.BlockParams, envelopes []*messaging.Envelope) (_ []*messaging.Envelope, keepHook bool) {
		dropMu.Lock()
		defer dropMu.Unlock()
		for _, env := range envelopes {
			for i := len(env.Messages) - 1; i >= 0; i-- {
				anchor, ok := env.Messages[i].(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				seq, ok := anchor.Anchor.(*messaging.SequencedMessage)
				if !ok {
					continue
				}
				txn, ok := seq.Message.(*messaging.TransactionMessage)
				if !ok || txn.Transaction.Body.Type() != TransactionTypeBlockValidatorAnchor {
					continue
				}
				// A FIXED source, not the first one seen: which anchor
				// arrives first varies by scheduling, and with it whether
				// the lost anchor sits on the path the test's transaction
				// needs. That choice alone was worth a coin flip.
				if !seq.Source.Equal(lostAnchorSource) {
					continue
				}
				if target == nil {
					// Wide enough that EVERY dispatched copy is lost -- at
					// two blocks a straggler sometimes got through and the
					// test passed without healing doing anything. Healing's
					// own answers land after the window, on re-ask.
					target, targetNum, dropUntil = seq.Source, seq.Number, params.Index+8
				}
				if seq.Number != targetNum {
					continue
				}
				dropped++
				env.Messages = append(env.Messages[:i], env.Messages[i+1:]...)
			}
		}
		return envelopes, target == nil || params.Index <= dropUntil
	})

	sim.StepUntil(True(func(*Harness) bool {
		dropMu.Lock()
		defer dropMu.Unlock()
		return dropped > 0 && target != nil
	}))

	// Cause a synthetic transaction
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(faucet).
			SendTokens(1, AcmeOraclePrecisionPower).To(lite).
			SignWith(faucet).Timestamp(1).Version(1).PrivateKey(faucetKey))
	sim.StepUntil(
		Txn(st.TxID).Succeeds(),
		Txn(st.TxID).Produced().Succeeds())
}
