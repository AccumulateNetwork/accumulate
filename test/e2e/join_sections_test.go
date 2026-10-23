// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
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
	// it handed off; notLoadedAtMatch, when its state first matched, before
	// it executed anything.
	notLoadedAtHandoff, notLoadedAtMatch int

	// roundsToMatch is how many pull rounds the join took to its first match.
	roundsToMatch int

	alice, bob *url.URL
	aliceKey   []byte
	ts         uint64

	// spread sends each deposit to the next of many, so the blocks the
	// joiner executes touch sections it has not loaded; buf, when set, is
	// the join's buffer.
	spread bool
	k      int
	buf    join.Buffer
	// depositsPerRound is how many deposits each round of the join submits
	// without waiting for them, to execute in the blocks the round's hook
	// steps; zero is one awaited send every fifth round.
	depositsPerRound int
}

const sectionsAccounts = 600

// sectionsOraclePrice is the partition's oracle when the joiner joins: not
// genesis's.
const sectionsOraclePrice = 31415

func joinWithSectionsNotLoaded(t *testing.T) *sectionsJoin {
	j := prepareSectionsJoin(t)
	j.run()
	return j
}

// prepareSectionsJoin builds the network and the reinstalled joiner, and
// restarts it, but does not run its join.
func prepareSectionsJoin(t *testing.T) *sectionsJoin { return prepareSectionsJoinBy(t, false) }

// prepareSectionsJoinBy is prepareSectionsJoin; byBlocks creates the many
// token accounts by transactions, so the validators' BPT history covers them
// as it covers every account of a live network, rather than writing them into
// the validators' stores outside any block, which no history can serve as of
// a block.
func prepareSectionsJoinBy(t *testing.T, byBlocks bool) *sectionsJoin {
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
	if byBlocks {
		// Lite token accounts, created by deposits as a live network's are
		// (an identity may not hold this many accounts): twenty a
		// transaction, each transaction executed before the next batch.
		j.many = j.many[:0]
		for i := 0; i < sectionsAccounts; i++ {
			h := sha256.Sum256([]byte(fmt.Sprintf("%s many %d", t.Name(), i)))
			u, err := LiteTokenAddressFromHash(h[:], "ACME")
			require.NoError(t, err)
			j.many = append(j.many, u)
		}
		for i := 0; i < len(j.many); i += 20 {
			end := min(i+20, len(j.many))
			j.ts++
			b := build.Transaction().For(j.alice, "tokens").SendTokens(1, 0).To(j.many[i])
			for _, u := range j.many[i+1 : end] {
				b = b.And(1, 0).To(u)
			}
			st := j.sim.BuildAndSubmitTxnSuccessfully(b.SignWith(j.alice, "book", "1").Version(1).Timestamp(j.ts).PrivateKey(j.aliceKey))
			sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
		}
		sim.StepN(3)
	}
	for n := 0; n < j.part.NodeCount(); n++ {
		if n == j.fresh {
			continue
		}
		db := j.part.NodeDatabase(n)
		b := db.Begin(true)
		if !byBlocks {
			for _, u := range j.many {
				require.NoError(t, b.Account(u).Main().Put(&TokenAccount{Url: u, TokenUrl: AcmeUrl(), Balance: *big.NewInt(1)}))
			}
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
			switch {
			case j.depositsPerRound > 0:
				for i := 0; i < j.depositsPerRound; i++ {
					j.submitDeposit()
				}
			case j.spread || round%5 == 0:
				j.send()
			}
			j.sim.StepN(3)
		}
		if round >= 200 {
			cancel()
		}
	}}
	j.notLoadedAtHandoff, j.notLoadedAtMatch = -1, -1
	settler, ok := j.part.NodeExecutor(j.fresh).(join.Settler)
	require.True(t, ok)
	buf := j.buf
	if buf == nil {
		buf = &handoffCounting{Buffer: j.part.NodeJoin(j.fresh)}
	}
	outcome, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    buf,
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

func (c *countingAtHandoff) Matched(ctx context.Context) (uint64, bool, error) {
	q, ok, err := c.steppingState.Matched(ctx)
	if ok && c.j.notLoadedAtMatch < 0 {
		c.j.notLoadedAtMatch = c.j.part.NodeJoinState(c.j.fresh).SectionsNotLoaded()
		c.j.roundsToMatch = c.steppingState.round
	}
	return q, ok, err
}

func (j *sectionsJoin) send() {
	j.ts++
	to := j.bob.JoinPath("tokens")
	if j.spread {
		to = j.many[j.k%len(j.many)]
		j.k++
	}
	st := j.sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(j.alice, "tokens").
			SendTokens(1, 0).To(to).
			SignWith(j.alice, "book", "1").Version(1).Timestamp(j.ts).PrivateKey(j.aliceKey))
	j.sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
}

// submitDeposit submits one deposit into the next of many without waiting
// for it: it executes in whatever block comes next.
func (j *sectionsJoin) submitDeposit() {
	j.ts++
	to := j.many[j.k%len(j.many)]
	j.k++
	j.sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(j.alice, "tokens").
			SendTokens(1, 0).To(to).
			SignWith(j.alice, "book", "1").Version(1).Timestamp(j.ts).PrivateKey(j.aliceKey))
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

