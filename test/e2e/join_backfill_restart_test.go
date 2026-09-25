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
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
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

// TestARestartBeforeTheBackfillEndsStillBackfills — #4438 re-review F-8(b).
// The join takes an account whose chain is past its first mark set by its
// chain heads alone, and brings in the entries below the open set only once
// the state has matched, while the node executes (backfill). The node is
// restarted at the match, before any backfill: the next join finds the
// account's leaf equal to the peers' and takes nothing of it, so unless the
// accounts left to backfill survive the restart the node never holds the
// account's chains whole.
func TestARestartBeforeTheBackfillEndsStillBackfills(t *testing.T) {
	const joiner = 1

	alice := url.MustParse("alice")
	carol := url.MustParse("carol")
	dave := url.MustParse("dave")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(carol, "BVN0")
	sim.SetRoute(dave, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(carol), carol, acctesting.GenerateKey(carol)[32:])
	MakeAccount(t, sim.DatabaseFor(carol), &TokenAccount{Url: carol.JoinPath("tokens"), TokenUrl: AcmeUrl()})
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
	traffic := func() { send(carol); sim.Step() }
	for i := 0; i < 5; i++ {
		traffic()
	}

	// The node stops. While it is down dave is sent to past his first mark
	// set, and never after, so no record after the pull starts names him.
	p := sim.S.Partition("BVN0")
	p.RestartNode(joiner)
	for i := 0; i < 10; i++ {
		for j := 0; j < 30; j++ {
			send(dave)
		}
		sim.Step()
	}
	for i := 0; i < 10; i++ {
		traffic()
	}
	require.Greater(t, chainHeight(t, p.NodeDatabase(0), dave.JoinPath("tokens"), "main"), int64(256),
		"precondition: dave's chain is not past its first mark set")

	// The join, to the match, and the process stopped there: the context
	// ends as the node is promoted, before the backfill of that check runs.
	p.RestartNode(joiner)
	runJoin(t, sim, p, joiner, traffic, true)
	require.False(t, p.Joining(joiner), "precondition: the node did not hand off")
	View(t, p.NodeDatabase(joiner), func(batch *database.Batch) {
		c, err := batch.Account(dave.JoinPath("tokens")).ChainByName("main")
		require.NoError(t, err)
		_, err = c.Inner().Entry(0)
		require.Error(t, err, "precondition: dave's chain was backfilled before the restart")
	})

	// The process starts again and joins, and executes on.
	p.RestartNode(joiner)
	runJoin(t, sim, p, joiner, traffic, false)
	requireSameChains(t, p.NodeDatabase(joiner), p.NodeDatabase(0), dave.JoinPath("tokens"))
}

// runJoin runs join.Run on node joiner of p, as the daemon runs it, stepping
// the network with traffic every round. With atPromotion the join is stopped
// the moment the node is promoted, before the backfill of that check;
// otherwise it runs on for a number of root checks after the promotion.
func runJoin(t *testing.T, sim *Sim, p *simulator.Partition, joiner int, traffic func(), atPromotion bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := p.NodeJoinState(joiner)
	const maxRounds = 150
	active := 0
	stepping := &steppingState{cancel: cancel, step: func(round int) {
		traffic()
		if state.Machine().State() == nodestate.StateActive {
			active++
		}
		if round >= maxRounds || active > 20 {
			cancel()
		}
	}}
	stepping.State = state
	if atPromotion {
		state.Machine().OnChange(func(ad nodestate.Advertisement) {
			if ad.State == nodestate.StateActive {
				cancel()
			}
		})
	}
	settler, ok := p.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok, "the executor must settle staging")
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    p.NodeJoin(joiner),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(joiner), Database: p.NodeDatabase(joiner)},
		State:     &watchingState{stepping},
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, nodestate.StateActive, state.Machine().State(), "the node was not promoted within %d rounds", maxRounds)
}

// watchingState is a steppingState whose root watch runs on past the
// promotion until the test's step ends it.
type watchingState struct{ *steppingState }

func (w *watchingState) Diverged(ctx context.Context) (uint64, bool, error) {
	w.round++
	w.step(w.round)
	return w.State.(join.RootWatch).Diverged(ctx)
}
