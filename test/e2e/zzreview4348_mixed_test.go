// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"log/slog"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestReview4348_AMixedSourceIsRetriedForever — the mixed shape (beyond on one
// chain, behind on another) is refused. What does a refusal cost the join when
// the source presents it every round?
func TestReview4348_AMixedSourceIsRetriedForever(t *testing.T) {
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
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	ctx := context.Background()
	part := PartitionUrl("BVN0")
	anchors := &pull.DirectoryAnchors{Query: sim.S.Services()}

	var ts uint64
	send := func() {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}

	send()
	send()
	behind, _, _ := copyOf(t, sim, anchors, part)
	send()
	send()
	local, _, _ := copyOf(t, sim, anchors, part)

	// Make the peer MIXED on alice/tokens: behind on main (it already is),
	// one entry ahead on the signature chain. A dishonest source can serve
	// exactly this; the node cannot tell it from a peer that is simply odd.
	tokens := alice.JoinPath("tokens")
	require.NoError(t, behind.Update(func(b *database.Batch) error {
		c, err := b.Account(tokens).ChainByName("signature")
		if err != nil {
			return err
		}
		e := make([]byte, 32)
		e[0] = 0xAB
		if err := c.Inner().AddEntry(e, false); err != nil {
			return err
		}
		return b.UpdateBPT()
	}))

	cap := new(capture)
	state, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  local,
		Logger:    slog.New(cap),
		Sources: &behindSources{
			part:  part,
			peer:  peerServingPart(behind, "BVN0"),
			peerQ: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: behind, Partition: "BVN0"}),
			dn:    sim.S.Services(),
		},
	})
	require.NoError(t, err)
	for round := 0; round < 5; round++ {
		require.NoError(t, state.Pull(ctx), "round %d", round)
	}
	require.Zero(t, pull.Held(), "held accounts leaked")
	refused := cap.count("An account could not be pulled")
	t.Logf("refusals over five rounds: %d", refused)
	for _, m := range cap.msgs {
		if len(m) > 240 {
			m = m[:240] + "..."
		}
		t.Logf("LOG: %s", m)
	}
	require.NotZero(t, refused, "the mixed source was not refused")
}
