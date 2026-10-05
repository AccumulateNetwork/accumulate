// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join_test

import (
	"context"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAnExecutedBlockThatDivergesIsCaughtWhileABackfillIsInFlight — #4472.
// The backfill of one large account (the Directory's anchor pool on Docker:
// tens of thousands of entries a chain) runs for minutes, and it ran inside
// the root watch, which compared no executed block's root with its anchor for
// as long. Here the account being backfilled is served by peers that answer
// its backfill not at all until the test ends, and the joined node's state is
// corrupted while that backfill waits: the root watch must still catch the
// divergence and sync the node again.
//
// Through the production wiring: join.Run, the state RestartNode builds as
// the daemon builds it, over QueryPeers; only dave's backfill pages are held.
func TestAnExecutedBlockThatDivergesIsCaughtWhileABackfillIsInFlight(t *testing.T) {
	const joiner = 2

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	dave := url.MustParse("dave")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(1024),
	)
	for _, u := range []*url.URL{alice, bob, dave} {
		sim.SetRoute(u, "BVN0")
	}
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	MakeIdentity(t, sim.DatabaseFor(dave), dave, acctesting.GenerateKey(dave)[32:])
	MakeAccount(t, sim.DatabaseFor(dave), &TokenAccount{Url: dave.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var ts uint64
	send := func(to *url.URL) {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(to, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}
	for i := 0; i < 5; i++ {
		send(bob)
		sim.StepN(3)
	}

	// The node stops. While it is down dave is sent to past his first mark
	// set, and never after: the join takes him by his chain heads, and the
	// backfill is what brings the rest in.
	p := sim.S.Partition("BVN0")
	p.RestartNode(joiner)
	for i := 0; i < 10; i++ {
		for j := 0; j < 30; j++ {
			send(dave)
		}
		sim.Step()
	}
	for i := 0; i < 5; i++ {
		send(bob)
		sim.StepN(3)
	}

	// dave's backfill is answered by no peer until the test ends: an account
	// whose backfill takes as long as the test does.
	p.RestartNode(joiner)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var held heldBackfill
	held.account = dave.JoinPath("tokens")
	p.SetSourceWrap(joiner, func(srcs []pull.Source) []pull.Source {
		out := make([]pull.Source, len(srcs))
		for i, s := range srcs {
			out[i] = &heldSource{Source: s, held: &held}
		}
		return out
	})

	buf := &countingBuffer{Buffer: p.NodeJoin(joiner)}
	pulled := p.NodeJoinState(joiner)
	state := &steppingPulledState{PulledState: pulled}
	// A pull round that runs while the backfill still writes: the pull
	// rewrites the accounts the backfill writes, so none may.
	var pullsBesideBackfill int
	state.step = func() {
		if pulled.Machine().State() != nodestate.StateActive && held.waiting.Load() > 0 {
			pullsBesideBackfill++
		}
		if state.steps%4 == 0 {
			send(bob)
		}
		state.steps++
		sim.Step()
	}
	var corruptedAt, checks, caughtWhileHeld int
	state.watch = func() {
		checks++
		switch {
		case len(buf.handoffs) == 1 && corruptedAt == 0 && held.waiting.Load() > 0:
			// The node is executing and dave's backfill is in flight.
			corrupt(t, p.NodeDatabase(joiner), bob.JoinPath("tokens"))
			corruptedAt = checks
		case buf.starts > 1 && corruptedAt != 0 && caughtWhileHeld == 0:
			// Caught: the node collects again. Where was the backfill?
			caughtWhileHeld = checks
			cancel()
		case corruptedAt == 0 && checks > 4000, corruptedAt != 0 && checks > corruptedAt+400:
			// The budget runs from the corruption, not from the start: the
			// corruption waits for dave's backfill to be in flight, which
			// runs on its own goroutine and arrives when it arrives. A
			// budget counted from the start cancelled runs whose backfill
			// came late, before the catch could happen (#4513).
			cancel()
		}
	}
	var heldAtCatch int64 = -1
	diverged := &divergedObserver{steppingPulledState: state, onDiverged: func() {
		heldAtCatch = held.waiting.Load()
	}}

	settler, ok := p.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok, "the executor must settle staging")
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    buf,
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(joiner), Database: p.NodeDatabase(joiner)},
		State:     diverged,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)

	require.NotEmpty(t, buf.handoffs, "the join hands off")
	require.NotZero(t, held.entered.Load(), "precondition: dave's backfill was never asked for")
	require.NotZero(t, corruptedAt, "the root watch never ran again once dave's backfill was in flight: it waits on the backfill")
	require.Equal(t, 2, buf.starts, "the divergence was not caught while dave's backfill was in flight")
	require.Equal(t, int64(1), heldAtCatch, "precondition: dave's backfill was in flight when the divergence was caught")
	require.Len(t, buf.handoffs, 2, "the node that diverged did not sync again and hand off")
	require.Zero(t, pullsBesideBackfill, "the node pulled while its backfill was still in flight")
	require.Zero(t, held.waiting.Load(), "the backfill outlived the join")
	require.Equal(t, nodestate.StateActive, pulled.Machine().State())
}

// heldBackfill counts the backfill pages of one account asked for and held.
type heldBackfill struct {
	account *url.URL
	entered atomic.Int64
	waiting atomic.Int64
}

// heldSource answers a backfill of the held account -- the chain from its
// first entry, a page or more at a time, which only the backfill asks for --
// not at all until the caller gives up.
type heldSource struct {
	pull.Source
	held *heldBackfill
}

func (s *heldSource) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	// The backfill walks from the head down (#4493): its first page of
	// dave is held, wherever it starts and however many entries it asks for.
	if u.Equal(s.held.account) && join.Backfilling(ctx) && q.Range != nil {
		s.held.entered.Add(1)
		s.held.waiting.Add(1)
		defer s.held.waiting.Add(-1)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return s.Source.QueryChainEntries(ctx, u, q)
}

// divergedObserver calls onDiverged when a root check reports a divergence.
type divergedObserver struct {
	*steppingPulledState
	onDiverged func()
}

func (d *divergedObserver) Diverged(ctx context.Context) (uint64, bool, error) {
	n, diverged, err := d.steppingPulledState.Diverged(ctx)
	if diverged {
		d.onDiverged()
	}
	return n, diverged, err
}
