// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
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

// TestABlockThatWritesAColdAccountIsNotHeldAndTheRootIsRepaired — #4485,
// DIFFERENCES E24 (Paul, 2026-10-02: no retention, no as-of-H load).
//
// The trigger: the node executes a block that writes an account in a section
// it has not loaded, after its peers have executed that block. The account
// was last written before any retained block -- idle past the retention
// window -- so no peer holds its state as of the block before, and the
// peers' current section is not the one the node's marker holds. Before
// Paul's decision the load asked as of H, was refused by every peer, and held
// the block. Now the node takes the peers' current section, executes, and its
// root differs from the anchor; that is the anchor mismatch of "Two
// mismatches", repaired from the block ledger by the production join.Run,
// and the node then agrees with its peers.
//
// The node lags its peers here as a joiner does: the deposit is executed by
// the validators while the joiner is still collecting, and the joiner
// executes it from its buffer when it hands off.
func TestABlockThatWritesAColdAccountIsNotHeldAndTheRootIsRepaired(t *testing.T) {
	j := prepareSectionsJoin(t)
	sim := j.sim
	state := j.part.NodeJoinState(j.fresh)

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
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(j.alice, "tokens").
				SendTokens(7, 0).To(target).
				SignWith(j.alice, "book", "1").Version(1).Timestamp(j.ts).PrivateKey(j.aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
		deposited, _ = j.at(0)
		require.Greater(t, deposited, q, "precondition: the deposit is in a block the joiner executes after its handoff")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rounds := 0
	w := &untilAgreed{State: state, cancel: cancel}
	w.step = func() {
		rounds++
		if rounds%5 == 0 {
			j.send()
		}
		sim.StepN(3)
		if rounds >= 300 {
			cancel()
		}
	}
	w.done = func() bool {
		if target == nil || state.Machine().State() != nodestate.StateActive {
			return false
		}
		fb, fr := j.at(j.fresh)
		vb, vr := j.at(0)
		return fb > deposited && fb == vb && fr == vr
	}

	settler, ok := j.part.NodeExecutor(j.fresh).(join.Settler)
	require.True(t, ok)
	start := time.Now()
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    buf,
		Stage:     &join.ExecutorStage{Settler: settler, Staging: j.part.NodeStaging(j.fresh), Database: j.part.NodeDatabase(j.fresh)},
		State:     w,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)
	t.Logf("deposit at block %d; demoted at %v; %d rounds, %v", deposited, w.demoted, rounds, time.Since(start))

	require.NotNil(t, target, "the join never handed off")
	require.Empty(t, buf.refused, "the block that wrote the cold account was held: the handoff could not produce it")
	require.True(t, w.done(), "the node did not come to agree with its peers within %d rounds", rounds)
	require.NotEmpty(t, w.demoted, "the node's root never differed from its anchor: the cold account's section was not taken as its peers hold it now")

	var got, want *TokenAccount
	require.NoError(t, j.part.NodeDatabase(j.fresh).Ungated().View(func(b *database.Batch) error { return b.Account(target).Main().GetAs(&got) }))
	require.NoError(t, j.part.NodeDatabase(0).View(func(b *database.Batch) error { return b.Account(target).Main().GetAs(&want) }))
	require.Equal(t, want.Balance.String(), got.Balance.String(), "the repair did not bring the account to its peers' state")
}

// at is a node's block and root.
func (j *sectionsJoin) at(n int) (uint64, [32]byte) {
	t := j.t
	b := j.part.NodeDatabase(n).Ungated().Begin(false)
	defer b.Discard()
	var ledger *SystemLedger
	require.NoError(t, b.Account(PartitionUrl("BVN0").JoinPath(Ledger)).Main().GetAs(&ledger))
	root, err := b.GetBptRootHash()
	require.NoError(t, err)
	return ledger.Index, root
}

// beforeHandoff calls before once, at the join's first handoff, before the
// buffer produces anything, and keeps every error a handoff returns from
// then on: with every block collected, a handoff that fails is a block the
// node could not produce.
type beforeHandoff struct {
	*handoffCounting
	before  func(q uint64)
	called  bool
	refused []error
}

func (b *beforeHandoff) Handoff(q uint64) error {
	if !b.called {
		b.called = true
		b.before(q)
	}
	err := b.handoffCounting.Handoff(q)
	if err != nil {
		b.refused = append(b.refused, err)
	}
	return err
}

// untilAgreed is the join's state, stepping the network as the join pulls and
// as it watches the roots, and ending the join once done says so. It records
// every demotion: a mismatch the join repaired.
type untilAgreed struct {
	join.State
	step    func()
	done    func() bool
	cancel  context.CancelFunc
	demoted []uint64
}

func (w *untilAgreed) Pull(ctx context.Context) error {
	err := w.State.Pull(ctx)
	w.step()
	return err
}

func (w *untilAgreed) HandedOff(q uint64) {
	if h, ok := w.State.(interface{ HandedOff(uint64) }); ok {
		h.HandedOff(q)
	}
}

func (w *untilAgreed) Demote(n uint64) {
	w.demoted = append(w.demoted, n)
	w.State.Demote(n)
}

func (w *untilAgreed) Diverged(ctx context.Context) (uint64, bool, error) {
	w.step()
	n, diverged, err := w.State.(join.RootWatch).Diverged(ctx)
	if !diverged && w.done() {
		w.cancel()
	}
	return n, diverged, err
}
