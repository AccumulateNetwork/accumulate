// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// syncedOf reads the backfill's record of u's main chain.
func syncedOf(t testing.TB, db *database.Database, u *url.URL) *database.SyncedEntries {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	v, err := b.Account(u).SyncedEntries("main").Get()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// requireGaps fails unless the record of u's main chain names exactly want
// missing, as {low, high, messages only}.
func requireGaps(t testing.TB, db *database.Database, u *url.URL, want ...database.SyncedGap) {
	t.Helper()
	got := syncedOf(t, db, u).Gaps
	ok := len(got) == len(want)
	for i := 0; ok && i < len(got); i++ {
		ok = got[i].Equal(&want[i])
	}
	if !ok {
		var s []database.SyncedGap
		for _, g := range got {
			s = append(s, *g)
		}
		t.Fatalf("the record names %+v missing, want %+v", s, want)
	}
}

// askedTwiceOutside fails if an entry outside [low, high) was asked for more
// than once: within it lies the subtree a walk was filling when it stopped,
// whose pages were held unproven and are asked for again.
func askedTwiceOutside(t testing.TB, s *pagedSource, low, high uint64) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for idx, n := range s.asked {
		if n > 1 && (idx < low || idx >= high) {
			t.Fatalf("entry %d, outside the unproven subtree [%d, %d), was asked for %d times", idx, low, high, n)
		}
	}
}

// TestABackfillInterruptedMidChainResumesWhereItStopped — #4493 defect 1. On
// Docker a retry restarted a 2.3M-entry chain from entry 0, hours of pages
// discarded. The backfill runs from the head down, a subtree of the head at a
// time, and closes each subtree's gap once it is proven; asked again after a
// failure half-way it asks again only for the subtree it was filling, and
// ends holding the chain whole.
//
// 5000 entries are the subtrees [0,4096), [4096,4608), [4608,4864) and the
// open set from 4864: the walk is interrupted inside [0,4096).
func TestABackfillInterruptedMidChainResumesWhereItStopped(t *testing.T) {
	const n = 5000 // 20 pages of 256
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	buildChain(t, src, u, n, 0x41)
	dst := newObservedDB(t)
	takeHeads(t, src, dst, u)

	s := newPagedSource(src)
	s.before = func(page int, _, _ uint64) error {
		if page == 9 {
			return errInterrupted
		}
		return nil
	}
	if err := Backfill(context.Background(), []Source{s}, dst, u, 256, nil); err == nil {
		t.Fatal("the interruption did not interrupt")
	}
	requireGaps(t, dst, u, database.SyncedGap{Low: 0, High: 4096})
	s.mu.Lock()
	s.before = nil
	s.mu.Unlock()
	// The progress the caller is told counts down from the head.
	var said []uint64
	progress := func(_ string, at, total uint64) {
		if total != n {
			t.Errorf("told a height of %d", total)
		}
		said = append(said, at)
	}
	if err := Backfill(context.Background(), []Source{s}, dst, u, 256, progress); err != nil {
		t.Fatal(err)
	}
	askedTwiceOutside(t, s, 0, 4096)
	if len(said) == 0 || said[len(said)-1] != 0 {
		t.Fatalf("the progress did not reach the first entry: %v", said)
	}
	for i := 1; i < len(said); i++ {
		if said[i] >= said[i-1] {
			t.Fatalf("the progress does not count down: %v", said)
		}
	}
	requireWhole(t, src, dst, u)
	requireGaps(t, dst, u)
}

