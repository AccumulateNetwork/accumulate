// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	bptkey "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func peerStore(t *testing.T) *database.Database {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// peerSet is several scripted peers of one partition, each reached by name.
type peerSet struct {
	noValidators
	peers []*scriptedPeer
}

func (s *peerSet) For(ctx context.Context, u *url.URL) ([]pull.Source, *url.URL, error) {
	var out []pull.Source
	for _, p := range s.peers {
		srcs, _, _ := p.For(ctx, u)
		out = append(out, srcs...)
	}
	return out, s.peers[0].partition, nil
}

func (s *peerSet) Querier(u *url.URL) api.Querier { return s.peers[len(s.peers)-1].Querier(u) }

// TestOnePeersBlockIsNotTheTarget — #4438 threat F3. The block the pull starts
// at, and each round's target, is the lowest any of the partition's peers
// names: one peer naming a block far ahead would otherwise set S there, so no
// record is ever read and the state is never ready.
func TestOnePeersBlockIsNotTheTarget(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")
	store := peerStore(t)
	putTokens(t, store, alice, 5)
	leaf := leafIn(t, store, alice)

	liar := &scriptedPeer{partition: here, block: 1 << 40, records: map[uint64][]*url.URL{}, state: store, page: []*api.BptLeafSummary{&leaf}}
	honest := &scriptedPeer{partition: here, block: 10, records: map[uint64][]*url.URL{}, state: store, page: []*api.BptLeafSummary{&leaf}}
	set := &peerSet{peers: []*scriptedPeer{liar, honest}}
	s, _ := joiningAt(t, honest)
	s.sources = set

	require.NoError(t, s.Pull(context.Background()))
	require.Equal(t, uint64(10), s.sync.start, "one peer's block set the start of the pull")

	honest.block = 12
	honest.records[12] = []*url.URL{alice}
	require.NoError(t, s.Pull(context.Background()))
	require.Equal(t, uint64(12), s.sync.last, "the records did not follow the partition")
}

// TestTheSourcesAreAskedInRotation — #4438 threat F6. A peer that serves a
// wrong body is not the peer asked first every time, so the next pull of the
// account takes it from another.
func TestTheSourcesAreAskedInRotation(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")
	bent, good := peerStore(t), peerStore(t)
	putTokens(t, bent, alice, 12)
	putTokens(t, good, alice, 5)

	peers := []*scriptedPeer{
		{partition: here, block: 10, state: bent},
		{partition: here, block: 10, state: good},
	}
	s, db := joiningAt(t, peers[1])
	s.sources = &peerSet{peers: peers}
	p := newSyncing()

	ctx := context.Background()
	s.pullOne(ctx, p, alice)
	s.pullOne(ctx, p, alice)
	require.Equal(t, int64(5), balanceIn(t, db, alice), "the same peer was asked first both times")
}

// TestRecordsAreReadFromTheOldestPageBlock — #4438 (Paul, 2026-09-25; review
// F1's benign cause). The pull starts at the peers' block S = 10. alice
// changed at block 10, 5 to 7, and never again. The walk's page is read from a
// peer that stood at 9: it shows alice at 5, which the restarted node holds
// too, so the walk skips her, and no record after S names her. The records
// are read from the block the page was served at, and block 10's names her.
func TestRecordsAreReadFromTheOldestPageBlock(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")
	before, after := peerStore(t), peerStore(t)
	putTokens(t, before, alice, 5)
	putTokens(t, after, alice, 7)
	leaf := leafIn(t, before, alice)

	peer := &scriptedPeer{partition: here, block: 10, records: map[uint64][]*url.URL{10: {alice}}, state: after,
		page: []*api.BptLeafSummary{&leaf}}
	peer.ledger = []uint64{10, 9} // S, then the page's peer
	s, db := joiningAt(t, peer)
	putTokens(t, db, alice, 5) // the restarted node's own

	require.NoError(t, s.Pull(context.Background()))
	require.True(t, s.sync.walked, "precondition: the walk covered the tree")
	require.Equal(t, uint64(9), s.sync.low, "the records were not read from the page's block")
	require.Equal(t, int64(7), balanceIn(t, db, alice),
		"alice changed at block 10 and the page was of block 9: the walk skipped her and no record read named her")
}

// TestARepairThatDoesNotMatchWalksAgain — #4438 review F1, Paul: "If the pull
// of accounts doesn't create a matching BPT root, then the joining node has to
// pull the accounts again." bob is quiet: no block after S names him. The
// walk takes him from a peer that serves him wrong. A repair from the block
// ledger never names him; the repair after one that brought no match walks the
// tree again, finds his leaf differs from the peers', and takes him whole.
func TestARepairThatDoesNotMatchWalksAgain(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	bob := protocol.AccountUrl("bob", "tokens")
	lie, truth := peerStore(t), peerStore(t)
	putTokens(t, lie, bob, 12)
	putTokens(t, truth, bob, 5)
	leaf := leafIn(t, truth, bob)

	peer := &scriptedPeer{partition: here, block: 10, records: map[uint64][]*url.URL{}, state: lie,
		page: []*api.BptLeafSummary{&leaf}}
	s, db := joiningAt(t, peer)
	ctx := context.Background()
	require.NoError(t, s.Pull(ctx))
	require.Equal(t, int64(12), balanceIn(t, db, bob), "precondition: the walk took bob wrong")

	// Every peer is honest from here.
	peer.state = truth
	for i := 0; i < 2; i++ {
		s.RepairFrom(10)
		for r := 0; r < 3; r++ {
			require.NoError(t, s.Pull(ctx))
		}
	}
	require.Equal(t, int64(5), balanceIn(t, db, bob), "two repairs brought no match and nothing pulled bob again")
}

