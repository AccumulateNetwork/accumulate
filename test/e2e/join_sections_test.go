// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// The join takes the state tree by its top and loads its sections as they
// are needed (executor spec, "Sync", §3a; #4485). These tests drive a fresh
// BVN0 node through the production join.Run, against a partition whose
// validators hold accounts the node does not, and then let it execute.

// sectionsJoin is a fresh node that has joined, ACTIVE and executing, and
// the network around it.
type sectionsJoin struct {
	t     *testing.T
	sim   *Sim
	part  *simulator.Partition
	fresh int

	// many are token accounts only the validators held when the node
	// joined, under alice; dave is an identity only they held.
	many    []*url.URL
	dave    *url.URL
	daveKey []byte

	// notLoadedAtHandoff is how many sections the node had not loaded when
	// it handed off.
	notLoadedAtHandoff int

	alice, bob *url.URL
	aliceKey   []byte
	ts         uint64
}

const sectionsAccounts = 600

// sectionsOraclePrice is the partition's oracle when the joiner joins: not
// genesis's.
const sectionsOraclePrice = 31415

func joinWithSectionsNotLoaded(t *testing.T) *sectionsJoin {
	j := &sectionsJoin{t: t, fresh: 3}
	j.alice = url.MustParse("alice")
	j.bob = url.MustParse("bob")
	j.dave = url.MustParse("dave")
	j.aliceKey = acctesting.GenerateKey(j.alice)
	j.daveKey = acctesting.GenerateKey(j.dave)

	net, _ := networkWithAFollower(t.Name(), 1, 3)
	j.sim = NewSim(t,
		simulator.WithNetwork(net),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(1024),
	)
	sim := j.sim
	for _, u := range []*url.URL{j.alice, j.bob, j.dave} {
		sim.SetRoute(u, "BVN0")
	}
	MakeIdentity(t, sim.DatabaseFor(j.alice), j.alice, j.aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(j.alice), j.alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(j.alice), &TokenAccount{Url: j.alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(j.alice), j.alice.JoinPath("tokens"), big.NewInt(1e9))
	MakeIdentity(t, sim.DatabaseFor(j.bob), j.bob, acctesting.GenerateKey(j.bob)[32:])
	MakeAccount(t, sim.DatabaseFor(j.bob), &TokenAccount{Url: j.bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	j.part = sim.S.Partition("BVN0")
	require.Equal(t, 4, j.part.NodeCount())
	j.part.RestartNode(j.fresh)
	for i := 0; i < 3; i++ {
		j.send()
	}
	sim.StepN(5)

	// What only the validators hold: many token accounts, and dave.
	for i := 0; i < sectionsAccounts; i++ {
		j.many = append(j.many, j.alice.JoinPath(fmt.Sprintf("t%d", i)))
	}
	for n := 0; n < j.part.NodeCount(); n++ {
		if n == j.fresh {
			continue
		}
		db := j.part.NodeDatabase(n)
		b := db.Begin(true)
		for _, u := range j.many {
			require.NoError(t, b.Account(u).Main().Put(&TokenAccount{Url: u, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1)}))
		}
		require.NoError(t, b.UpdateBPT())
		require.NoError(t, b.Commit())
		MakeIdentity(t, db, j.dave, j.daveKey[32:])
		CreditCredits(t, db, j.dave.JoinPath("book", "1"), 1e9)
		MakeAccount(t, db, &TokenAccount{Url: j.dave.JoinPath("tokens"), TokenUrl: AcmeUrl(), Balance: *big.NewInt(1e6)})

		// The partition's oracle is not genesis's, as a live network's
		// never is: the joiner must take it, not keep a copy of its own.
		b = db.Begin(true)
		g := new(network.GlobalValues)
		get := func(u *url.URL, v interface{}) error { return b.Account(u).Main().GetAs(v) }
		require.NoError(t, g.Load(PartitionUrl("BVN0"), get))
		g.Oracle.Price = sectionsOraclePrice
		require.NoError(t, g.InitializeDataAccounts(PartitionUrl("BVN0"), get, func(a Account) error { return b.Account(a.GetUrl()).Main().Put(a) }))
		require.NoError(t, b.UpdateBPT())
		require.NoError(t, b.Commit())
	}

	// The joiner is a new install: an empty store with the genesis
	// snapshot loaded, as the daemon loads it. Its oracle is genesis's,
	// not the partition's, and it holds nothing the test wrote, so every
	// section it needs, it takes (#4485 review, finding 1). A store with
	// no genesis at all cannot join: the join verifies what it pulls by
	// the network definition it holds.
	j.part.ReinstallNode(j.fresh)

	// The join, through the production join.Run.
	j.part.RestartNode(j.fresh)
	j.run()
	return j
}

// run joins the fresh node by the production join.Run and returns once it is
// ACTIVE, with the number of sections it had not loaded then.
func (j *sectionsJoin) run() {
	t := j.t
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state := j.part.NodeJoinState(j.fresh)
	stepping := &steppingState{cancel: cancel, State: state, leaveBackfill: true, step: func(round int) {
		if j.notLoadedAtHandoff < 0 || state.Machine().State() != nodestate.StateActive {
			if round%5 == 0 {
				j.send()
			}
			j.sim.StepN(3)
		}
		if round >= 200 {
			cancel()
		}
	}}
	j.notLoadedAtHandoff = -1
	settler, ok := j.part.NodeExecutor(j.fresh).(join.Settler)
	require.True(t, ok)
	outcome, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    &handoffCounting{Buffer: j.part.NodeJoin(j.fresh)},
		Stage:     &join.ExecutorStage{Settler: settler, Staging: j.part.NodeStaging(j.fresh), Database: j.part.NodeDatabase(j.fresh)},
		State:     &countingAtHandoff{steppingState: stepping, j: j},
		Peers:     &join.APIPeers{Partition: "BVN0", Client: j.sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err, "the fresh node did not join")
	require.Equal(t, join.Joined, outcome)
	require.False(t, j.part.Joining(j.fresh))
	require.Equal(t, nodestate.StateActive, state.Machine().State())
}

// countingAtHandoff notes how many sections the node had not loaded when it
// handed off: Run calls HandedOff with the block, before the node executes.
type countingAtHandoff struct {
	*steppingState
	j *sectionsJoin
}

func (c *countingAtHandoff) HandedOff(q uint64) {
	c.steppingState.HandedOff(q)
	c.j.notLoadedAtHandoff = c.j.part.NodeJoinState(c.j.fresh).SectionsNotLoaded()
}

func (j *sectionsJoin) send() {
	j.ts++
	st := j.sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(j.alice, "tokens").
			SendTokens(1, 0).To(j.bob, "tokens").
			SignWith(j.alice, "book", "1").Version(1).Timestamp(j.ts).PrivateKey(j.aliceKey))
	j.sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
}

// notLoaded is whether the fresh node holds the account's section by its hash
// alone: its tree holds the section's marker.
func (j *sectionsJoin) notLoaded(u *url.URL) bool {
	b := j.part.NodeDatabase(j.fresh).Ungated().Begin(false)
	defer b.Discard()
	_, err := b.BPT().Get(database.SectionOfAccount(u).MarkerKey())
	if errors.Is(err, errors.NotFound) {
		return false
	}
	require.NoError(j.t, err)
	return true
}

// agree checks that the fresh node and a validator stand at the same block
// with the same root.
func (j *sectionsJoin) agree(why string) {
	t := j.t
	t.Helper()
	at := func(n int) (uint64, [32]byte) {
		b := j.part.NodeDatabase(n).Ungated().Begin(false)
		defer b.Discard()
		var ledger *SystemLedger
		require.NoError(t, b.Account(PartitionUrl("BVN0").JoinPath(Ledger)).Main().GetAs(&ledger))
		root, err := b.GetBptRootHash()
		require.NoError(t, err)
		return ledger.Index, root
	}
	fb, fr := at(j.fresh)
	vb, vr := at(0)
	require.Equal(t, vb, fb, "%s: the fresh node is not at the validator's block", why)
	require.Equal(t, vr, fr, "%s: the fresh node's root at block %d is not the validator's", why, fb)
}

// fillAll runs a joined node's fill beside the network, calling step between
// checks, until the node has loaded every section of its state tree: what it
// loads is written by the blocks step executes.
func fillAll(t *testing.T, state *join.PulledState, step func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); state.Fill(ctx) }()
	defer func() { cancel(); <-done }()
	for i := 0; i < 400 && state.SectionsNotLoaded() > 0; i++ {
		step()
		time.Sleep(time.Millisecond)
	}
	require.Zero(t, state.SectionsNotLoaded(), "the fill did not load every section")
}

