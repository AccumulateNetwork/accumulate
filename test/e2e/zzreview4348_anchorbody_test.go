// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestReview4348_F5RewindsTheAnchorLedger — F5 ("level on every chain, take
// the peer's body") applied to <partition>/anchors, whose body carries the
// sequence number this partition stamps on the anchors it sends.
//
// The anchor ledger's body moves in blocks where not one of its ten chains
// moves, so a peer one such block behind is EXACTLY level on every chain and
// behind on the body: the pull is not past, and the peer's anchor ledger is
// written over the node's.
func TestReview4348_F5RewindsTheAnchorLedger(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	part := PartitionUrl("BVN0")
	anchorPool := part.JoinPath(AnchorPool)
	db := sim.S.Database("BVN0")

	copyAnchors := func() *database.Database {
		out := emptyDb()
		require.NoError(t, pullStateOnlyFrom(t, peerServingPart(db, "BVN0"), out, part, []*url.URL{anchorPool}))
		return out
	}
	seqOf := func(d database.Beginner) (uint64, uint64) {
		b := d.Begin(false)
		defer b.Discard()
		var l *AnchorLedger
		require.NoError(t, b.Account(anchorPool).Main().GetAs(&l))
		return l.MinorBlockSequenceNumber, l.LastAnchorBlock
	}
	chainsOf := func(d database.Beginner) map[string]int64 {
		b := d.Begin(false)
		defer b.Discard()
		out := map[string]int64{}
		cs, err := b.Account(anchorPool).Chains().Get()
		require.NoError(t, err)
		for _, cm := range cs {
			c, err := b.Account(anchorPool).ChainByName(cm.Name)
			require.NoError(t, err)
			h, err := c.Head().Get()
			require.NoError(t, err)
			out[cm.Name] = h.Count
		}
		return out
	}

	var ts uint64
	prev := copyAnchors()
	for i := 0; i < 60; i++ {
		if i%9 == 2 {
			ts++
			sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					SendTokens(1, 0).To(bob, "tokens").
					SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		}
		sim.Step()
		now := copyAnchors()

		pSeq, pLast := seqOf(prev)
		nSeq, nLast := seqOf(now)
		if pSeq == nSeq && pLast == nLast {
			prev = now
			continue
		}
		if !sameChains(chainsOf(prev), chainsOf(now)) {
			prev = now
			continue
		}

		// A peer one block behind, level on every chain, behind on the body.
		t.Logf("step %d: the anchor ledger moved seq %d->%d lastAnchorBlock %d->%d with every chain level",
			i, pSeq, nSeq, pLast, nLast)
		b := now.Begin(true)
		p, err := pull.Fetch(context.Background(), peerServingPart(prev, "BVN0"), b, anchorPool,
			pull.Options{Mode: pull.ModeFullSpine, Partition: part}, false)
		require.NoError(t, err)
		t.Logf("   the pull says past=%v", p.Past())
		require.NoError(t, p.Keep()) // pullSpine keeps without verifying
		require.NoError(t, b.UpdateBPT())
		require.NoError(t, b.Commit())

		gSeq, gLast := seqOf(now)
		t.Logf("   after the pull the node's anchor ledger is seq %d lastAnchorBlock %d", gSeq, gLast)
		require.Equal(t, nSeq, gSeq,
			"**** the node's anchor sequence number was rewound to the peer's: it will re-stamp anchors the network has already seen")
		require.Equal(t, nLast, gLast, "the node's lastAnchorBlock was rewound to the peer's")
		return
	}
	t.Skip("no block in this run moved the anchor ledger's body alone")
}
