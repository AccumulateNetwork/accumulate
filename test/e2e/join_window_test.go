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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAFollowerJoinsByItsWindow — #4452. A follower syncs by the BPT, the
// accounts and the transactions arriving from consensus; none of that needs
// history, which is collected after the match. On a network older than the
// anchor pool's window (pull.AnchorPoolWindow) a fresh node joins holding
// the pool's recent window and not its first entries; the first block it
// opens seeds the synthetic cache from what it holds, it executes as its
// peers do, and a restart finds its meeting point again. The backfill then
// brings the history in.
func TestAFollowerJoinsByItsWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the network past the anchor pool's window")
	}
	for _, partition := range []string{"BVN0", Directory} {
		t.Run(partition, func(t *testing.T) { followerJoinsByItsWindow(t, partition) })
	}
}

func followerJoinsByItsWindow(t *testing.T, partition string) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	net, _ := networkWithAFollower(t.Name(), 1, 3)
	sim := NewSim(t,
		simulator.WithNetwork(net),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(1024),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	part := sim.S.Partition(partition)
	const fresh = 3 // networkWithAFollower appends it after the validators
	part.RestartNode(fresh)

	var ts uint64
	send := func() {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}

	// History past the window by more than a mark set of the pool's
	// entries (a window starts on a mark point): synthetic traffic
	// throughout, so the synthetic ledger and the pool carry entries at
	// every age.
	pool := PartitionUrl(partition).JoinPath(AnchorPool)
	for ledgerIndex(t, part.NodeDatabase(0), partition) < pull.AnchorPoolWindow+2400 {
		send()
		sim.StepN(20)
	}

	join1 := func(what string, afterHandoff func()) *windowAtHandoff {
		t.Helper()
		part.RestartNode(fresh)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		const maxRounds = 200
		stepping := &steppingState{cancel: cancel, State: part.NodeJoinState(fresh), step: func(round int) {
			if round%5 == 0 {
				send()
			}
			sim.StepN(3)
			if round >= maxRounds {
				cancel()
			}
		}}
		settler, ok := part.NodeExecutor(fresh).(join.Settler)
		require.True(t, ok)
		// The watch compares each block executed since the handoff against
		// its anchored root, from the ledger's bpt index chain as the window
		// left it; a comparison that cannot be made is a watch gone blind.
		watching := &watchErrors{steppingState: stepping}
		seen := &windowAtHandoff{handoffCounting: handoffCounting{Buffer: part.NodeJoin(fresh)}, db: part.NodeDatabase(fresh), pool: pool, after: afterHandoff}
		began := time.Now()
		outcome, err := join.Run(ctx, join.Options{
			Partition: partition,
			Buffer:    seen,
			Stage:     &join.ExecutorStage{Settler: settler, Staging: part.NodeStaging(fresh), Database: part.NodeDatabase(fresh)},
			State:     watching,
			Peers:     &join.APIPeers{Partition: partition, Client: sim.S.Services(), Network: t.Name()},
			Retry:     time.Millisecond,
		})
		require.NoError(t, err, "%s: the node did not join within %d pull rounds", what, maxRounds)
		require.Equal(t, join.Joined, outcome)
		require.False(t, part.Joining(fresh))
		require.Empty(t, seen.failed, "%s: a handoff failed to produce a block: the seed could not read what the window left", what)
		require.Empty(t, watching.errs, "%s: the node could not compare its executed roots with their anchors", what)
		require.NotZero(t, watching.calls, "%s: the watch never ran", what)
		t.Logf("%s: joined in %v; at the handoff the pool's main chain held %d of %d entries, its anchor-sequence chain %d of %d, the ledger's bpt index chain %d of %d",
			what, time.Since(began), seen.main[0], seen.main[1], seen.sequence[0], seen.sequence[1], seen.bpt[0], seen.bpt[1])
		return seen
	}
	agrees := func(what string) {
		t.Helper()
		for i := 0; i < 3; i++ {
			send()
		}
		sim.StepN(10)
		a, ra := ledgerRoot(t, part.NodeDatabase(0), partition)
		b, rb := ledgerRoot(t, part.NodeDatabase(fresh), partition)
		require.Equal(t, a, b, "%s: the node is not at its peers' block", what)
		require.Equal(t, ra, rb, "%s: the node's state is not its peers' at block %d", what, a)
	}

	// Past the first handoff the peers serve no chain from its first entry
	// a page at a time, as the backfill asks, so it cannot run until the
	// test lets it; the window and the restart's pull ask for none of that.
	refuse := func() {
		part.SetSourceWrap(fresh, func(srcs []pull.Source) []pull.Source {
			out := make([]pull.Source, len(srcs))
			for i, s := range srcs {
				out[i] = noBackfill{s}
			}
			return out
		})
	}
	first := join1("the join", refuse)
	require.Less(t, first.main[0], first.main[1], "the join took the pool's whole main chain before its handoff")
	require.Less(t, first.sequence[0], first.sequence[1], "the join took the pool's whole anchor-sequence chain before its handoff")
	require.Less(t, first.bpt[0], first.bpt[1], "the join took the ledger's whole bpt index chain before its handoff")

	// A process start before the backfill has run: the node finds where its
	// executed state and the peers' meet from what the window left it.
	second := join1("the restart", nil)
	require.Less(t, second.bpt[0], second.bpt[1], "precondition: the ledger was backfilled before the restart")
	agrees("after the restart")

	// The backfill, let run: the history comes in after the match, while the
	// node executes.
	watch := part.NodeJoinState(fresh)
	part.SetSourceWrap(fresh, nil)

	// The watch's first check since: it compares each block executed since
	// the handoff against its anchored root, from the ledger's bpt index
	// chain as the window left it, before the backfill that ends the check
	// brings the rest.
	_, diverged, err := watch.Diverged(context.Background())
	require.NoError(t, err, "the restarted node could not compare its roots from what the window left it")
	require.False(t, diverged)
	whole := func() bool {
		b := part.NodeDatabase(fresh).Begin(false)
		defer b.Discard()
		for _, h := range [][2]int64{heldOf(b, pool, "main"), heldOf(b, pool, "anchor-sequence"), heldOf(b, pool.RootIdentity().JoinPath(Ledger), "bpt-index")} {
			if h[0] != h[1] {
				return false
			}
		}
		return true
	}
	for i := 0; i < 20 && !whole(); i++ {
		sim.StepN(3)
		_, _, err := watch.Diverged(context.Background())
		require.NoError(t, err)
	}
	require.True(t, whole(), "the backfill did not bring the history in")
	agrees("after the backfill")

	// And a process start with the history held.
	last := join1("the restart after the backfill", nil)
	require.Equal(t, last.main[0], last.main[1])
	agrees("after the second restart")

	// The trust chain is taken whole, and it is small.
	View(t, part.NodeDatabase(fresh), func(batch *database.Batch) {
		for _, u := range pull.WholeAccounts(PartitionUrl(partition)) {
			chains, err := batch.Account(u).Chains().Get()
			require.NoError(t, err)
			var n int64
			for _, meta := range chains {
				c, err := batch.Account(u).ChainByName(meta.Name)
				require.NoError(t, err)
				head, err := c.Head().Get()
				require.NoError(t, err)
				n += head.Count
			}
			t.Logf("taken whole: %v, %d chain entries", u, n)
		}
	})
}