// holdWhole loads every section a joined node has not loaded, then backfills
// every account it took by its chain heads, calling step between checks.
func holdWhole(t *testing.T, state *join.PulledState, step func()) {
	t.Helper()
	fillAll(t, state, step)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); state.Backfill(ctx) }()
	defer func() { cancel(); <-done }()
	for i := 0; i < 400 && state.BackfillLeft() > 0; i++ {
		step()
		time.Sleep(time.Millisecond)
	}
	require.Zero(t, state.BackfillLeft(), "the backfill did not finish")
}

// TestAJoinerExecutesBeforeEverySectionIsLoaded — #4485. The node hands off
// holding most sections by their hash alone, and executes from there in
// agreement with the validators.
func TestAJoinerExecutesBeforeEverySectionIsLoaded(t *testing.T) {
	j := joinWithSectionsNotLoaded(t)
	require.Positive(t, j.notLoadedAtHandoff, "the node handed off with every section loaded: the join pulled the whole tree")
	t.Logf("handed off with %d of %d sections not loaded", j.notLoadedAtHandoff, bpt.Sections)

	for i := 0; i < 3; i++ {
		j.send()
	}
	j.sim.StepN(5)
	j.agree("after executing with sections not loaded")
	require.Positive(t, j.part.NodeJoinState(j.fresh).SectionsNotLoaded(), "precondition: sections are still not loaded")
}