// TestABlockExecutedBehindThePeersLoadsASectionAsOfItsOwnBlock — #4518, soak
// 20261005T043754Z. After the handoff the node executes the blocks it
// buffered while the validators move on, so every block it executes is behind
// them. A block that loads a section must take it as of its own block: taken
// as a peer holds it now, the section holds what the later blocks did to the
// account the block is about to change, the block changes it again, the root
// differs from the proven root and the node is demoted -- on the soak, within
// a block or two of every handoff, until every section was loaded. Here every
// block of the join deposits into an account only the validators held, in a
// section the joiner has not loaded, and the node must hand off once.
func TestABlockExecutedBehindThePeersLoadsASectionAsOfItsOwnBlock(t *testing.T) {
	j := prepareSectionsJoinBy(t, true)
	j.spread = true
	buf := &countingBuffer{Buffer: j.part.NodeJoin(j.fresh)}
	j.buf = buf
	j.run()
	require.Positive(t, j.notLoadedAtHandoff, "precondition: every section was loaded at the handoff")
	require.Len(t, buf.handoffs, 1, "the node handed off %d times (collected again %d times): a block it executed behind the peers diverged", len(buf.handoffs), buf.starts)
	// The blocks executed behind the peers did load sections: the walk is
	// not exercised otherwise.
	left := j.part.NodeJoinState(j.fresh).SectionsNotLoaded()
	require.Less(t, left, j.notLoadedAtMatch, "precondition: no block executed after the match loaded a section (%d not loaded, %d at the match)", left, j.notLoadedAtMatch)
	t.Logf("matched after %d pull rounds with %d sections not loaded; the blocks executed since loaded %d", j.roundsToMatch, j.notLoadedAtMatch, j.notLoadedAtMatch-left)
	j.agree("after joining under deposits into sections not loaded")
}

// TestThePullMatchesWhileEveryBlockOpensSections — #4518, soak
// 20261005T043754Z: the BVN pull took twenty minutes. A record naming an
// account in a section not loaded opens the section, and the pull loads it;
// loaded as the peer holds it now, the load is of a block past L, the pull is
// complete only once the records reach that block, and those records open
// more sections. At ten deposits a round into sections not loaded, the pull
// never matches; loading every opened section as of L, it matches in a round
// or two whatever the blocks do.
func TestThePullMatchesWhileEveryBlockOpensSections(t *testing.T) {
	j := prepareSectionsJoinBy(t, true)
	j.spread = true
	j.depositsPerRound = 10
	// The partition moves WHILE the node pulls, as a real one does: the
	// round's deposits into sections not loaded execute in a block between
	// two of the pull's reads (Partition.SetPullHook). Standing still, every
	// read of a round is of one block and no load can be past L.
	j.part.SetPullHook(j.fresh, func() { j.sim.Step() })
	defer j.part.SetPullHook(j.fresh, nil)
	j.run()
	require.LessOrEqual(t, j.roundsToMatch, 5, "the pull took %d rounds to match: it chased the blocks", j.roundsToMatch)
	t.Logf("matched after %d pull rounds with %d sections not loaded", j.roundsToMatch, j.notLoadedAtMatch)
	j.agree("after a join under deposits into sections not loaded every block")
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
func (j *sectionsJoin) fill(stop func(left int) bool) { j.fillFrom(context.Background(), stop) }

// fillFrom is fill, with the fill's context derived from parent.
func (j *sectionsJoin) fillFrom(parent context.Context, stop func(left int) bool) {
	t := j.t
	state := j.part.NodeJoinState(j.fresh)
	ctx, cancel := context.WithCancel(parent)
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
//
// The fill is held after its first span. A block writes every section the
// fill has ready, and the fill's spans run in parallel, so left alone it can
// have the whole tree ready for one block: 563 sections not loaded went to 0
// in one block in 1 run of 60 here, and stopping below half failed 9 of 60.
func TestARestartMidFillResumes(t *testing.T) {
	j := joinWithSectionsNotLoaded(t)
	start := j.notLoadedAtHandoff
	require.Positive(t, start)
	hold := &firstSpanOnly{}
	j.part.SetSourceWrap(j.fresh, hold.wrap)
	j.fillFrom(context.WithValue(context.Background(), heldFill{}, true), func(left int) bool { return left < start })
	j.part.SetSourceWrap(j.fresh, nil)
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

// heldFill marks the context of a fill that firstSpanOnly holds.
type heldFill struct{}

// firstSpanOnly serves a held fill the page of the first span it asks for,
// as often as it asks, and holds every other page until the fill ends. What
// else asks a page -- a block's load -- is served.
type firstSpanOnly struct {
	mu    sync.Mutex
	first *[32]byte
}

func (h *firstSpanOnly) wrap(srcs []pull.Source) []pull.Source {
	out := make([]pull.Source, len(srcs))
	for i, s := range srcs {
		out[i] = heldSource{s, h}
	}
	return out
}

type heldSource struct {
	pull.Source
	hold *firstSpanOnly
}

func (s heldSource) QueryBptPage(ctx context.Context, scope *url.URL, q *api.BptPageQuery) (*api.BptPageRecord, error) {
	if ctx.Value(heldFill{}) != nil {
		s.hold.mu.Lock()
		if s.hold.first == nil {
			first := q.StartHash
			s.hold.first = &first
		}
		served := *s.hold.first == q.StartHash
		s.hold.mu.Unlock()
		if !served {
			<-ctx.Done()
			return nil, ctx.Err()
		}
	}
	return s.Source.QueryBptPage(ctx, scope, q)
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
