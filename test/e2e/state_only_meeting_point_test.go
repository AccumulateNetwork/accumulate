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
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/enumerate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
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

// peerServingPart is peerServing for a partition other than the Directory.
func peerServingPart(db database.Viewer, part string) pull.Source {
	return api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{
		Database:  db,
		Partition: part,
	})}
}

// pullStateOnlyFrom is what join.fetch does for the long tail, against one
// source: ModeStateOnly, one account at a time, into one batch.
func pullStateOnlyFrom(t *testing.T, src pull.Source, into *database.Database, part *url.URL, accounts []*url.URL) error {
	t.Helper()
	batch := into.Begin(true)
	defer batch.Discard()
	for _, u := range accounts {
		p, err := pull.Fetch(context.Background(), src, batch, u,
			pull.Options{Mode: pull.ModeStateOnly, Partition: part}, false)
		if err != nil {
			return err
		}
		if err := p.Keep(); err != nil {
			return err
		}
	}
	if err := batch.UpdateBPT(); err != nil {
		return err
	}
	return batch.Commit()
}

// chainHeights reads the height of every chain of every named account.
func chainHeights(t *testing.T, db *database.Database, accounts []*url.URL) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	View(t, db, func(b *database.Batch) {
		for _, u := range accounts {
			chains, err := b.Account(u).Chains().Get()
			require.NoError(t, err)
			for _, cm := range chains {
				c, err := b.Account(u).ChainByName(cm.Name)
				require.NoError(t, err)
				head, err := c.Head().Get()
				require.NoError(t, err)
				out[u.String()+"#"+cm.Name] = head.Count
			}
		}
	})
	return out
}

// leafOf is the account's BPT leaf — what a peer's receipt proves and what
// Verify compares the pulled state against.
func leafOf(t *testing.T, db *database.Database, u *url.URL) [32]byte {
	t.Helper()
	var h [32]byte
	View(t, db, func(b *database.Batch) {
		x, err := b.Account(u).Hash()
		require.NoError(t, err)
		h = x
	})
	return h
}

func dnLedgerIndex(t *testing.T, db *database.Database) uint64 {
	t.Helper()
	var n uint64
	View(t, db, func(b *database.Batch) {
		var l *SystemLedger
		require.NoError(t, b.Account(DnUrl().JoinPath(Ledger)).Main().GetAs(&l))
		n = l.Index
	})
	return n
}

// TestPullStateOnly_MeetsAPeerThatIsBehind is the long tail's half of the
// meeting point, through the production querier.
//
// ModeStateOnly is what join.fetch pulls every account with, and the long tail
// includes the spine: ChangedAccounts adds <partition>/ledger unconditionally,
// and when the node is ahead, enumerate.Stale names every account whose leaf
// differs from the peer's IN EITHER DIRECTION — so the ahead case is exactly
// the case the long tail runs every round.
//
// The peer served the chain heads and they were restored unconditionally, so a
// node that had executed further had its chains SHORTENED to the peer's and
// its ledger body rewound to the peer's block — the account the node reads its
// own executed height from (#4344).
func TestPullStateOnly_MeetsAPeerThatIsBehind(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	// A peer that stopped early.
	sim.StepN(10)
	behind := emptyDb()
	require.NoError(t, pullSpineFrom(t, peerServing(sim.S.Database(Directory)), behind))

	// This node ran on. This is the state a restart comes back with.
	sim.StepN(25)
	local := emptyDb()
	require.NoError(t, pullSpineFrom(t, peerServing(sim.S.Database(Directory)), local))

	spine := pull.SpineAccounts(DnUrl())
	before := chainHeights(t, local, spine)
	beforeLeaf := leafOf(t, local, DnUrl().JoinPath(Ledger))
	mine, theirs := dnLedgerIndex(t, local), dnLedgerIndex(t, behind)
	require.Greater(t, mine, theirs, "the peer is not behind; the test proves nothing")
	t.Logf("node at DN block %d, peer at block %d", mine, theirs)

	require.NoError(t, pullStateOnlyFrom(t, peerServing(behind), local, DnUrl(), spine),
		"a peer that is simply behind was refused")

	after := chainHeights(t, local, spine)
	var shortened int
	for k, v := range before {
		if after[k] < v {
			shortened++
			t.Logf("SHORTENED %s: %d -> %d", k, v, after[k])
		}
	}
	require.Zero(t, shortened, "%d of %d chains were shortened to the peer's height", shortened, len(before))
	require.Equal(t, mine, dnLedgerIndex(t, local),
		"the long tail rewound the node's own ledger to the peer's block")
	require.Equal(t, beforeLeaf, leafOf(t, local, DnUrl().JoinPath(Ledger)),
		"the node's ledger leaf moved")
}

