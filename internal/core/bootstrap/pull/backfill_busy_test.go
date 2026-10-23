// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// quickRetry makes the backfill's back-off short for a test.
func quickRetry(t *testing.T) {
	was := backfillRetry
	backfillRetry.first, backfillRetry.most = time.Millisecond, 4*time.Millisecond
	t.Cleanup(func() { backfillRetry = was })
}

// capacityExhausted is a validator's capacity refusal as the Docker
// follower saw it (#4493): NotReady, wrapped as the NotFound of the entry
// whose message could not be fetched.
func capacityExhausted(index uint64) error {
	return errors.NotFound.WithFormat("entry %d: the peer does not serve it: %w", index,
		errors.NotReady.With("query capacity exhausted, retry later"))
}

// TestACapacityRefusalRetriesThePage — #4493 defect 2. Three followers
// pulling at once exhausted a validator's query capacity; its NotReady ended
// the account's attempt, and the attempt restarted. A capacity refusal is not
// a refusal of the entries: the page is asked again, after a pause, and the
// chain completes in one attempt, asking for no entry twice.
func TestACapacityRefusalRetriesThePage(t *testing.T) {
	quickRetry(t)
	const n = 3000
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	buildChain(t, src, u, n, 0x45)
	dst := newObservedDB(t)
	takeHeads(t, src, dst, u)

	s := newPagedSource(src)
	refused := 0
	s.before = func(page int, start, _ uint64) error {
		if page%3 == 0 {
			refused++
			return capacityExhausted(start)
		}
		return nil
	}
	if err := Backfill(context.Background(), []Source{s}, dst, u, 256, nil); err != nil {
		t.Fatalf("a capacity refusal ended the attempt: %v", err)
	}
	if refused == 0 {
		t.Fatal("no page was refused")
	}
	if _, twice := s.count(); twice != 0 {
		t.Fatalf("%d entries were asked for twice", twice)
	}
	requireWhole(t, src, dst, u)
}

// TestABusyPeerIsPassedForTheNext — #4493: a page refused for capacity is
// asked of the next peer, not only of the same one after a pause, and the
// peer that served is asked first for the pages after.
func TestABusyPeerIsPassedForTheNext(t *testing.T) {
	quickRetry(t)
	const n = 1000
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	buildChain(t, src, u, n, 0x46)
	dst := newObservedDB(t)
	takeHeads(t, src, dst, u)

	busy, free := newPagedSource(src), newPagedSource(src)
	busy.before = func(_ int, start, _ uint64) error { return capacityExhausted(start) }
	if err := Backfill(context.Background(), []Source{busy, free}, dst, u, 256, nil); err != nil {
		t.Fatal(err)
	}
	if pages, _ := busy.count(); pages != 1 {
		t.Fatalf("the busy peer was asked for %d pages after another peer served, want 1", pages)
	}
	if pages, _ := free.count(); pages != 4 {
		t.Fatalf("the free peer was asked for %d pages, want 4", pages)
	}
	requireWhole(t, src, dst, u)
}

// TestAPageNoPeerServesFailsTheChainAndTheNextAttemptResumes — #4493: only a
// NotFound from every peer fails the chain, and even then what was written is
// kept: the next attempt resumes, it does not restart.
func TestAPageNoPeerServesFailsTheChainAndTheNextAttemptResumes(t *testing.T) {
	quickRetry(t)
	const n = 3000
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(t)
	buildChain(t, src, u, n, 0x47)
	dst := newObservedDB(t)
	takeHeads(t, src, dst, u)

	const missing = 1024 // A page start half-way down the walk
	gone := true
	notFound := func(_ int, start, _ uint64) error {
		if gone && start <= missing && missing < start+256 {
			return errors.NotFound.WithFormat("entry %d: pruned", start)
		}
		return nil
	}
	a, b := newPagedSource(src), newPagedSource(src)
	a.before, b.before = notFound, notFound
	err := Backfill(context.Background(), []Source{a, b}, dst, u, 256, nil)
	if !errors.Is(err, errors.NotFound) {
		t.Fatalf("a page no peer serves did not fail the chain: %v", err)
	}
	// 3000 entries: [2048,2560) and [2560,2816) were proven before the walk
	// reached the page no peer serves, inside [0,2048).
	requireGaps(t, dst, u, database.SyncedGap{Low: 0, High: 2048})

	gone = false
	if err := Backfill(context.Background(), []Source{a, b}, dst, u, 256, nil); err != nil {
		t.Fatal(err)
	}
	pa, _ := a.count()
	pb, _ := b.count()
	askedTwiceOutside(t, a, 0, 2048)
	askedTwiceOutside(t, b, 0, 2048)
	for idx := range a.asked {
		if idx >= 2048 && b.asked[idx] > 0 {
			t.Fatalf("entry %d was asked of both peers (%d and %d pages)", idx, pa, pb)
		}
	}
	requireWhole(t, src, dst, u)
	requireGaps(t, dst, u)
}
