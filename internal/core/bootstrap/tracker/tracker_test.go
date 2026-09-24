// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tracker

import (
	"context"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// part is the partition every tracker in these tests belongs to.
func part() *url.URL { return protocol.PartitionUrl("BVN0") }

func newTrackerDB(t *testing.T) *database.Database {
	t.Helper()
	return database.OpenInMemory(nil)
}

// rootOf returns the current BPT root of db.
func rootOf(t *testing.T, db *database.Database) [32]byte {
	t.Helper()
	b := db.Begin(false)
	defer b.Discard()
	r, err := b.GetBptRootHash()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// fillRange inserts entries [start, start+n) into db's BPT and
// returns the resulting root. Distinct ranges produce distinct roots.
func fillRange(t *testing.T, db *database.Database, start, n int) [32]byte {
	t.Helper()
	b := db.Begin(true)
	for i := start; i < start+n; i++ {
		var k [32]byte
		k[0] = byte(i + 1) // avoid zero key
		k[1] = byte((i + 1) >> 8)
		var v [32]byte
		v[0] = byte(i + 1)
		v[1] = byte((i + 1) >> 8)
		if err := b.BPT().Insert(record.KeyFromHash(k), v[:]); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
	return rootOf(t, db)
}

// fillN is fillRange(0, n).
func fillN(t *testing.T, db *database.Database, n int) [32]byte {
	t.Helper()
	return fillRange(t, db, 0, n)
}

// TestCheck_ReportsTheMatch is the happy path: the tracker holds the anchor,
// the local root equals it, and Check reports the block it was anchored for
// and the root.
func TestCheck_ReportsTheMatch(t *testing.T) {
	db := newTrackerDB(t)
	root := fillN(t, db, 5)

	tr, err := New(db, part())
	if err != nil {
		t.Fatal(err)
	}
	tr.MatchThreshold = 1
	tr.Observe(part(), 42, root)

	m, ok, err := tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected a match")
	}
	if m.Block != 42 || m.Anchor != root {
		t.Errorf("match = %+v, want block 42 at %x", m, root)
	}
}

// TestCheck_NoMatch — the local root equals no observed anchor.
func TestCheck_NoMatch(t *testing.T) {
	db := newTrackerDB(t)
	fillN(t, db, 3)

	tr, _ := New(db, part())
	tr.MatchThreshold = 1
	var bogus [32]byte
	bogus[0] = 0xff
	tr.Observe(part(), 7, bogus)

	_, ok, err := tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("did not expect a match")
	}
}

// TestCheck_MovingTarget — anchors arrive faster than ingestion. The tracker
// holds anchor1 and anchor2; the local root catches up to anchor1 first and
// Check reports block 10; it catches up to anchor2 and Check reports block 20.
// A match is reported every time it holds: the tracker promotes nothing, so
// there is no "already promoted" to stop it (#4385).
func TestCheck_MovingTarget(t *testing.T) {
	src := newTrackerDB(t)
	dst := newTrackerDB(t)

	root1 := fillRange(t, src, 0, 4)
	root2 := fillRange(t, src, 4, 4)
	if root1 == root2 {
		t.Fatal("test setup: roots should differ")
	}

	tr, _ := New(dst, part())
	tr.MatchThreshold = 1
	tr.Observe(part(), 10, root1)
	tr.Observe(part(), 20, root2)

	if got := fillRange(t, dst, 0, 4); got != root1 {
		t.Fatalf("expected dst root %x to equal root1 %x", got, root1)
	}
	m, ok, err := tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok || m.Block != 10 {
		t.Fatalf("match = %+v %v, want block 10", m, ok)
	}

	if got := fillRange(t, dst, 4, 4); got != root2 {
		t.Fatalf("expected dst root %x to equal root2 %x", got, root2)
	}
	m, ok, err = tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok || m.Block != 20 {
		t.Fatalf("match = %+v %v, want block 20", m, ok)
	}
}

// TestObserve_IgnoresZeroAnchor — defensive: a zero anchor (e.g., a
// pre-genesis or malformed header) is silently ignored.
func TestObserve_IgnoresZeroAnchor(t *testing.T) {
	db := newTrackerDB(t)
	tr, _ := New(db, part())
	tr.Observe(part(), 99, [32]byte{})
	if tr.ObservedCount() != 0 {
		t.Errorf("ObservedCount=%d, want 0 (zero anchor ignored)", tr.ObservedCount())
	}
}

// TestObserve_KeepsEarliestBlockForSameAnchor — the same anchor seen at two
// different blocks is recorded with the earliest block, since that's when it
// first became valid.
func TestObserve_KeepsEarliestBlockForSameAnchor(t *testing.T) {
	db := newTrackerDB(t)
	root := fillN(t, db, 2)

	tr, _ := New(db, part())
	tr.MatchThreshold = 1
	tr.Observe(part(), 100, root)
	tr.Observe(part(), 50, root) // earlier — should win
	tr.Observe(part(), 150, root)

	m, ok, err := tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok || m.Block != 50 {
		t.Errorf("match = %+v %v, want block 50 (earliest seen)", m, ok)
	}
}

// TestCheck_ContextCanceled returns ctx.Err.
func TestCheck_ContextCanceled(t *testing.T) {
	db := newTrackerDB(t)
	tr, _ := New(db, part())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := tr.Check(ctx); err == nil {
		t.Fatal("expected ctx.Err()")
	}
}

// TestNew_RejectsMissingInputs — guards.
func TestNew_RejectsMissingInputs(t *testing.T) {
	db := newTrackerDB(t)
	if _, err := New(nil, part()); err == nil {
		t.Error("expected err for nil db")
	}
	if _, err := New(db, nil); err == nil {
		t.Error("expected err for nil partition")
	}
}

// TestSnapshot_RestoreRoundTrip — Snapshot of one tracker, RestoreFrom into a
// fresh tracker, both match the same way.
func TestSnapshot_RestoreRoundTrip(t *testing.T) {
	db := newTrackerDB(t)
	root := fillN(t, db, 4)

	src, _ := New(db, part())
	src.Observe(part(), 11, root)
	src.Observe(part(), 22, [32]byte{0xab})
	src.Observe(part(), 7, [32]byte{0xcd})

	snap := src.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("snap len = %d, want 3", len(snap))
	}

	dst, _ := New(db, part())
	dst.MatchThreshold = 1
	dst.RestoreFrom(snap)
	if dst.ObservedCount() != 3 {
		t.Errorf("dst observed count = %d, want 3", dst.ObservedCount())
	}

	m, ok, err := dst.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok || m.Block != 11 {
		t.Fatalf("match after restore = %+v %v, want block 11", m, ok)
	}
}

