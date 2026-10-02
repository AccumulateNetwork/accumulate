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
)

// The handoff is an overlap (executor spec, "Sync", §3a; #4495): for the
// first blocks after the match the node executes each block and takes the
// partition's record of it, the record's state standing where execution
// differs, until a run of join.OverlapBlocks clean blocks. Each test joins
// through the production join.Run, executor, staging and API peers.

// overlapJoin runs the production join of the sections fixture's fresh node
// until done says so or the rounds run out, stepping the network as it goes.
func overlapJoin(t *testing.T, j *sectionsJoin, buf *beforeHandoff, step func(round int), done func() bool) *untilAgreed {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rounds := 0
	w := &untilAgreed{State: j.part.NodeJoinState(j.fresh), cancel: cancel}
	w.step = func() {
		rounds++
		if step != nil {
			step(rounds)
		}
		if rounds%5 == 0 {
			j.send()
		}
		j.sim.StepN(3)
		if rounds >= 300 {
			cancel()
		}
	}
	w.done = done
	settler, ok := j.part.NodeExecutor(j.fresh).(join.Settler)
	require.True(t, ok)
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    buf,
		Stage:     &join.ExecutorStage{Settler: settler, Staging: j.part.NodeStaging(j.fresh), Database: j.part.NodeDatabase(j.fresh)},
		State:     w,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: j.sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)
	return w
}

// agreedPast is whether the fresh node is ACTIVE past block n, at its
// validators' block and root.
func (j *sectionsJoin) agreedPast(n uint64) bool {
	if j.part.NodeJoinState(j.fresh).Machine().State() != nodestate.StateActive {
		return false
	}
	fb, fr := j.at(j.fresh)
	vb, vr := j.at(0)
	return fb > n && fb == vb && fr == vr
}

// balance is a token account's balance on a node.
func (j *sectionsJoin) balance(n int, u *url.URL) string {
	db := j.part.NodeDatabase(n)
	if n == j.fresh {
		db = db.Ungated()
	}
	var acct *TokenAccount
	require.NoError(j.t, db.View(func(b *database.Batch) error { return b.Account(u).Main().GetAs(&acct) }))
	return acct.Balance.String()
}

// TestAnExecutionThatDiffersFromItsRecordSendsTheNodeBackToTheLedger: at the
// handoff, the fresh node's copy of an account in a section it holds loaded is
// made wrong behind its executor's back -- its main state, not its leaf, so the
// proven root still stands -- and a transfer to the account is executed by the
// validators while the node collects, so the node executes it in the overlap,
// behind its peers. Its execution leaves the account other than the record;
// the difference is counted, the record's state is written, and the node's
// root is its anchor's: no divergence and no repair. The overlap is held open
// for the test (SetOverlapBlocks), whatever the number of blocks collected.
func TestAnExecutionThatDiffersFromItsRecordSendsTheNodeBackToTheLedger(t *testing.T) {
	j := prepareSectionsJoin(t)
	state := j.part.NodeJoinState(j.fresh)
	state.SetOverlapBlocks(1 << 20)

	var target *url.URL
	var sent uint64
	buf := &beforeHandoff{handoffCounting: &handoffCounting{Buffer: j.part.NodeJoin(j.fresh)}}
	buf.before = func(q uint64) {
		for _, u := range j.many {
			if !j.notLoaded(u) {
				target = u
				break
			}
		}
		require.NotNil(t, target, "precondition: no account only the validators held is in a loaded section")
		b := j.part.NodeDatabase(j.fresh).Ungated().Begin(true)
		var acct *TokenAccount
		require.NoError(t, b.Account(target).Main().GetAs(&acct))
		acct.Balance.Add(&acct.Balance, big.NewInt(1000))
		require.NoError(t, b.Account(target).Main().Put(acct))
		require.NoError(t, b.Commit())

		j.ts++
		st := j.sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(j.alice, "tokens").
				SendTokens(7, 0).To(target).
				SignWith(j.alice, "book", "1").Version(1).Timestamp(j.ts).PrivateKey(j.aliceKey))
		j.sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
		sent, _ = j.at(0)
		require.Greater(t, sent, q, "precondition: the transfer is in a block the node executes after its handoff")
	}
	w := overlapJoin(t, j, buf, nil, func() bool { return sent > 0 && j.agreedPast(sent) })

	require.NotNil(t, target, "the join never handed off")
	require.True(t, w.done(), "the node did not come to agree with its peers")
	require.Empty(t, buf.refused)
	require.NotZero(t, state.Overlap().Differences, "the difference was not counted")
	require.NotEmpty(t, state.Overlap().Returns, "the difference did not send the node back to the block ledger")
	requireOnlyOverlapReturns(t, w.demoted, state, "the node's root differed from its anchor")
	require.Equal(t, j.balance(0, target), j.balance(j.fresh, target), "the record's state does not stand")
}

