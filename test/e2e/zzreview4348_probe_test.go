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
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// capture counts the join's messages so the probe can tell "the node was past
// the peer" from "the peer could not serve anything at all".
type capture struct {
	mu   sync.Mutex
	msgs []string
}

func (c *capture) Enabled(context.Context, slog.Level) bool { return true }
func (c *capture) WithAttrs([]slog.Attr) slog.Handler       { return c }
func (c *capture) WithGroup(string) slog.Handler            { return c }
func (c *capture) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var sb strings.Builder
	sb.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool { sb.WriteString(" | " + a.Key + "=" + a.Value.String()); return true })
	c.msgs = append(c.msgs, sb.String())
	return nil
}
func (c *capture) count(sub string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, m := range c.msgs {
		if strings.Contains(m, sub) {
			n++
		}
	}
	return n
}

// TestReview4348_TheJoinRoundActuallyReachesThePastPath re-runs the headline
// test's setup and asks what the join actually did: how many accounts took the
// past path, and how many were simply refused (which would make the "the node
// kept its own state" assertion pass for the wrong reason).
func TestReview4348_TheJoinRoundActuallyReachesThePastPath(t *testing.T) {
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
	behind, r, _ := copyOf(t, sim, anchors, part)
	send()
	send()
	send()
	local, q, _ := copyOf(t, sim, anchors, part)
	require.Greater(t, q, r)
	t.Logf("node at block %d, peer at block %d", q, r)

	// (1) Can the behind peer, built by copyOf, serve a receipt at all?
	peerSrc := peerServingPart(behind, "BVN0")
	for _, u := range []*url.URL{part.JoinPath(Ledger), part.JoinPath(AnchorPool), alice.JoinPath("tokens")} {
		b := local.Begin(true)
		p, err := pull.Fetch(ctx, peerSrc, b, u, pull.Options{Mode: pull.ModeStateOnly, Partition: part}, true)
		if err != nil {
			t.Logf("DIRECT FETCH %v: error %v", u, err)
		} else {
			t.Logf("DIRECT FETCH %v: past=%v block=%d", u, p.Past(), p.Block)
			p.Discard()
		}
		b.Discard()
	}

	// (2) What the join round does, counted.
	cap := new(capture)
	state, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  local,
		Logger:    slog.New(cap),
		Sources: &behindSources{
			part:  part,
			peer:  peerSrc,
			peerQ: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: behind, Partition: "BVN0"}),
			dn:    sim.S.Services(),
		},
	})
	require.NoError(t, err)
	for round := 0; round < 4; round++ {
		require.NoError(t, state.Pull(ctx), "pull round %d", round)
	}
	require.Zero(t, pull.Held(), "accounts are still held after the rounds: MaxHeld leaked")
	past := cap.count("The node is past the peer")
	refused := cap.count("An account could not be pulled")
	t.Logf("PAST=%d REFUSED=%d", past, refused)
	for _, m := range cap.msgs {
		t.Logf("LOG: %s", m)
	}
	require.NotZero(t, past, "the join never took the past path: the headline test passes for another reason")
}
