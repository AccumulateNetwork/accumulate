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

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/enumerate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/tracker"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
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

// emptyDb is a node's data directory before it has any state — or, for the
// second half of TestPullReachesTheAnchoredRoot, after it has been restored to
// block R.
func emptyDb() *database.Database {
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	return db
}

func bptRoot(t *testing.T, db *database.Database) [32]byte {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	r, err := b.GetBptRootHash()
	require.NoError(t, err)
	return r
}

// pullEvery fetches every named account from src while the network is frozen,
// holds them unverified, and returns them with the block the peer served them
// at. Every account must come from the same block, or the assembled state is a
// mixture of two and hashes to neither.
func pullEvery(t *testing.T, src pull.Source, batch *database.Batch, part *url.URL, accounts []*url.URL) ([]*pull.Pending, uint64) {
	t.Helper()
	var held []*pull.Pending
	var block uint64
	for _, u := range accounts {
		p, err := pull.Fetch(context.Background(), src, batch, u, pull.Options{Mode: pull.ModeStateOnly, Partition: part}, true)
		require.NoError(t, err, "fetch %v", u)
		if block == 0 {
			block = p.Block
		}
		require.Equal(t, block, p.Block, "%v was served at a different block; the network moved during the pull", u)
		held = append(held, p)
	}
	require.NotZero(t, block)
	return held, block
}

// waitForAnchor steps the simulator until the Directory has anchored the
// partition's block, and returns the root it anchored. The pull runs ahead of
// the anchors by design: a peer serves its current block, and the Directory
// anchors it a few blocks later.
func waitForAnchor(t *testing.T, sim *Sim, anchors *anchorsrc.Source, part *url.URL, block uint64) [32]byte {
	t.Helper()
	for i := 0; i < 50; i++ {
		root, err := anchors.AnchoredRoot(context.Background(), part, block)
		if err == nil {
			return root
		}
		require.True(t, errors.Is(err, anchorsrc.ErrNotAnchored), "%v", err)
		sim.Step()
	}
	t.Fatalf("the directory did not anchor %v block %d within 50 blocks", part, block)
	return [32]byte{}
}

