// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tracker

import (
	"context"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

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

// TestCheck_PromotesOnMatch is the happy path: tracker holds the
// expected anchor, local root matches, machine flips to ACTIVE with
// the correct anchor and sinceBlock recorded.
func TestCheck_PromotesOnMatch(t *testing.T) {
	db := newTrackerDB(t)
	root := fillN(t, db, 5)

	m := nodestate.New()
	tr, err := New(db, m); tr.MatchThreshold = 1
	if err != nil {
		t.Fatal(err)
	}

	tr.Observe(42, root)

	promoted, err := tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !promoted {
		t.Fatal("expected promotion on match")
	}

	ad := m.Get()
	if ad.State != nodestate.StateActive {
		t.Errorf("state=%v, want ACTIVE", ad.State)
	}
	if ad.SinceBlock != 42 {
		t.Errorf("sinceBlock=%d, want 42", ad.SinceBlock)
	}
	if ad.VerifiedAnchor != root {
		t.Errorf("anchor=%x, want %x", ad.VerifiedAnchor, root)
	}
}

// TestCheck_NoMatchStaysBooting — local doesn't match any observed
// anchor, machine stays BOOTING.
func TestCheck_NoMatchStaysBooting(t *testing.T) {
	db := newTrackerDB(t)
	fillN(t, db, 3)

	m := nodestate.New()
	tr, _ := New(db, m); tr.MatchThreshold = 1

	var bogus [32]byte
	bogus[0] = 0xff
	tr.Observe(7, bogus)

	promoted, err := tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if promoted {
		t.Fatal("did not expect promotion")
	}
	if m.State() != nodestate.StateBooting {
		t.Errorf("state=%v, want BOOTING", m.State())
	}
}

// TestCheck_MovingTarget — anchors arrive faster than ingestion. The
// tracker holds anchor1 and anchor2; local catches up to anchor1
// first; Check promotes with anchor1's block. If we then continue
// ingesting and Check again, we don't re-promote (already ACTIVE).
func TestCheck_MovingTarget(t *testing.T) {
	src := newTrackerDB(t)
	dst := newTrackerDB(t)

	// Source ahead by two anchors; we observe both before catching up.
	root1 := fillRange(t, src, 0, 4)
	root2 := fillRange(t, src, 4, 4) // adds entries 4..7 — different root
	if root1 == root2 {
		t.Fatal("test setup: roots should differ")
	}

	m := nodestate.New()
	tr, _ := New(dst, m); tr.MatchThreshold = 1

	tr.Observe(10, root1)
	tr.Observe(20, root2)

	// Catch dst up to anchor1. Same input → same BPT root.
	if got := fillRange(t, dst, 0, 4); got != root1 {
		t.Fatalf("expected dst root %x to equal root1 %x", got, root1)
	}

	promoted, err := tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !promoted {
		t.Fatal("expected promotion at root1")
	}
	if m.Get().SinceBlock != 10 {
		t.Errorf("sinceBlock=%d, want 10 (the block of root1)", m.Get().SinceBlock)
	}

	// Continue ingesting up to root2.
	if got := fillRange(t, dst, 4, 4); got != root2 {
		t.Fatalf("expected dst root %x to equal root2 %x", got, root2)
	}
	again, err := tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if again {
		t.Error("expected Check to return false on already-ACTIVE machine")
	}
}

// TestObserve_IgnoresZeroAnchor — defensive: a zero anchor (e.g., a
// pre-genesis or malformed header) is silently ignored.
func TestObserve_IgnoresZeroAnchor(t *testing.T) {
	db := newTrackerDB(t)
	tr, _ := New(db, nodestate.New()); tr.MatchThreshold = 1
	tr.Observe(99, [32]byte{})
	if tr.ObservedCount() != 0 {
		t.Errorf("ObservedCount=%d, want 0 (zero anchor ignored)", tr.ObservedCount())
	}
}

// TestObserve_KeepsEarliestBlockForSameAnchor — the same anchor seen
// at two different blocks is recorded with the earliest block, since
// that's when it first became valid.
func TestObserve_KeepsEarliestBlockForSameAnchor(t *testing.T) {
	db := newTrackerDB(t)
	root := fillN(t, db, 2)

	m := nodestate.New()
	tr, _ := New(db, m); tr.MatchThreshold = 1
	tr.Observe(100, root)
	tr.Observe(50, root) // earlier — should win
	tr.Observe(150, root)

	promoted, err := tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !promoted {
		t.Fatal("expected promotion")
	}
	if got := m.Get().SinceBlock; got != 50 {
		t.Errorf("sinceBlock=%d, want 50 (earliest seen)", got)
	}
}

// TestCheck_ContextCanceled returns ctx.Err.
func TestCheck_ContextCanceled(t *testing.T) {
	db := newTrackerDB(t)
	tr, _ := New(db, nodestate.New()); tr.MatchThreshold = 1
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := tr.Check(ctx); err == nil {
		t.Fatal("expected ctx.Err()")
	}
}

// TestNew_RejectsMissingInputs — guards.
func TestNew_RejectsMissingInputs(t *testing.T) {
	db := newTrackerDB(t)
	m := nodestate.New()
	if _, err := New(nil, m); err == nil {
		t.Error("expected err for nil db")
	}
	if _, err := New(db, nil); err == nil {
		t.Error("expected err for nil machine")
	}
}

// TestSnapshot_RestoreRoundTrip — Snapshot of one tracker, RestoreFrom
// into a fresh tracker, both produce the same promotion behavior.
func TestSnapshot_RestoreRoundTrip(t *testing.T) {
	db := newTrackerDB(t)
	root := fillN(t, db, 4)

	src, _ := New(db, nodestate.New())
	src.Observe(11, root)
	src.Observe(22, [32]byte{0xab})
	src.Observe(7, [32]byte{0xcd})

	snap := src.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("snap len = %d, want 3", len(snap))
	}

	dst, _ := New(db, nodestate.New())
	dst.MatchThreshold = 1
	dst.RestoreFrom(snap)
	if dst.ObservedCount() != 3 {
		t.Errorf("dst observed count = %d, want 3", dst.ObservedCount())
	}

	// Promotion still fires on the matching anchor with its block.
	promoted, err := dst.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !promoted {
		t.Fatal("expected promotion after restore")
	}
}