// windowAtHandoff records, at the first handoff that produces a block, how
// many entries of the pool's main and anchor-sequence chains the node holds
// against their height.
type windowAtHandoff struct {
	handoffCounting
	db             *database.Database
	pool           *url.URL
	done           bool
	main, sequence [2]int64
	bpt            [2]int64 // the ledger's bpt index chain
	after          func()
}

// watchErrors records what the root watch could not compare.
type watchErrors struct {
	*steppingState
	calls int
	errs  []error
}

func (w *watchErrors) Diverged(ctx context.Context) (uint64, bool, error) {
	n, diverged, err := w.steppingState.Diverged(ctx)
	w.calls++
	if err != nil && ctx.Err() == nil {
		w.errs = append(w.errs, err)
	}
	return n, diverged, err
}

// noBackfill is a peer that refuses a chain's first page, which only the
// backfill asks for of a chain of a page or more.
type noBackfill struct{ pull.Source }

func (s noBackfill) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	if r := q.Range; r != nil && r.Start == 0 && r.Count != nil && *r.Count >= 256 {
		return nil, errors.NotReady.With("this test's peers serve no backfill until it lets them")
	}
	return s.Source.QueryChainEntries(ctx, u, q)
}

func (w *windowAtHandoff) Handoff(q uint64) error {
	if !w.done {
		w.done = true
		batch := w.db.Begin(false)
		w.main = heldOf(batch, w.pool, "main")
		w.sequence = heldOf(batch, w.pool, "anchor-sequence")
		w.bpt = heldOf(batch, w.pool.RootIdentity().JoinPath(Ledger), "bpt-index")
		batch.Discard()
		if w.after != nil {
			w.after()
		}
	}
	return w.handoffCounting.Handoff(q)
}

func heldOf(batch *database.Batch, u *url.URL, name string) [2]int64 {
	c, err := batch.Account(u).ChainByName(name)
	if err != nil {
		return [2]int64{}
	}
	head, err := c.Head().Get()
	if err != nil {
		return [2]int64{}
	}
	var held int64
	for i := int64(0); i < head.Count; i++ {
		if _, err := c.Inner().Entry(i); err == nil {
			held++
		}
	}
	return [2]int64{held, head.Count}
}

func ledgerIndex(t *testing.T, db *database.Database, partition string) uint64 {
	i, _ := ledgerRoot(t, db, partition)
	return i
}

func ledgerRoot(t *testing.T, db *database.Database, partition string) (uint64, [32]byte) {
	t.Helper()
	var index uint64
	var root [32]byte
	View(t, db, func(batch *database.Batch) {
		var ledger *SystemLedger
		require.NoError(t, batch.Account(PartitionUrl(partition).JoinPath(Ledger)).Main().GetAs(&ledger))
		index = ledger.Index
		var err error
		root, err = batch.GetBptRootHash()
		require.NoError(t, err)
	})
	return index, root
}