// TestPullStateOnly_VerificationCannotCatchTheRewind — the rewind is invisible
// to Verify. Verify asks whether the state in the batch hashes to the leaf the
// peer's receipt proves; a node rewound to the peer's state hashes to exactly
// that leaf, so it verifies and commits.
//
// That is why this is fixed at the meeting point and not at the verifier: a
// pull that takes the peer's state wholesale is, by construction, the state
// the peer can prove.
func TestPullStateOnly_VerificationCannotCatchTheRewind(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	sim.StepN(10)
	behind := emptyDb()
	require.NoError(t, pullSpineFrom(t, peerServing(sim.S.Database(Directory)), behind))
	sim.StepN(25)
	local := emptyDb()
	require.NoError(t, pullSpineFrom(t, peerServing(sim.S.Database(Directory)), local))

	ledger := DnUrl().JoinPath(Ledger)
	require.Greater(t, dnLedgerIndex(t, local), dnLedgerIndex(t, behind))
	require.NoError(t, pullStateOnlyFrom(t, peerServing(behind), local, DnUrl(), []*url.URL{ledger}))

	// The node's leaf must not be the behind peer's. If it is, the node has
	// taken the peer's state whole and nothing downstream can tell.
	require.NotEqual(t, leafOf(t, behind, ledger), leafOf(t, local, ledger),
		"the node's ledger leaf IS the behind peer's: it took the peer's state whole, "+
			"which is precisely the state the peer's receipt proves, so Verify passes and commits")
}

// TestPullMeetingPoint_KeepsTheAccountsOwnDirectory — the meeting point is the
// ACCOUNT's, and an account's leaf is its body, its directory list, its chains
// and its pending list (observer_prod.go, hashState). Keeping the body while
// taking the peer's directory builds a leaf that is neither side's — the
// hybrid the pull exists to avoid.
//
// For a key book that is not abstract: the book's directory names its pages,
// so a book pulled from a peer that is one page behind is a book that does not
// name one of its own pages.
func TestPullMeetingPoint_KeepsTheAccountsOwnDirectory(t *testing.T) {
	alice := url.MustParse("alice")
	aliceKey := acctesting.GenerateKey(alice)
	book := alice.JoinPath("book")

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)

	var ts uint64
	addPage := func() {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(book).
				CreateKeyPage().WithEntry().Key(acctesting.GenerateKey(alice, ts)[32:], SignatureTypeED25519).FinishEntry().
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds())
		sim.StepN(2)
	}

	src := func() pull.Source { return peerServingPart(sim.DatabaseFor(alice), "BVN0") }
	pullBook := func(from pull.Source, into *database.Database) error {
		batch := into.Begin(true)
		defer batch.Discard()
		p, err := pull.Fetch(context.Background(), from, batch, book,
			pull.Options{Mode: pull.ModeFullSpine, Partition: PartitionUrl("BVN0")}, false)
		if err != nil {
			return err
		}
		if err := p.Keep(); err != nil {
			return err
		}
		if err := batch.UpdateBPT(); err != nil {
			return err
		}
		return batch.Commit()
	}
	directoryOf := func(db *database.Database) []string {
		var out []string
		View(t, db, func(b *database.Batch) {
			dir, err := b.Account(book).Directory().Get()
			require.NoError(t, err)
			for _, u := range dir {
				out = append(out, u.String())
			}
		})
		return out
	}

	// The peer stopped with two pages.
	addPage()
	behind := emptyDb()
	require.NoError(t, pullBook(src(), behind))

	// The node ran on and made a third.
	addPage()
	local := emptyDb()
	require.NoError(t, pullBook(src(), local))

	theirs, mine := directoryOf(behind), directoryOf(local)
	require.Less(t, len(theirs), len(mine), "the peer is not behind on the directory; the test proves nothing")
	beforeLeaf := leafOf(t, local, book)
	t.Logf("node's book names %v, the peer's names %v", mine, theirs)

	// Meeting a peer that is behind: everything it can give, the node has.
	require.NoError(t, pullBook(peerServingPart(behind, "BVN0"), local))

	require.Equal(t, mine, directoryOf(local),
		"the node kept its own body and took the peer's directory: a book that no longer names one of its own pages")
	require.Equal(t, beforeLeaf, leafOf(t, local, book),
		"the account's leaf moved to one that is neither the node's nor the peer's")
}

// behindSources is the join's Sources with one peer: a store that is behind
// this node. The reads that stand on their own — the block ledger, the BPT
// pages — go to that peer too, because the point is a round in which every
// answer the node gets comes from a peer that is behind it. Only the
// Directory's anchor chain is read from the live network, which is where a
// restarting node reads it from.
type behindSources struct {
	part  *url.URL
	peer  pull.Source
	peerQ api.Querier
	dn    api.Querier
}

