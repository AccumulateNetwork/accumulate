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

// TestABackfillInterruptedMidChainResumesWhereItStopped — #4493 defect 1. On
// Docker a retry restarted a 2.3M-entry chain from entry 0, hours of pages
// discarded. The backfill runs from the head down and records how far it has
// got; asked again after a failure half-way it asks for no entry it already
// holds, and ends holding the chain whole.
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
	pages, twice := s.count()
	if twice != 0 {
		t.Fatalf("the retry asked again for %d entries the node already held (%d pages in all)", twice, pages)
	}
	if len(said) == 0 || said[len(said)-1] != 0 {
		t.Fatalf("the progress did not reach the first entry: %v", said)
	}
	for i := 1; i < len(said); i++ {
		if said[i] >= said[i-1] {
			t.Fatalf("the progress does not count down: %v", said)
		}
	}
	requireWhole(t, src, dst, u)
	if got := syncedOf(t, dst, u); got.Synced != n || len(got.Spans) != 0 {
		t.Fatalf("the record of a chain held whole is %+v", got)
	}
}

// TestARestartMidFillResumesFromTheRecord — #4493. The process ends half-way
// through a chain; the chain grows meanwhile and the node takes its new heads.
// The next process fills what is new above, steps over what the first process
// brought in -- the record says where -- and asks for no entry held.
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
	rec := syncedOf(t, dst, u)
	if len(rec.Spans) != 1 || rec.Spans[0].Top != n || rec.Spans[0].Low == 0 || rec.Spans[0].Low >= n {
		t.Fatalf("the record of a walk stopped half-way is %+v", rec)
	}
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}

	// The chain grows while the node is down, and the restarted node takes
	// the new heads.
	growChain(t, src, u, n, 700, 0x42)
	dst = open()
	defer dst.Close()
	takeHeads(t, src, dst, u)

	s.mu.Lock()
	s.before = nil
	s.mu.Unlock()
	if err := Backfill(context.Background(), []Source{s}, dst, u, 256, nil); err != nil {
		t.Fatal(err)
	}
	pages, twice := s.count()
	if twice != 0 {
		t.Fatalf("the restarted backfill asked again for %d entries the node already held (%d pages in all)", twice, pages)
	}
	requireWhole(t, src, dst, u)
	if got := syncedOf(t, dst, u); got.Synced != n+700 {
		t.Fatalf("the record of a chain held whole is %+v", got)
	}
}

// TestAResyncOfAChainHeldWholeStopsAtTheFirstEntryThatExists — #4493, Paul
// 2026-10-02: "When resyncing, one stops once the first entry exists." A node
// that held a chain whole -- by executing it, so with no record, and then by a
// backfill that wrote one -- restarts, takes the account's new heads, and
// asks for one page: the entries it does not hold. Everything older is held.
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

	s := newPagedSource(src)
	held := n
	for i, grow := range []int{100, 200} {
		growChain(t, src, u, held, grow, 0x43)
		takeHeads(t, src, dst, u)
		s.reset()
		if err := Backfill(context.Background(), []Source{s}, dst, u, 256, nil); err != nil {
			t.Fatal(err)
		}
		pages, _ := s.count()
		if pages != 1 {
			t.Fatalf("resync %d: %d pages asked for %d entries the node did not hold", i+1, pages, grow)
		}
		for idx := range s.asked {
			if idx < uint64(held) {
				t.Fatalf("resync %d: entry %d was asked for and the node held it", i+1, idx)
			}
		}
		requireWhole(t, src, dst, u)
		held += grow
	}
	if got := syncedOf(t, dst, u); got.Synced != uint64(held) {
		t.Fatalf("the record of a chain held whole is %+v", got)
	}
}

// TestAnUnfinishedWalkBelowExecutedEntriesIsStillFinished — #4493. A walk is
// interrupted; the node then executes blocks that append to the chain, and
// later restarts and takes newer heads. Above the unfinished walk now lie
// entries the node executed: they exist, but the chain is not whole below
// them, so the walk does not stop there -- it steps over them and the earlier
// walk's span and goes on to the entries it never had.
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
// another chain below the node's open set: the entries replayed do not
// reproduce the head, and the walk fails. What it wrote is not trusted: the
// next walk asks for every entry again -- and when that walk is interrupted
// in turn, the one after asks only for what the refetch had not reached.
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
	if !syncedOf(t, dst, u).Refetch {
		t.Fatal("the record does not say what is held is not trusted")
	}

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
	if len(s.asked) != n {
		t.Fatalf("the refetch asked for %d entries of %d", len(s.asked), n)
	}
	requireWhole(t, src, dst, u)
	if got := syncedOf(t, dst, u); got.Refetch || got.Synced != n {
		t.Fatalf("the record of a chain held whole is %+v", got)
	}
}
