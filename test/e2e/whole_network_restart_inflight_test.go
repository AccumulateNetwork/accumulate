// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAWholeNetworkRestartDeliversWhatWasInFlight — #4464. BVN1 receives
// nothing from BVN0's synthetic stream and nothing from the Directory's
// anchor stream for longer than the producer's cache holds (the horizon),
// with synthetics produced for it in that time. Then every validator of every
// partition restarts at once, from its own store, through the production join
// (restartAndJoin), and the streams open again.
//
// Nothing in memory anywhere holds what BVN1 lacks: BVN0's cache trimmed its
// entries at the horizon and its seed reaches back only to the Directory's
// newest receipt; the Directory's cache trimmed the anchors the same way; and
// every peer restarted too. The entries are on the producers' own chains. A
// destination's requester asks the source by number, and the source must
// answer from those chains, with a proof BVN1 accepts: every synthetic
// produced for BVN1 is delivered afterwards, and every anchor.
//
// Before #4464 the source answered "synthetic N is not in the cache" and
// "anchor N is not in the cache" for ever, as the Docker network did after
// the whole-network restart at 2026-09-29T00:49:32Z (BVN2→BVN1 97 short,
// Directory anchor 12262 never delivered).
func TestAWholeNetworkRestartDeliversWhatWasInFlight(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	// BVN1 hears nothing from BVN0's synthetics or the Directory's anchors
	// while hold is set: not the dispatch, not a healing package
	var hold atomic.Bool
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.CaptureDispatchedMessages(func(_ context.Context, env *messaging.Envelope) (bool, error) {
			if !hold.Load() {
				return true, nil
			}
			for _, m := range env.Messages {
				if heldFromBVN1(m) {
					return false, nil
				}
			}
			return true, nil
		}),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var ts uint64
	send := func() *url.TxID {
		t.Helper()
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds())
		return st.TxID
	}

	// One delivered normally, so the streams are open
	sim.StepUntil(Txn(send()).Produced().Succeeds())
	sim.StepN(10)

	hold.Store(true)

	// Synthetics in flight for BVN1, spread over the hold, which outlasts
	// the producer's cache
	var inFlight []*url.TxID
	for i := 0; i < 3; i++ {
		inFlight = append(inFlight, send())
		sim.StepN(synthcache.DefaultHorizon/3 + 20)
	}
	produced := syntheticLedger(t, sim.S.Partition("BVN0").NodeDatabase(0), "BVN0").Partition(PartitionUrl("BVN1")).Produced
	delivered := syntheticLedger(t, sim.S.Partition("BVN1").NodeDatabase(0), "BVN1").Partition(PartitionUrl("BVN0")).Delivered
	require.Less(t, delivered, produced, "precondition: BVN1 is missing BVN0's synthetics")
	anchorsHeld := anchorLedger(t, sim.S.Partition("BVN1").NodeDatabase(0), "BVN1").Partition(DnUrl()).Delivered
	t.Logf("before the restart: BVN0 produced %d for BVN1, BVN1 delivered %d; BVN1 delivered Directory anchor %d", produced, delivered, anchorsHeld)

	// The whole network, at once
	var all []restartTarget
	for _, name := range []string{Directory, "BVN0", "BVN1"} {
		p := sim.S.Partition(name)
		for i := 0; i < p.NodeCount(); i++ {
			all = append(all, restartTarget{name, p, i})
		}
	}
	restartAndJoin(t, sim, all)

	// The streams open again, and everything in flight is delivered
	hold.Store(false)
	var conds []Condition
	for _, id := range inFlight {
		conds = append(conds, Txn(id).Produced().Succeeds())
	}
	sim.StepUntilN(2*synthcache.DefaultHorizon, conds...)

	for i := 0; i < 3; i++ {
		got := syntheticLedger(t, sim.S.Partition("BVN1").NodeDatabase(i), "BVN1").Partition(PartitionUrl("BVN0")).Delivered
		require.GreaterOrEqual(t, got, produced, "BVN1 node %d did not deliver every synthetic BVN0 produced for it", i)
		a := anchorLedger(t, sim.S.Partition("BVN1").NodeDatabase(i), "BVN1").Partition(DnUrl()).Delivered
		require.Greater(t, a, anchorsHeld, "BVN1 node %d delivered no Directory anchor after the restart", i)
	}
	var account *TokenAccount
	require.NoError(t, sim.S.Partition("BVN1").View(func(batch *database.Batch) error {
		return batch.Account(bob.JoinPath("tokens")).Main().GetAs(&account)
	}))
	require.Equal(t, int64(1+len(inFlight)), account.Balance.Int64(), "bob did not receive every deposit")
}

// heldFromBVN1 is whether m is a synthetic from BVN0 or an anchor from the
// Directory, for BVN1.
func heldFromBVN1(m messaging.Message) bool {
	var seq *messaging.SequencedMessage
	switch m := m.(type) {
	case *messaging.SyntheticMessage:
		seq, _ = m.Message.(*messaging.SequencedMessage)
	case *messaging.BadSyntheticMessage:
		seq, _ = m.Message.(*messaging.SequencedMessage)
	case *messaging.BlockAnchor:
		seq, _ = m.Anchor.(*messaging.SequencedMessage)
	case *messaging.SequencedMessage:
		seq = m
	}
	if seq == nil || seq.Source == nil || seq.Destination == nil || !PartitionUrl("BVN1").ParentOf(seq.Destination) && !PartitionUrl("BVN1").Equal(seq.Destination) {
		return false
	}
	return PartitionUrl("BVN0").LocalTo(seq.Source) || DnUrl().LocalTo(seq.Source)
}

func syntheticLedger(t *testing.T, db *database.Database, partition string) *SyntheticLedger {
	t.Helper()
	var ledger *SyntheticLedger
	require.NoError(t, db.View(func(batch *database.Batch) error {
		return batch.Account(PartitionUrl(partition).JoinPath(Synthetic)).Main().GetAs(&ledger)
	}))
	return ledger
}

func anchorLedger(t *testing.T, db *database.Database, partition string) *AnchorLedger {
	t.Helper()
	var ledger *AnchorLedger
	require.NoError(t, db.View(func(batch *database.Batch) error {
		return batch.Account(PartitionUrl(partition).JoinPath(AnchorPool)).Main().GetAs(&ledger)
	}))
	return ledger
}