// TestPullReachesTheAnchoredRoot is the shape of executor.md, "Sync", step 3.
//
// A node's database is restored to block R — here, built by pulling every
// account the partition had at R. The network then runs on to Q. The node
// pulls only the accounts whose leaf moved in (R, Q], verifies each against
// the root the Directory anchored for Q, and its own root then equals that
// root: the convergence test of step 4, which is what promotes it.
func TestPullReachesTheAnchoredRoot(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
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
	send := func() {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}

	ctx := context.Background()
	part := PartitionUrl("BVN0")
	src := api.Querier2{Querier: sim.S.Services()}

	// The tracker sees every root the Directory anchors, as the node collecting
	// blocks would hand it (#4292).
	machine := nodestate.New(part)
	local := emptyDb()
	trk, err := tracker.New(local, machine)
	require.NoError(t, err)
	anchors := anchorSourceFor(t, sim, part)
	// Every VERIFIED anchor is handed over; the tracker keeps the ones for
	// its own partition and drops the rest, because a block number without
	// its partition names nothing (#4205). An anchor that did not verify
	// never reaches it at all (#4301).
	anchors.OnAnchor = func(p *url.URL, block uint64, root [32]byte) {
		trk.Observe(p, block, root)
	}

	// --- The node's database as of R -----------------------------------
	send()
	send()

	batch := local.Begin(true)
	named, err := enumerate.Stale(ctx, src, part, batch, enumerate.Options{PageSize: 8})
	require.NoError(t, err)
	require.NotEmpty(t, named)
	total := len(named)
	t.Logf("the partition has %d accounts at R", total)

	held, r := pullEvery(t, src, batch, part, named)
	rootR := waitForAnchor(t, sim, anchors, part, r)
	for _, p := range held {
		require.NoError(t, p.Settle(rootR), "settle %v", p.Account)
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	require.Equal(t, rootR, bptRoot(t, local),
		"the pulled state does not hash to the root the directory anchored for block %d", r)
	t.Logf("restored to block R=%d, root %x", r, rootR)

	// --- The network runs on to Q --------------------------------------
	send()
	send()
	send()

	batch = local.Begin(true)
	stale, err := enumerate.Stale(ctx, src, part, batch, enumerate.Options{PageSize: 8})
	require.NoError(t, err)
	require.NotEmpty(t, stale, "nothing changed between R and Q; the test proves nothing")
	require.Less(t, len(stale), total,
		"the whole partition was re-pulled; only the accounts touched in (R, Q] should be")
	t.Logf("%d of %d accounts were touched in (R, Q]: %v", len(stale), total, stale)

	held, q := pullEvery(t, src, batch, part, stale)
	require.Greater(t, q, r)
	rootQ := waitForAnchor(t, sim, anchors, part, q)
	for _, p := range held {
		require.NoError(t, p.Settle(rootQ), "settle %v", p.Account)
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	require.Equal(t, rootQ, bptRoot(t, local),
		"after pulling only what changed, the local root is not the root anchored for block %d", q)
	t.Logf("reached block Q=%d, root %x", q, rootQ)

	// Step 4: the tracker says the local root equals an anchored root, so this
	// is block Q and the node can execute from Q+1.
	promoted, err := trk.Check(ctx)
	require.NoError(t, err)
	require.True(t, promoted, "the tracker did not recognise the anchored root")
	require.Equal(t, nodestate.StateActive, machine.State())
	require.Equal(t, q, machine.Get().SinceBlock)
	require.Equal(t, rootQ, machine.Get().VerifiedAnchor)
}

// TestPullSpineCarriesTheChains — the spine is pulled with its chain history,
// because that is what the node needs to verify the signatures on the anchors
// it verifies everything else against (executor.md, "Sync", step 3).
func TestPullSpineCarriesTheChains(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.StepN(10)

	ctx := context.Background()
	src := api.Querier2{Querier: sim.S.Services()}

	local := emptyDb()
	batch := local.Begin(true)
	spine := pull.SpineAccounts(DnUrl())
	require.Equal(t, pull.DnSpineAccounts(), spine)
	for _, u := range spine {
		require.NoError(t, pull.Account(ctx, src, batch, u, pull.Options{Mode: pull.ModeFullSpine}),
			"pull %v", u)
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())

	// Every spine account's chains must match the Directory's, entry for
	// entry: a head alone would let the node hold an anchor pool it cannot
	// read back.
	for _, u := range spine {
		var want, got []*api.ChainRecord
		rec, err := src.QueryAccountChains(ctx, u, &api.ChainQuery{})
		require.NoError(t, err, "chains of %v", u)
		want = rec.Records
		require.NotEmpty(t, want, "%v has no chains", u)

		View(t, local, func(b *database.Batch) {
			for _, w := range want {
				c, err := b.Account(u).ChainByName(w.Name)
				require.NoError(t, err)
				head, err := c.Head().Get()
				require.NoError(t, err)
				require.Equal(t, int64(w.Count), head.Count, "%v chain %s height", u, w.Name)
				got = append(got, w)
			}
		})
	}

	// And the pulled leaves are the Directory's leaves.
	View(t, sim.S.Database(Directory), func(peer *database.Batch) {
		View(t, local, func(mine *database.Batch) {
			for _, u := range spine {
				theirs, err := peer.Account(u).Hash()
				require.NoError(t, err)
				ours, err := mine.Account(u).Hash()
				require.NoError(t, err)
				require.Equal(t, theirs, ours, "leaf of %v", u)
			}
		})
	})
}

// anchorSourceFor is the anchor source the join builds, built the same way:
// the validator sets come from the partition's own store and the pool is the
// one that holds the anchors that partition PRODUCED (anchorsrc.PoolFor).
func anchorSourceFor(t *testing.T, sim *Sim, part *url.URL) *anchorsrc.Source {
	t.Helper()
	id, ok := ParsePartitionUrl(part)
	require.True(t, ok, "%v is not a partition", part)
	a, err := anchorsrc.FromStore(sim.Database(id), part)
	require.NoError(t, err)
	pool, err := anchorsrc.PoolFor(part, a.BvnNames())
	require.NoError(t, err)
	src, err := anchorsrc.New(sim.S.Services(), pool, part, a)
	require.NoError(t, err)
	return src
}
