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
	repairs int
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
func (f *repairFixture) join(t *testing.T, each func(round int)) *repairCounter {
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
// "Execute, and repair on a mismatch": a joining node executes blocks as they
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

	counter := f.join(t, func(round int) {
		if round == 10 {
			withhold.Store(false)
		}
	})
	require.NotZero(t, withheld.Load(), "precondition: no synthetic was withheld from the joiner")
	require.NotZero(t, counter.repairs, "the joiner never found a mismatch, so nothing was repaired")
	t.Logf("%d envelopes withheld; %d repairs before the match", withheld.Load(), counter.repairs)
	f.requireOneRootChain(t)
}

// TestAJoinDeletesAnAccountOnlyItsOwnExecutionCreated — executor spec, "Sync",
// "Execute, and repair on a mismatch": "A node that executed an account into
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
		SignWith(f.bob, "book", "1").Version(1).Timestamp(1).PrivateKey(f.bobKey))
	// The next block the joiner is handed carries the creation, and no
	// other node's does.
	var injected atomic.Bool
	f.sim.S.SetNodeBlockHook("BVN1", func(node int, _ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		if node != f.joiner || injected.Swap(true) {
			return envelopes, true
		}
		return append(envelopes, env), true
	})

	created := false
	counter := f.join(t, func(round int) {
		if !created {
			View(t, f.p.NodeDatabase(f.joiner), func(batch *database.Batch) {
				_, err := batch.Account(extra).Main().Get()
				created = err == nil
			})
		}
	})
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
	})
	t.Logf("%d repairs before the match", counter.repairs)
	f.requireOneRootChain(t)
}