// TestLatestObservedBlock — accessor accuracy.
func TestLatestObservedBlock(t *testing.T) {
	db := newTrackerDB(t)
	tr, _ := New(db, part())
	if tr.LatestObservedBlock() != 0 {
		t.Errorf("LatestObservedBlock=%d, want 0", tr.LatestObservedBlock())
	}
	var a, b [32]byte
	a[0] = 1
	b[0] = 2
	tr.Observe(part(), 7, a)
	tr.Observe(part(), 3, b) // earlier — should not regress latest
	tr.Observe(part(), 15, a)
	if tr.LatestObservedBlock() != 15 {
		t.Errorf("LatestObservedBlock=%d, want 15", tr.LatestObservedBlock())
	}
}

// TestCheck_ThresholdRequiresConsecutiveMatches — a match is reported only
// after MatchThreshold consecutive Check calls match, and on every call after
// that while it holds.
func TestCheck_ThresholdRequiresConsecutiveMatches(t *testing.T) {
	const threshold = 10
	db := newTrackerDB(t)
	root := fillN(t, db, 3)

	tr, _ := New(db, part())
	tr.MatchThreshold = threshold
	tr.Observe(part(), 50, root)

	for i := 1; i < threshold; i++ {
		_, ok, err := tr.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Fatalf("matched at streak %d, want only at %d", i, threshold)
		}
		if got := tr.ConsecutiveMatches(); got != i {
			t.Errorf("after Check %d: ConsecutiveMatches=%d, want %d", i, got, i)
		}
	}
	for i := 0; i < 2; i++ {
		m, ok, err := tr.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok || m.Block != 50 {
			t.Fatalf("check %d at or past the threshold: %+v %v, want block 50", threshold+i, m, ok)
		}
	}
}

// TestCheck_MismatchResetsStreak — any non-matching Check resets the
// consecutive counter to zero, and so does ResetStreak.
func TestCheck_MismatchResetsStreak(t *testing.T) {
	db := newTrackerDB(t)
	root := fillN(t, db, 3)

	tr, _ := New(db, part())
	tr.MatchThreshold = 3
	tr.Observe(part(), 7, root)

	for i := 0; i < 2; i++ {
		_, _, _ = tr.Check(context.Background())
	}
	if got := tr.ConsecutiveMatches(); got != 2 {
		t.Fatalf("expected streak 2, got %d", got)
	}
	tr.ResetStreak()
	if got := tr.ConsecutiveMatches(); got != 0 {
		t.Fatalf("after ResetStreak: streak=%d, want 0", got)
	}

	for i := 0; i < 2; i++ {
		_, _, _ = tr.Check(context.Background())
	}
	_ = fillRange(t, db, 100, 1)
	_, _, _ = tr.Check(context.Background())
	if got := tr.ConsecutiveMatches(); got != 0 {
		t.Errorf("after mismatch: streak=%d, want 0", got)
	}
}