// TestARestartMidFillResumesFromTheRecord — #4493. The process ends half-way
// through a chain; the chain grows meanwhile and the node takes its new heads.
// The next process fills what is new above, asks for nothing the first process
// proved -- the record says what is missing -- and ends holding it whole.
func TestARestartMidFillResumesFromTheRecord(t *testing.T) {
	const n = 5000
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	buildChain(t, src, u, n, 0x42)

	dir := t.TempDir()
	open := func() *database.Database {
		db, err := database.OpenLevelDB(dir, nil)
		if err != nil {
			t.Fatal(err)
		}
		return db
	}
	dst := open()
	takeHeads(t, src, dst, u)

	s := newPagedSource(src)
	s.before = func(page int, _, _ uint64) error {
		if page == 11 {
			return errInterrupted
		}
		return nil
	}
	if err := Backfill(context.Background(), []Source{s}, dst, u, 256, nil); err == nil {
		t.Fatal("the interruption did not interrupt")
	}
	requireGaps(t, dst, u, database.SyncedGap{Low: 0, High: 4096})
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}

	// The chain grows while the node is down, and the restarted node takes
	// the new heads.
	growChain(t, src, u, n, 700, 0x42)
	dst = open()
	defer dst.Close()
	takeHeads(t, src, dst, u)
	// 5700 entries, the open set from 5632: what lies between the old head
	// and the new open set is missing whole, the open set's messages, and
	// what the first process never proved.
	requireGaps(t, dst, u,
		database.SyncedGap{Low: 5632, High: 5700, MessagesOnly: true},
		database.SyncedGap{Low: 5000, High: 5632},
		database.SyncedGap{Low: 0, High: 4096})

	s.mu.Lock()
	s.before = nil
	s.mu.Unlock()
	if err := Backfill(context.Background(), []Source{s}, dst, u, 256, nil); err != nil {
		t.Fatal(err)
	}
	// Every entry from 4096 to the old head was proven or taken by the
	// first process, and asked for at most once, by it.
	askedTwiceOutside(t, s, 0, 4096)
	requireWhole(t, src, dst, u)
	requireGaps(t, dst, u)
}

// TestAResyncOfAChainHeldWholeStopsAtTheFirstEntryThatExists — #4493, Paul
// 2026-10-02: "When resyncing, one stops once the first entry exists." A node
// that held a chain whole -- taken whole, so with no gap -- restarts, takes the
// account's new heads, and asks only for what the take left missing: the
// entries between its old head and the new open set, and the open set's
// messages. Everything older is held.
func TestAResyncOfAChainHeldWholeStopsAtTheFirstEntryThatExists(t *testing.T) {
	const n = 3000
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	buildChain(t, src, u, n, 0x43)

	// Held whole, entry by entry, as a node that executed it holds it.
	dst := newObservedDB(t)
	b := dst.Begin(true)
	if err := Account(context.Background(), &dbSource{db: src}, b, u, Options{Mode: ModeFullSpine}); err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}

	requireGaps(t, dst, u)
	s := newPagedSource(src)
	held := n
	// 3100: entries [3000,3072) and the open set's messages, one page each.
	// 3300: the open set from 3072 is taken with its entries; the messages
	// from the old head up are missing, one page.
	wantPages := []int{2, 1}
	for i, grow := range []int{100, 200} {
		growChain(t, src, u, held, grow, 0x43)
		takeHeads(t, src, dst, u)
		s.reset()
		if err := Backfill(context.Background(), []Source{s}, dst, u, 256, nil); err != nil {
			t.Fatal(err)
		}
		pages, twice := s.count()
		if pages != wantPages[i] || twice != 0 {
			t.Fatalf("resync %d: %d pages (%d entries twice) asked for %d entries the node did not hold", i+1, pages, twice, grow)
		}
		for idx := range s.asked {
			if idx < uint64(held) {
				t.Fatalf("resync %d: entry %d was asked for and the node held it", i+1, idx)
			}
		}
		requireWhole(t, src, dst, u)
		held += grow
	}
	requireGaps(t, dst, u)
}