// TestTheWalkDropsLeavesNoPeerHolds — Paul: the node "must also drop leaves it
// holds that no peer holds". Once the walk has covered the tree, a leaf the
// node holds that no page named and no record brought current is deleted.
func TestTheWalkDropsLeavesNoPeerHolds(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")
	ghost := protocol.AccountUrl("ghost", "tokens")
	store := peerStore(t)
	putTokens(t, store, alice, 5)
	leaf := leafIn(t, store, alice)

	peer := &scriptedPeer{partition: here, block: 10, records: map[uint64][]*url.URL{}, state: store,
		page: []*api.BptLeafSummary{&leaf}}
	s, db := joiningAt(t, peer)
	putTokens(t, db, ghost, 3)

	require.NoError(t, s.Pull(context.Background()))
	require.True(t, s.sync.walked, "precondition: the walk covered the tree")
	View(db, func(batch *database.Batch) {
		_, err := batch.BPT().Get(bptkey.NewKey("Account", ghost))
		require.ErrorIs(t, err, errors.NotFound, "the node still holds a leaf no peer holds")
	})
}

// TestReadyIsNotBehindTheRecords: the pulled ledger names the block the node
// hands off at, and a ledger pulled from a peer behind the records read is not
// a block the state is at: the records have brought accounts past it.
func TestReadyIsNotBehindTheRecords(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	peer := &scriptedPeer{partition: here, block: 12, records: map[uint64][]*url.URL{}, state: peerStore(t)}
	s, db := joiningAt(t, peer)
	s.sync = newSyncing()
	s.sync.last, s.sync.ready = 12, true
	putLedger(t, db, here, 10)
	_, ok := s.Ready()
	require.False(t, ok, "the node was ready at block 10 with the records read through 12")
	putLedger(t, db, here, 12)
	n, ok := s.Ready()
	require.True(t, ok)
	require.Equal(t, uint64(12), n)
}

// View is a read of db.
func View(db *database.Database, fn func(*database.Batch)) {
	batch := db.Begin(false)
	defer batch.Discard()
	fn(batch)
}

// TestAPeerWhoseAnswersBroughtNoMatchIsAskedLast — executor spec, "Sync",
// "Two mismatches", 1: "a source whose answers were part of a re-pull that did
// not bring the match is moved to the back of the order". Peer a serves bob
// wrong and b serves him right. A pull a answered is handed off from and its
// root does not match; from then on b is asked first, every time, not in turn.
func TestAPeerWhoseAnswersBroughtNoMatchIsAskedLast(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	bob := protocol.AccountUrl("bob", "tokens")
	lie, truth := peerStore(t), peerStore(t)
	putTokens(t, lie, bob, 12)
	putTokens(t, truth, bob, 5)

	a := &scriptedPeer{name: "a", partition: here, block: 10, state: lie}
	b := &scriptedPeer{name: "b", partition: here, block: 10, state: truth}
	s, db := joiningAt(t, b)
	s.sources = &peerSet{peers: []*scriptedPeer{a, b}}
	ctx := context.Background()

	// The pull handed off from took bob from a, and the root after it did
	// not match.
	s.sync = newSyncing()
	s.pullOne(ctx, s.sync, bob)
	require.Equal(t, int64(12), balanceIn(t, db, bob), "precondition: a answered first")
	s.HandedOff(10)
	s.RepairFrom(10)

	s.sync = newSyncing()
	for i := 0; i < 4; i++ {
		s.pullOne(ctx, s.sync, bob)
		require.Equal(t, int64(5), balanceIn(t, db, bob), "pull %d asked the peer whose answers brought no match first", i)
	}
}

// TestALeafAPageOmitsIsNotDeletedOnThePagesWord — executor spec, "Sync", "Two
// mismatches", 1: "It deletes nothing on this comparison". A page is a peer's
// word: one that omits carol, whom the peers hold, must not cost the node her.
// She is asked for, and taken.
func TestALeafAPageOmitsIsNotDeletedOnThePagesWord(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")
	carol := protocol.AccountUrl("carol", "tokens")
	store := peerStore(t)
	putTokens(t, store, alice, 5)
	putTokens(t, store, carol, 9)
	leaf := leafIn(t, store, alice)

	peer := &scriptedPeer{partition: here, block: 10, records: map[uint64][]*url.URL{}, state: store,
		page: []*api.BptLeafSummary{&leaf}} // carol omitted
	s, db := joiningAt(t, peer)
	putTokens(t, db, carol, 9)

	require.NoError(t, s.Pull(context.Background()))
	require.True(t, s.sync.walked, "precondition: the walk covered the tree")
	require.Equal(t, int64(9), balanceIn(t, db, carol), "a page's omission deleted an account the peers hold")
}