// TestTheOverlapEndsAfterItsCleanRunAndALaterColdLoadIsRepaired: the overlap
// ends after its run of clean blocks, and from there the node executes alone.
// TestTheOverlapHoldsWhileASectionIsNotLoaded: the clean run is a run of
// blocks with everything loaded. The run is one block here
// (SetOverlapBlocks), and a deposit to a cold account is executed by the
// validators while the node collects; the node then executes it from its
// buffer, loading the cold section as its peers hold it after the block. The
// overlap has not ended -- sections are still not loaded -- so the record
// settles the block and no anchor mismatch demotes the node (run
// 20261002T090546Z: an overlap that ended with 808 sections not loaded let a
// cold load diverge on the next block).
func TestTheOverlapHoldsWhileASectionIsNotLoaded(t *testing.T) {
	j := prepareSectionsJoin(t)
	state := j.part.NodeJoinState(j.fresh)
	state.SetOverlapBlocks(1)

	var target *url.URL
	var deposited uint64
	buf := &beforeHandoff{handoffCounting: &handoffCounting{Buffer: j.part.NodeJoin(j.fresh)}}
	buf.before = func(q uint64) {
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
		deposited, _ = j.at(0)
	}
	// The first overlap's end, before a demotion and match begin another.
	var ended join.OverlapStatus
	step := func(int) {
		if ended.EndedAt == 0 {
			ended = state.Overlap()
		}
	}
	w := overlapJoin(t, j, buf, step, func() bool { return deposited > 0 && j.agreedPast(deposited) })

	require.NotNil(t, target, "the join never handed off")
	require.True(t, ended.EndedAt == 0 || ended.EndedAt >= deposited,
		"the overlap ended at %d, before the cold deposit at %d, while sections were not loaded", ended.EndedAt, deposited)
	require.True(t, w.done(), "the node did not come to agree with its peers")
	requireOnlyOverlapReturns(t, w.demoted, state, "a cold load inside the overlap diverged at the root watch")
	require.Equal(t, j.balance(0, target), j.balance(j.fresh, target))
}

// TestARestartMidOverlapBeginsTheOverlapAgain: the overlap is memory. A node
// restarted while it runs joins again (a restart is a join), and the match the
// join reaches begins the overlap again, at the block it matched, with no
// clean block counted from before the restart. The first overlap is held open
// for the test (SetOverlapBlocks), so the node is stopped in it.
func TestARestartMidOverlapBeginsTheOverlapAgain(t *testing.T) {
	j := prepareSectionsJoin(t)

	// The first join, stopped in its overlap.
	state := j.part.NodeJoinState(j.fresh)
	state.SetOverlapBlocks(1 << 20)
	var mid join.OverlapStatus
	buf := &beforeHandoff{handoffCounting: &handoffCounting{Buffer: j.part.NodeJoin(j.fresh)}, before: func(uint64) {}}
	overlapJoin(t, j, buf, nil, func() bool {
		if state.Machine().State() != nodestate.StateActive {
			return false
		}
		mid = state.Overlap()
		return true
	})
	require.True(t, mid.Active, "precondition: the node was stopped outside its overlap (%+v)", mid)

	// Stopped, restarted, and moved on while it was down.
	j.part.RestartNode(j.fresh)
	for i := 0; i < 3; i++ {
		j.send()
	}
	j.sim.StepN(5)

	restarted := j.part.NodeJoinState(j.fresh)
	require.False(t, restarted.Overlap().Active, "the restarted node is in an overlap before it has matched")
	buf = &beforeHandoff{handoffCounting: &handoffCounting{Buffer: j.part.NodeJoin(j.fresh)}, before: func(uint64) {}}
	var again join.OverlapStatus
	sent, _ := j.at(0)
	w := overlapJoin(t, j, buf, nil, func() bool {
		if restarted.Machine().State() != nodestate.StateActive {
			return false
		}
		if again.From == 0 {
			again = restarted.Overlap()
		}
		return j.agreedPast(sent)
	})
	require.True(t, w.done(), "the restarted node did not come to agree with its peers")
	require.Greater(t, again.From, mid.From, "the overlap did not begin again at the restarted join's match")
	requireOnlyOverlapReturns(t, w.demoted, restarted, "the restarted node")
}