// TestAnUnfinishedWalkBelowExecutedEntriesIsStillFinished — #4493. A walk is
// interrupted; the node then executes blocks that append to the chain, and
// later restarts and takes newer heads. Above the unfinished walk now lie
// entries the node executed: the record keeps the gap below them, and the
// walk asks for it and for what the new take left missing, and not for what
// the node executed.
func TestAnUnfinishedWalkBelowExecutedEntriesIsStillFinished(t *testing.T) {
	const n = 3000
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	buildChain(t, src, u, n, 0x44)
	dst := newObservedDB(t)
	takeHeads(t, src, dst, u)

	s := newPagedSource(src)
	s.before = func(page int, _, _ uint64) error {
		if page == 4 {
			return errInterrupted
		}
		return nil
	}
	if err := Backfill(context.Background(), []Source{s}, dst, u, 256, nil); err == nil {
		t.Fatal("the interruption did not interrupt")
	}
	// 3000 entries: the open set's messages and [2560,2816) were brought in
	// and proven; the walk stopped inside [2048,2560).
	requireGaps(t, dst, u, database.SyncedGap{Low: 0, High: 2560})

	// The node executes 300 entries as its peers do.
	growChain(t, src, u, n, 300, 0x44)
	growChain(t, dst, u, n, 300, 0x44)

	// It restarts later and takes the heads of a chain that has grown again.
	growChain(t, src, u, n+300, 400, 0x44)
	takeHeads(t, src, dst, u)

	s.reset()
	if err := Backfill(context.Background(), []Source{s}, dst, u, 256, nil); err != nil {
		t.Fatal(err)
	}
	if _, twice := s.count(); twice != 0 {
		t.Fatalf("%d entries asked for twice", twice)
	}
	for idx := range s.asked {
		if idx >= n && idx < n+300 {
			t.Fatalf("entry %d, which the node executed, was asked for", idx)
		}
	}
	requireWhole(t, src, dst, u)
	requireGaps(t, dst, u)
}

// mixedSource serves another chain's entries below below and the node's own
// from there up: a peer whose open set is right and whose history is not.
type mixedSource struct {
	*pagedSource
	other *dbSource
	below uint64
}

func (s *mixedSource) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	good, err := s.pagedSource.QueryChainEntries(ctx, u, q)
	if err != nil {
		return nil, err
	}
	bad, err := s.other.QueryChainEntries(ctx, u, q)
	if err != nil {
		return nil, err
	}
	for i, r := range good.Records {
		if r.Index < s.below {
			good.Records[i] = bad.Records[i]
		}
	}
	return good, nil
}

// TestEntriesThatDoNotReproduceTheHeadAreAskedForAgain — #4493. A peer serves
// another chain below the node's open set: the first subtree it fills does
// not hash to the head's root, and the walk fails. What it wrote is not
// trusted -- the gap stays -- and the next walk asks for every entry below the
// open set again; when that walk is interrupted in turn, the one after asks
// only for what the refetch had not proven.
func TestEntriesThatDoNotReproduceTheHeadAreAskedForAgain(t *testing.T) {
	const n = 2000
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	buildChain(t, src, u, n, 0x48)
	other := newObservedDB(t)
	buildChain(t, other, u, n, 0x49)
	dst := newObservedDB(t)
	takeHeads(t, src, dst, u)

	liar := &mixedSource{pagedSource: newPagedSource(src), other: &dbSource{db: other}, below: n &^ 255}
	err := Backfill(context.Background(), []Source{liar}, dst, u, 256, nil)
	if err == nil {
		t.Fatal("entries that do not reproduce the head were kept")
	}
	requireGaps(t, dst, u, database.SyncedGap{Low: 0, High: n &^ 255})

	s := newPagedSource(src)
	s.before = func(page int, _, _ uint64) error {
		if page == 4 {
			return errInterrupted
		}
		return nil
	}
	if err := Backfill(context.Background(), []Source{s}, dst, u, 256, nil); err == nil {
		t.Fatal("the interruption did not interrupt")
	}
	s.mu.Lock()
	s.before = nil
	s.mu.Unlock()
	if err := Backfill(context.Background(), []Source{s}, dst, u, 256, nil); err != nil {
		t.Fatal(err)
	}
	if _, twice := s.count(); twice != 0 {
		t.Fatalf("the refetch asked for %d entries twice", twice)
	}
	if len(s.asked) != n&^255 {
		t.Fatalf("the refetch asked for %d entries of the %d below the open set", len(s.asked), n&^255)
	}
	requireWhole(t, src, dst, u)
	requireGaps(t, dst, u)
}