// TestLatestObservedBlock — accessor accuracy.
func TestLatestObservedBlock(t *testing.T) {
	db := newTrackerDB(t)
	tr, _ := New(db, nodestate.New()); tr.MatchThreshold = 1
	if tr.LatestObservedBlock() != 0 {
		t.Errorf("LatestObservedBlock=%d, want 0", tr.LatestObservedBlock())
	}
	var a, b [32]byte
	a[0] = 1
	b[0] = 2
	tr.Observe(7, a)
	tr.Observe(3, b) // earlier — should not regress latest
	tr.Observe(15, a)
	if tr.LatestObservedBlock() != 15 {
		t.Errorf("LatestObservedBlock=%d, want 15", tr.LatestObservedBlock())
	}
}

// TestCheck_ThresholdRequiresConsecutiveMatches — promotion only
// fires after MatchThreshold consecutive Check calls match. A
// single mismatch resets the streak. Default threshold (10) is
// used to validate the production wiring; smaller thresholds are
// covered by other tests.
func TestCheck_ThresholdRequiresConsecutiveMatches(t *testing.T) {
	db := newTrackerDB(t)
	root := fillN(t, db, 3)

	m := nodestate.New()
	tr, _ := New(db, m) // default MatchThreshold = 10
	tr.Observe(50, root)

	// First 9 checks: streak grows but no promotion.
	for i := 1; i < DefaultMatchThreshold; i++ {
		promoted, err := tr.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if promoted {
			t.Fatalf("promoted at streak %d, want only at %d", i, DefaultMatchThreshold)
		}
		if got := tr.ConsecutiveMatches(); got != i {
			t.Errorf("after Check %d: ConsecutiveMatches=%d, want %d", i, got, i)
		}
	}

	// 10th check: promotion fires.
	promoted, err := tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !promoted {
		t.Fatal("expected promotion at default threshold")
	}
	if m.Get().State != nodestate.StateActive {
		t.Errorf("state = %v, want ACTIVE", m.Get().State)
	}
}

// TestCheck_MismatchResetsStreak — any non-matching Check resets
// the consecutive counter to zero. Subsequent matches must
// re-accumulate from scratch.
func TestCheck_MismatchResetsStreak(t *testing.T) {
	db := newTrackerDB(t)
	root := fillN(t, db, 3)

	m := nodestate.New()
	tr, _ := New(db, m)
	tr.MatchThreshold = 3
	tr.Observe(7, root)

	// 2 matches, then mutate db so root changes (mismatch), then back.
	for i := 0; i < 2; i++ {
		_, _ = tr.Check(context.Background())
	}
	if got := tr.ConsecutiveMatches(); got != 2 {
		t.Fatalf("expected streak 2, got %d", got)
	}

	// Mutate db — local root changes, no longer matches observed.
	_ = fillRange(t, db, 100, 1)
	_, _ = tr.Check(context.Background())
	if got := tr.ConsecutiveMatches(); got != 0 {
		t.Errorf("after mismatch: streak=%d, want 0", got)
	}
}