func (s *behindSources) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return []pull.Source{s.peer}, s.part, nil
}

func (s *behindSources) Querier(u *url.URL) api.Querier {
	if u != nil && DnUrl().Equal(u) {
		return s.dn
	}
	return s.peerQ
}

// copyOf is a node's database as of the block the network is at: every account
// of the partition, pulled and verified against the root the Directory
// anchored, so its BPT root IS that block's root. Both the peer and the node
// below are built this way — a store a real node could have, rather than one
// assembled by hand.
func copyOf(t *testing.T, sim *Sim, anchors *anchorsrc.Source, part *url.URL) (*database.Database, uint64, [32]byte) {
	t.Helper()
	ctx := context.Background()
	src := api.Querier2{Querier: sim.S.Services()}
	db := emptyDb()

	batch := db.Begin(true)
	named, err := enumerate.Stale(ctx, src, part, batch, enumerate.Options{PageSize: 8})
	require.NoError(t, err)
	require.NotEmpty(t, named)
	held, block := pullEvery(t, src, batch, part, named)
	root := waitForAnchor(t, sim, anchors, part, block)
	for _, p := range held {
		require.NoError(t, p.Settle(root), "settle %v", p.Account)
	}
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	require.Equal(t, root, bptRoot(t, db),
		"the copy does not hash to the root the directory anchored for block %d", block)
	return db, block, root
}

func partLedgerIndex(t *testing.T, db *database.Database, part *url.URL) uint64 {
	t.Helper()
	var n uint64
	View(t, db, func(b *database.Batch) {
		var l *SystemLedger
		require.NoError(t, b.Account(part.JoinPath(Ledger)).Main().GetAs(&l))
		n = l.Index
	})
	return n
}

// TestJoinRound_ANodeAheadOfItsPeerKeepsItsOwnState is the whole of #4348 at
// the level the daemon runs it: join.PulledState.Pull, round after round,
// against a peer that is behind this node.
//
// It is the restart. A node comes back holding the state it executed to; the
// peer it is given is at an earlier block. changedAccounts returns nothing —
// the peer's block is not past the node's, so the ledger walk has nothing to
// say (state.go: "if peer <= r") — so the BPT page diff decides the round, and
// enumerate.Stale names every account whose leaf differs from the peer's IN
// EITHER DIRECTION, which for a node that is ahead is every account it is
// ahead on. Every round. The long tail then pulled each of them in
// ModeStateOnly and rewound the node to the peer's block.
//
// Nothing downstream could catch it: the rewound account hashes to exactly the
// leaf the peer's receipt proves, so it verified and committed.
func TestJoinRound_ANodeAheadOfItsPeerKeepsItsOwnState(t *testing.T) {
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

	ctx := context.Background()
	part := PartitionUrl("BVN0")
	anchors := anchorSourceFor(t, sim, part)

	var ts uint64
	send := func() {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}

	// The peer: a node that stopped at R.
	send()
	send()
	behind, r, _ := copyOf(t, sim, anchors, part)

	// This node: it ran on to Q and came back holding that.
	send()
	send()
	send()
	local, q, rootQ := copyOf(t, sim, anchors, part)
	require.Greater(t, q, r, "the peer is not behind; the test proves nothing")
	t.Logf("node at %v block %d, peer at block %d", part, q, r)

	accounts := []*url.URL{
		part.JoinPath(Ledger), part.JoinPath(AnchorPool), part.JoinPath(Synthetic),
		alice.JoinPath("tokens"), bob.JoinPath("tokens"),
	}
	before := chainHeights(t, local, accounts)
	require.NotEmpty(t, before)
	require.Equal(t, q, partLedgerIndex(t, local, part))

	// The join, driven as the daemon drives it, against that peer.
	state, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  local,
		Sources: &behindSources{
			part:  part,
			peer:  peerServingPart(behind, "BVN0"),
			peerQ: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: behind, Partition: "BVN0"}),
			dn:    sim.S.Services(),
		},
	})
	require.NoError(t, err)
	for round := 0; round < 4; round++ {
		require.NoError(t, state.Pull(ctx), "pull round %d", round)
	}

	require.Equal(t, q, partLedgerIndex(t, local, part),
		"the join rewound the node's own ledger to the peer's block %d", r)
	require.Equal(t, before, chainHeights(t, local, accounts),
		"the join moved the node's own chains")
	require.Equal(t, rootQ, bptRoot(t, local),
		"the join moved the node off the root the directory anchored for its own block %d", q)
}
