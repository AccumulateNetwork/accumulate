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
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// repairFixture is a BVN1 validator down while a cross-partition send runs in
// every block (alice on BVN0 to bob on BVN1), so every BVN1 block delivers a
// synthetic deposit, and the join that brings it back.
type repairFixture struct {
	sim     *Sim
	p       *simulator.Partition
	part    *url.URL
	joiner  int
	traffic func()
	bob     *url.URL
	bobKey  []byte
}

func newRepairFixture(t *testing.T) *repairFixture {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e12))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	MakeAccount(t, sim.DatabaseFor(bob), &DataAccount{Url: bob.JoinPath("data")})

	var ts uint64
	f := &repairFixture{sim: sim, p: sim.S.Partition("BVN1"), part: PartitionUrl("BVN1"), joiner: 1, bob: bob, bobKey: bobKey}
	f.traffic = func() {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.Step()
	}
	for i := 0; i < 10; i++ {
		f.traffic()
	}
	// bob acts too, so his identity and his key page hold chains on every
	// node.
	st := sim.BuildAndSubmitTxnSuccessfully(build.Transaction().For(bob).
		CreateTokenAccount(bob, "savings").ForToken(AcmeUrl()).
		SignWith(bob, "book", "1").Version(1).Timestamp(1).PrivateKey(bobKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	st = sim.BuildAndSubmitTxnSuccessfully(build.Transaction().For(bob, "data").
		WriteData().DoubleHash([]byte("on every node")).
		SignWith(bob, "book", "1").Version(1).Timestamp(2).PrivateKey(bobKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())

	// Down: it stops executing, the network runs on, and it comes back with
	// an empty buffer, so it must pull.
	f.p.RestartNode(f.joiner)
	for i := 0; i < 30; i++ {
		f.traffic()
	}
	f.p.RestartNode(f.joiner)
	return f
}

// repairCounter is the join's state with the repairs it made counted: every
// executed block whose root differed from its signed anchor.
type repairCounter struct {
	*steppingState
	repairs   int
	onHandoff func()
}

func (r *repairCounter) HandedOff(q uint64) {
	r.steppingState.HandedOff(q)
	if r.onHandoff != nil {
		r.onHandoff()
	}
}

func (r *repairCounter) Diverged(ctx context.Context) (uint64, bool, error) {
	n, diverged, err := r.steppingState.Diverged(ctx)
	if diverged {
		r.repairs++
	}
	return n, diverged, err
}

// join runs the production join for the fixture's node, with traffic in every
// block, until the node is ACTIVE or maxRounds have passed. each runs on every
// round, before the network steps.
func (f *repairFixture) join(t *testing.T, each func(round int), onHandoff func(*repairCounter)) *repairCounter {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const maxRounds = 200
	state := f.p.NodeJoinState(f.joiner)
	counter := &repairCounter{steppingState: &steppingState{cancel: cancel, State: state, step: func(round int) {
		if each != nil {
			each(round)
		}
		f.traffic()
		if round >= maxRounds {
			cancel()
		}
	}}}
	if onHandoff != nil {
		counter.onHandoff = func() { onHandoff(counter) }
	}
	settler, ok := f.p.NodeExecutor(f.joiner).(join.Settler)
	require.True(t, ok)
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN1",
		Buffer:    f.p.NodeJoin(f.joiner),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: f.p.NodeStaging(f.joiner), Database: f.p.NodeDatabase(f.joiner)},
		State:     counter,
		Peers:     &join.APIPeers{Partition: "BVN1", Client: f.sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, nodestate.StateActive, state.Machine().State(),
		"the node was not proven within %d rounds: %d repairs", maxRounds, counter.repairs)
	return counter
}

// requireOneRootChain requires every node of the partition on one root chain
// after the network runs on with traffic.
func (f *repairFixture) requireOneRootChain(t *testing.T) {
	t.Helper()
	for i := 0; i < 10; i++ {
		f.traffic()
	}
	f.sim.StepN(10)
	var anchors [][]byte
	for i := 0; i < f.p.NodeCount(); i++ {
		View(t, f.p.NodeDatabase(i), func(batch *database.Batch) {
			a, err := batch.Account(f.part.JoinPath(Ledger)).RootChain().Anchor()
			require.NoError(t, err)
			anchors = append(anchors, a)
		})
	}
	for i := 1; i < len(anchors); i++ {
		require.Equal(t, anchors[0], anchors[i], "BVN1 node %d's root chain differs from node 0's after the join", i)
	}
}

// TestAJoinRepairsABlockExecutedWithoutItsSynthetic — executor spec, "Sync",
// "Two mismatches": a joining node executes blocks as they
// come, and one executed without a synthetic transaction it needed is wrong
// only in the accounts its record names. The joiner's copies of BVN1's blocks
// lose every synthetic deposit for its first rounds, its root then differs
// from the partition's signed anchor, and the node repairs from the block
// ledger and goes on until it matches.
func TestAJoinRepairsABlockExecutedWithoutItsSynthetic(t *testing.T) {
	f := newRepairFixture(t)

	var withhold atomic.Bool
	var withheld atomic.Int64
	withhold.Store(true)
	f.sim.S.SetNodeBlockHook("BVN1", func(node int, _ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		if node != f.joiner || !withhold.Load() {
			return envelopes, true
		}
		var kept []*messaging.Envelope
		for _, env := range envelopes {
			synthetic := false
			for _, m := range env.Messages {
				if _, ok := m.(*messaging.SyntheticMessage); ok {
					synthetic = true
				}
			}
			if synthetic {
				withheld.Add(1)
				continue
			}
			kept = append(kept, env)
		}
		return kept, true
	})

	// At every handoff after a repair, the repaired account is held whole
	// already -- every entry the node's chains count, with the message
	// behind each -- not only a head that hashes into a matching leaf.
	checked := 0
	counter := f.join(t, func(round int) {
		if round == 10 {
			withhold.Store(false)
		}
	}, func(c *repairCounter) {
		if c.repairs == 0 {
			return
		}
		requireHeldWhole(t, f.p.NodeDatabase(f.joiner), f.p.NodeDatabase(0), f.bob.JoinPath("tokens"))
		checked++
	})
	require.NotZero(t, checked, "precondition: no handoff followed a repair")
	require.NotZero(t, withheld.Load(), "precondition: no synthetic was withheld from the joiner")
	require.NotZero(t, counter.repairs, "the joiner never found a mismatch, so nothing was repaired")
	t.Logf("%d envelopes withheld; %d repairs before the match", withheld.Load(), counter.repairs)

	// The account the withheld deposits were for was repaired whole: its
	// chains are the peers', entry by entry.
	requireSameChains(t, f.p.NodeDatabase(f.joiner), f.p.NodeDatabase(0), f.bob.JoinPath("tokens"))
	f.requireOneRootChain(t)
}

// TestAJoinDeletesAnAccountOnlyItsOwnExecutionCreated — executor spec, "Sync",
// "Two mismatches": "A node that executed an account into
// existence that no peer holds loses it at the repair, because its own record
// names it." The joiner alone is handed a transaction creating bob/extra; it
// executes it, its root differs from the partition's, and the repair, reading
// the joiner's own block-ledger record, finds the account no peer holds and
// deletes it.
func TestAJoinDeletesAnAccountOnlyItsOwnExecutionCreated(t *testing.T) {
	f := newRepairFixture(t)
	extra := f.bob.JoinPath("extra")

	env := MustBuild(t, build.Transaction().For(f.bob).
		CreateTokenAccount(f.bob, "extra").ForToken(AcmeUrl()).
		SignWith(f.bob, "book", "1").Version(1).Timestamp(3).PrivateKey(f.bobKey))
	// A scratch write gives bob/data a chain no peer's bob/data has.
	scratch := MustBuild(t, build.Transaction().For(f.bob, "data").
		WriteData().DoubleHash([]byte("only the joiner")).Scratch().
		SignWith(f.bob, "book", "1").Version(1).Timestamp(4).PrivateKey(f.bobKey))
	// The next block the joiner is handed carries the creation, and no
	// other node's does.
	var injected atomic.Bool
	f.sim.S.SetNodeBlockHook("BVN1", func(node int, _ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		if node != f.joiner || injected.Swap(true) {
			return envelopes, true
		}
		return append(envelopes, env, scratch), true
	})

	// The joiner's own execution grows bob's key page's chains -- it pays
	// for the creation -- beyond what any peer's hold.
	page := f.bob.JoinPath("book", "1")
	grew := false
	created := false
	scratched := false
	counter := f.join(t, func(round int) {
		View(t, f.p.NodeDatabase(f.joiner), func(batch *database.Batch) {
			chains, err := batch.Account(f.bob.JoinPath("data")).Chains().Get()
			require.NoError(t, err)
			for _, c := range chains {
				if c.Name == "scratch" {
					scratched = true
				}
			}
		})
		if chainHeight(t, f.p.NodeDatabase(f.joiner), page, "main") > chainHeight(t, f.p.NodeDatabase(0), page, "main") ||
			chainHeight(t, f.p.NodeDatabase(f.joiner), page, "signature") > chainHeight(t, f.p.NodeDatabase(0), page, "signature") {
			grew = true
		}
		if !created {
			View(t, f.p.NodeDatabase(f.joiner), func(batch *database.Batch) {
				_, err := batch.Account(extra).Main().Get()
				created = err == nil
			})
		}
	}, nil)
	require.True(t, created, "precondition: the joiner never executed its own creation of %v", extra)
	View(t, f.sim.S.Partition("BVN1").NodeDatabase(0), func(batch *database.Batch) {
		_, err := batch.Account(extra).Main().Get()
		require.ErrorIs(t, err, errors.NotFound, "precondition: a peer holds %v", extra)
	})
	require.NotZero(t, counter.repairs, "the joiner never found a mismatch, so nothing was repaired")

	View(t, f.p.NodeDatabase(f.joiner), func(batch *database.Batch) {
		_, err := batch.Account(extra).Main().Get()
		require.ErrorIs(t, err, errors.NotFound, "the joiner still holds %v, which no peer holds", extra)
		_, err = batch.BPT().Get(record.NewKey("Account", extra))
		require.ErrorIs(t, err, errors.NotFound, "the joiner's tree still holds a leaf for %v", extra)

		// Removed whole: nothing of it is left to hold (invariant 13).
		chains, err := batch.Account(extra).Chains().Get()
		require.NoError(t, err)
		require.Empty(t, chains, "the joiner still lists chains of %v, which it deleted", extra)
		c, err := batch.Account(extra).ChainByName("main")
		require.NoError(t, err)
		head, err := c.Inner().Head().Get()
		require.NoError(t, err)
		require.Zero(t, head.Count, "the joiner still holds the main chain of %v, which it deleted", extra)
	})
	t.Logf("%d repairs before the match", counter.repairs)

	// The chains the joiner grew wrongly are the peers' again, not the
	// peers' appended to its own.
	require.True(t, grew, "precondition: the joiner's execution never grew %v's chains beyond the peers'", page)
	requireSameChains(t, f.p.NodeDatabase(f.joiner), f.p.NodeDatabase(0), page)
	// And bob/data lists the peers' chains, not the scratch chain only the
	// joiner's execution created.
	require.True(t, scratched, "precondition: the joiner never listed a scratch chain on bob/data")
	requireSameChains(t, f.p.NodeDatabase(f.joiner), f.p.NodeDatabase(0), f.bob.JoinPath("data"))
	f.requireOneRootChain(t)
}

// requireSameChains requires an account's chains on node to equal its chains
// on peer entry by entry: the same chains, the same heights, and the same
// entry at every index -- not only a leaf that hashes the same (executor
// spec, "Sync", "Two mismatches": the node ends holding every
// account's chains and entries).
func requireSameChains(t *testing.T, node, peer *database.Database, u *url.URL) {
	t.Helper()
	// The chains the account's index names, and its main and signature
	// chains, which are read whether or not the index lists them. A chain
	// of height zero is one the account does not hold.
	names := func(batch *database.Batch) []string {
		seen := map[string]bool{"main": true, "signature": true}
		out := []string{"main", "signature"}
		chains, err := batch.Account(u).Chains().Get()
		require.NoError(t, err)
		for _, meta := range chains {
			if !seen[meta.Name] {
				seen[meta.Name] = true
				out = append(out, meta.Name)
			}
		}
		return out
	}
	read := func(db *database.Database) map[string][][]byte {
		out := map[string][][]byte{}
		View(t, db, func(batch *database.Batch) {
			for _, name := range names(batch) {
				c, err := batch.Account(u).ChainByName(name)
				require.NoError(t, err)
				head, err := c.Inner().Head().Get()
				require.NoError(t, err)
				if head.Count == 0 {
					continue
				}
				var entries [][]byte
				for i := int64(0); i < head.Count; i++ {
					e, err := c.Inner().Entry(i)
					require.NoError(t, err, "%v chain %s entry %d of %d", u, name, i, head.Count)
					entries = append(entries, e)
				}
				out[name] = entries
			}
		})
		return out
	}
	want, got := read(peer), read(node)
	require.NotEmpty(t, want, "precondition: the peer holds no chain of %v", u)

	// The chain lists are the same, both ways: a chain only the node lists
	// is hashed into its leaf and is not the peers'.
	listed := func(db *database.Database) []string {
		var out []string
		View(t, db, func(batch *database.Batch) {
			chains, err := batch.Account(u).Chains().Get()
			require.NoError(t, err)
			for _, meta := range chains {
				out = append(out, meta.Name)
			}
		})
		return out
	}
	require.ElementsMatch(t, listed(peer), listed(node), "%v: the node lists other chains than the peer", u)
	for name, entries := range want {
		require.Equal(t, len(entries), len(got[name]), "%v chain %s: the node holds another height", u, name)
		for i := range entries {
			require.Equal(t, entries[i], got[name][i], "%v chain %s entry %d differs", u, name, i)
		}
	}
}

// chainHeight is the height of one of an account's chains in db, zero when
// the account or chain is not there.
func chainHeight(t *testing.T, db *database.Database, u *url.URL, name string) int64 {
	t.Helper()
	var n int64
	View(t, db, func(batch *database.Batch) {
		c, err := batch.Account(u).ChainByName(name)
		if err != nil {
			return
		}
		head, err := c.Inner().Head().Get()
		if err == nil {
			n = head.Count
		}
	})
	return n
}

// requireHeldWhole requires every entry an account's main and signature chains
// count on node to be held there, equal to the peer's at the same index, with
// the message behind it: the account was taken whole, chains and entries, not
// by its heads.
func requireHeldWhole(t *testing.T, node, peer *database.Database, u *url.URL) {
	t.Helper()
	nb, pb := node.Begin(false), peer.Begin(false)
	defer nb.Discard()
	defer pb.Discard()
	for _, name := range []string{"main", "signature"} {
		nc, err := nb.Account(u).ChainByName(name)
		require.NoError(t, err)
		pc, err := pb.Account(u).ChainByName(name)
		require.NoError(t, err)
		head, err := nc.Inner().Head().Get()
		require.NoError(t, err)
		for i := int64(0); i < head.Count; i++ {
			e, err := nc.Inner().Entry(i)
			require.NoError(t, err, "%v chain %s: entry %d of %d is not held", u, name, i, head.Count)
			want, err := pc.Inner().Entry(i)
			if err != nil {
				continue // The peer has not got that far
			}
			require.Equal(t, want, e, "%v chain %s entry %d differs from the peer's", u, name, i)
			_, err = nb.Message([32]byte(e)).Main().Get()
			require.NoError(t, err, "%v chain %s entry %d: the message behind it is not held", u, name, i)
		}
	}
}

// provenWatch records the node's state at every handoff, so a test can say
// whether the node handed off from a state not yet proven.
type provenWatch struct {
	*steppingState
	state    *join.PulledState
	unproven int
}

func (w *provenWatch) HandedOff(q uint64) {
	if w.state.Machine().State() != nodestate.StateActive {
		w.unproven++
	}
	w.steppingState.HandedOff(q)
}

// TestARootWatchMatchMovesTheTrustedSets — #4438 threat review "in passing",
// code review F5. A node that hands off from a pulled state not yet proven is
// proven by the root watch (Diverged), not by Matched. The network definition
// changed while the node was down; at the root watch's match the state holding
// the new definition is proven, and the node trusts it from there. Before,
// only Matched moved the sets, so a node proven by the root watch verified with
// the definition it started with for as long as it ran.
func TestARootWatchMatchMovesTheTrustedSets(t *testing.T) {
	const joiner = 1
	// A joining node executes nothing, so it reports no results for the
	// blocks it collects; that is not a consensus failure.
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.StepN(10)
	p := sim.S.Partition("BVN0")
	part := PartitionUrl("BVN0")
	p.RestartNode(joiner)
	before := versionOf(t, p.NodeDatabase(joiner), part)

	// A governance write while the node is down: a validator added.
	current := loadDirectoryGlobals(t, sim)
	newKey := acctesting.GenerateKey(t.Name(), "new-validator")
	current.Network.AddValidator(newKey[32:], Directory, false)
	current.Network.Version++
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(DnUrl(), Network).
			Body(&WriteData{Entry: current.FormatNetwork(), WriteToState: true}).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(1).Signer(sim.SignWithNode(Directory, 0)).
			SignWith(DnUrl(), Operators, "1").Version(1).Timestamp(2).Signer(sim.SignWithNode(Directory, 1)))
	sim.StepUntil(Txn(st.TxID).Completes())
	sim.StepN(20)
	after := versionOf(t, p.NodeDatabase(0), part)
	require.Greater(t, after, before, "precondition: BVN0's network definition did not change")
	p.RestartNode(joiner)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const maxRounds = 120
	state := p.NodeJoinState(joiner)
	require.Equal(t, before, state.TrustedVersion(), "precondition: the node starts trusting the definition it executed with")
	watch := &provenWatch{state: state, steppingState: &steppingState{cancel: cancel, State: state, step: func(round int) {
		sim.StepN(3)
		if round >= maxRounds {
			cancel()
		}
	}}}
	settler, ok := p.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok)
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    p.NodeJoin(joiner),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(joiner), Database: p.NodeDatabase(joiner)},
		State:     watch,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, nodestate.StateActive, state.Machine().State(), "precondition: the node was never proven")
	require.NotZero(t, watch.unproven, "precondition: the node was proven at its handoff, by Matched, not by the root watch")
	require.Equal(t, after, state.TrustedVersion(),
		"the root watch proved a state holding definition version %d and the node still trusts version %d", after, state.TrustedVersion())
}
