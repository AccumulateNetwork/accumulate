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

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// joiningNodeQuerier is the joining node's OWN query service: the store the
// pull exists to fill. It holds nothing, and it says so, which is what a real
// one does -- 18,313 times in run 20260918T131713Z (#4303).
type joiningNodeQuerier struct{ hits atomic.Int64 }

func (j *joiningNodeQuerier) Query(_ context.Context, scope *url.URL, _ api.Query) (api.Record, error) {
	j.hits.Add(1)
	return nil, errors.NotFound.WithFormat(
		"%v: this node is joining, and its store is the one the pull exists to fill", scope)
}

// selfAnsweringDialer is what a real node's dialer does with a query that names
// no peer: it answers it locally. p2p.DialNetwork installs a self-discoverer
// unconditionally ("Always use self-discovery", dial_network.go) and every
// partition a node serves has its query:<id> registered, so an unaddressed read
// from a joining node's own client never leaves the process.
//
// Here that is made visible instead of invisible: a query dial carrying no /p2p
// component is counted and served by the joining node's own empty store; one
// that names a peer is counted separately and passed to the simulator, which
// serves it from that node.
type selfAnsweringDialer struct {
	inner   message.Dialer
	handler *message.Handler

	self atomic.Int64 // query reads that named no peer
	peer atomic.Int64 // query reads addressed at a named peer
}

func (d *selfAnsweringDialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	_, id, sa, _, err := api.UnpackAddress(addr)
	if err == nil && sa != nil && sa.Type == api.ServiceTypeQuery {
		if id == "" {
			d.self.Add(1)
			p, q := message.DuplexPipe(ctx)
			go func() {
				defer p.Close()
				d.handler.Handle(p)
			}()
			return q, nil
		}
		d.peer.Add(1)
	}
	return d.inner.Dial(ctx, addr)
}

// TestJoinReadsOnlyFromNamedPeers is the guard on the #4303 fix that had none
// (#4317): EVERY READ THE PULL MAKES IS ADDRESSED AT A NAMED PEER, NEVER AT
// THIS NODE.
//
// The auditor's mutation run established that reverting that fix -- making
// join.QueryPeers.clientFor ignore the peer ID and return the routed client --
// leaves the whole suite green, TestJoinPullsFromPeersAndPromotes included.
// That test cannot catch it and says so at its own line 89: "This node is
// joining, so it is not one of the simulator's peers", so there is nothing for
// the routed client to answer from and the read reaches a real node anyway.
// The defect cost 18,313 refusals and every join on the live network.
//
// This test removes that concession. The joining node's own query service
// exists here, it is what an unaddressed query dial reaches, and it holds
// nothing -- exactly the production arrangement. So the property is checkable:
// the join must converge, and it must do so having made zero unaddressed
// reads.
func TestJoinReadsOnlyFromNamedPeers(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
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

	var ts uint64
	for i := 0; i < 3; i++ {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	sim.StepN(20)

	ctx := context.Background()
	part := PartitionUrl("BVN0")

	// This node's own query service, standing where the self-discoverer puts
	// it: at the end of every query dial that names no peer.
	itself := new(joiningNodeQuerier)
	handler, err := message.NewHandler(message.Querier{Querier: itself})
	require.NoError(t, err)

	// The production client, with one thing added: a dial that names no peer
	// reaches this node instead of vanishing into the simulator's map.
	base, ok := sim.S.Services().Transport.(*message.RoutedTransport)
	require.True(t, ok, "the simulator's client is not a routed transport")
	dialer := &selfAnsweringDialer{inner: base.Dialer, handler: handler}
	client := &message.Client{Transport: &message.RoutedTransport{
		Network:  base.Network,
		Attempts: base.Attempts,
		Router:   base.Router,
		Dialer:   dialer,
	}}

	sources := &join.QueryPeers{
		Client:  client,
		Network: t.Name(),
		Router:  sim.S.Router(),
	}

	// What a joining node really holds before it asks anybody anything: its
	// own genesis network accounts. They are the keys it verifies anchors
	// against, and they are the one thing it must not take from a peer
	// (#4301). An empty store is not a case: every node loads genesis before
	// it joins, and a node with no key to start from cannot verify a root.
	local := genesisOf(t, sim, "BVN0")
	state, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  local,
		Sources:   sources,
	})
	require.NoError(t, err)

	var q uint64
	var promoted bool
	for round := 0; round < 40 && !promoted; round++ {
		require.NoError(t, state.Pull(ctx), "pull round %d", round)
		sim.StepN(3)
		q, promoted, err = state.Matched(ctx)
		require.NoError(t, err)
	}

	// The assertion this test exists for.
	require.Zero(t, dialer.self.Load(),
		"the pull made %d read(s) that named no peer; a joining node's own client answers those from the store the pull exists to fill (#4303)",
		dialer.self.Load())
	require.Zero(t, itself.hits.Load(),
		"this node answered %d of its own reads", itself.hits.Load())

	// The join ran, and it ran over the network: a join that read nothing
	// would satisfy "no self-addressed read" for the wrong reason.
	require.NotZero(t, dialer.peer.Load(), "the pull made no peer-addressed read at all")

	// And it worked: reads addressed at named peers are enough to join.
	require.True(t, promoted, "the join never reached a root the Directory anchored")
	require.NotZero(t, q)
	require.Equal(t, nodestate.StateActive, state.Machine().State())
}