// TestAnAccountInASectionNotLoadedIsLoadedWhenABlockWritesIt — #4485. A
// deposit to an account whose section the node has not loaded: the block
// loads the section, as of the block before it, and executes as the
// validators did.
func TestAnAccountInASectionNotLoadedIsLoadedWhenABlockWritesIt(t *testing.T) {
	j := joinWithSectionsNotLoaded(t)
	var target *url.URL
	for _, u := range j.many {
		if j.notLoaded(u) {
			target = u
			break
		}
	}
	require.NotNil(t, target, "precondition: every account only the validators held is in a loaded section")

	j.ts++
	st := j.sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(j.alice, "tokens").
			SendTokens(7, 0).To(target).
			SignWith(j.alice, "book", "1").Version(1).Timestamp(j.ts).PrivateKey(j.aliceKey))
	j.sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	j.sim.StepN(3)

	require.False(t, j.notLoaded(target), "the block that wrote the account did not load its section")
	var got, want *TokenAccount
	require.NoError(t, j.part.NodeDatabase(j.fresh).Ungated().View(func(b *database.Batch) error { return b.Account(target).Main().GetAs(&got) }))
	require.NoError(t, j.part.NodeDatabase(0).View(func(b *database.Batch) error { return b.Account(target).Main().GetAs(&want) }))
	require.Equal(t, want.Balance.String(), got.Balance.String())
	require.Equal(t, "8", got.Balance.String(), "the deposit was not executed onto the account's loaded state")
	j.agree("after the block that loaded a section")
}

// TestAKeyPageInASectionNotLoadedIsLoadedForTheSignatureAndTheBlockIsUnchanged
// — #4485. dave's key page is in a section the node has not loaded. A
// transaction dave signs reads it to check the signature; the node loads it,
// and the block is the validators' block.
func TestAKeyPageInASectionNotLoadedIsLoadedForTheSignatureAndTheBlockIsUnchanged(t *testing.T) {
	j := joinWithSectionsNotLoaded(t)
	page := j.dave.JoinPath("book", "1")
	require.True(t, j.notLoaded(page), "precondition: dave's key page is in a section the node has loaded")

	st := j.sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(j.dave, "tokens").
			SendTokens(3, 0).To(j.bob, "tokens").
			SignWith(j.dave, "book", "1").Version(1).Timestamp(1).PrivateKey(j.daveKey))
	j.sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	j.sim.StepN(3)

	require.False(t, j.notLoaded(page), "the signature's read of the key page did not load its section")
	j.agree("after a block that read a key page in a section not loaded")
}

// fill runs the node's fill beside the network until every section is
// loaded, stepping the network and checking the roots agree at every step,
// or until stop says to stop.
func (j *sectionsJoin) fill(stop func(left int) bool) {
	t := j.t
	state := j.part.NodeJoinState(j.fresh)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); state.Fill(ctx) }()
	defer func() { cancel(); <-done }()
	for i := 0; i < 400; i++ {
		left := state.SectionsNotLoaded()
		if left == 0 || stop(left) {
			return
		}
		if i%10 == 0 {
			j.send()
		}
		j.sim.StepN(1)
		j.agree(fmt.Sprintf("while the fill runs, %d sections left", left))
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("the fill did not finish: %d sections left", state.SectionsNotLoaded())
}

// holdsAll checks that the fresh node holds every account only the
// validators held, as they hold it, and no marker.
func (j *sectionsJoin) holdsAll() {
	t := j.t
	fresh := j.part.NodeDatabase(j.fresh).Ungated().Begin(false)
	defer fresh.Discard()
	peer := j.part.NodeDatabase(0).Begin(false)
	defer peer.Discard()
	for _, u := range append(append([]*url.URL{}, j.many...), j.dave.JoinPath("book", "1"), j.dave.JoinPath("tokens")) {
		want, err := peer.Account(u).Hash()
		require.NoError(t, err)
		got, err := fresh.Account(u).Hash()
		require.NoError(t, err)
		require.Equal(t, want, got, "%v is not the validator's", u)
	}
	for i := 0; i < bpt.Sections; i++ {
		_, err := fresh.BPT().Get(bpt.Section(i).MarkerKey())
		require.ErrorIs(t, err, errors.NotFound, "section %d is still not loaded", i)
	}
}

// TestTheFillLoadsEverySectionAndTheRootMatchesThroughout — #4485. The fill
// runs beside execution until every section is loaded; at every block the
// node's root is the validators'.
func TestTheFillLoadsEverySectionAndTheRootMatchesThroughout(t *testing.T) {
	j := joinWithSectionsNotLoaded(t)
	require.Positive(t, j.notLoadedAtHandoff)
	j.fill(func(int) bool { return false })
	j.agree("once every section is loaded")
	j.holdsAll()
}

// TestARestartMidFillResumes — #4485. The node restarts with part of the
// tree loaded. The sections it had not loaded are still not loaded after the
// restart -- the markers are in its tree -- and once it has joined again the
// fill loads the rest.
func TestARestartMidFillResumes(t *testing.T) {
	j := joinWithSectionsNotLoaded(t)
	start := j.notLoadedAtHandoff
	require.Positive(t, start)
	j.fill(func(left int) bool { return left < start/2 })
	before := j.part.NodeJoinState(j.fresh).SectionsNotLoaded()
	require.Positive(t, before, "precondition: the fill finished before the restart")
	require.Less(t, before, start, "precondition: the fill loaded nothing before the restart")

	j.part.RestartNode(j.fresh)
	require.Equal(t, before, j.part.NodeJoinState(j.fresh).SectionsNotLoaded(),
		"the restarted node does not know which sections it had not loaded")

	j.run()
	j.fill(func(int) bool { return false })
	j.agree("after the restart's fill")
	j.holdsAll()
}

// TestATrulyFreshJoinerTakesTheNetworksOracleAndHandsOff — #4485 review,
// finding 1. The joiner is a new install -- genesis and nothing else -- and
// the partition's oracle is not genesis's. The handoff reloads the executor's globals from the state it
// settles at -- the oracle and the routing table among them -- through a
// batch no section loads into, so every system account the handoff reads
// must be loaded by the join. The node hands off, holds the partition's
// oracle, and executes in agreement with the validators.
func TestATrulyFreshJoinerTakesTheNetworksOracleAndHandsOff(t *testing.T) {
	j := joinWithSectionsNotLoaded(t)
	require.Positive(t, j.notLoadedAtHandoff, "precondition: the node handed off with every section loaded")

	oracle := PartitionUrl("BVN0").JoinPath(Oracle)
	g := new(network.GlobalValues)
	require.NoError(t, j.part.NodeDatabase(j.fresh).View(func(b *database.Batch) error {
		return g.LoadOracle(PartitionUrl("BVN0"), func(u *url.URL, v interface{}) error { return b.Account(u).Main().GetAs(v) })
	}), "the joiner cannot read %v through a batch that loads nothing", oracle)
	require.Equal(t, uint64(sectionsOraclePrice), g.Oracle.Price, "the joiner does not hold the partition's oracle")

	// Every system account the node reads outside a block is loaded.
	for _, u := range join.SystemAccounts(PartitionUrl("BVN0")) {
		require.False(t, j.notLoaded(u), "%v is in a section the joiner has not loaded", u)
	}

	for i := 0; i < 3; i++ {
		j.send()
	}
	j.sim.StepN(5)
	j.agree("after a truly fresh joiner executed")
}
