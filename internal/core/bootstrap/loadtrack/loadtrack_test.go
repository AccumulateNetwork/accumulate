// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package loadtrack

import (
	"sync/atomic"
	"testing"
)

type mapSource map[[32]byte]struct{}

func (m mapSource) AllKeyHashes(fn func([32]byte) error) error {
	for kh := range m {
		if err := fn(kh); err != nil {
			return err
		}
	}
	return nil
}

func mkHash(b byte) [32]byte {
	var h [32]byte
	h[0] = b
	return h
}

func TestTracker_BasicLoadProgression(t *testing.T) {
	src := mapSource{
		mkHash(1): {},
		mkHash(2): {},
		mkHash(3): {},
	}
	tr := New(src)
	if err := tr.Init(); err != nil {
		t.Fatal(err)
	}
	if got, want := tr.UnloadedCount(), 3; got != want {
		t.Fatalf("UnloadedCount = %d, want %d", got, want)
	}
	if got, want := tr.KnownCount(), 3; got != want {
		t.Fatalf("KnownCount = %d, want %d", got, want)
	}

	tr.MarkLoaded(mkHash(1))
	if got, want := tr.UnloadedCount(), 2; got != want {
		t.Fatalf("after one load: UnloadedCount = %d, want %d", got, want)
	}
	if !tr.IsLoaded(mkHash(1)) {
		t.Fatal("hash(1) should be loaded")
	}
	if tr.IsLoaded(mkHash(2)) {
		t.Fatal("hash(2) should not be loaded")
	}
}

func TestTracker_OnAllLoaded(t *testing.T) {
	src := mapSource{mkHash(1): {}, mkHash(2): {}}
	tr := New(src)
	if err := tr.Init(); err != nil {
		t.Fatal(err)
	}

	var fired int32
	tr.OnAllLoaded(func() { atomic.AddInt32(&fired, 1) })
	if atomic.LoadInt32(&fired) != 0 {
		t.Fatal("callback should not have fired yet")
	}

	tr.MarkLoaded(mkHash(1))
	if atomic.LoadInt32(&fired) != 0 {
		t.Fatal("callback should not have fired with 1 unloaded")
	}

	tr.MarkLoaded(mkHash(2))
	if atomic.LoadInt32(&fired) != 1 {
		t.Fatalf("callback should have fired exactly once, got %d", fired)
	}

	// Marking an already-loaded entry should not refire.
	tr.MarkLoaded(mkHash(2))
	if atomic.LoadInt32(&fired) != 1 {
		t.Fatalf("idempotent mark should not refire callback, got %d", fired)
	}
}

func TestTracker_OnAllLoaded_AlreadyAtZero(t *testing.T) {
	src := mapSource{}
	tr := New(src)
	if err := tr.Init(); err != nil {
		t.Fatal(err)
	}

	var fired int32
	tr.OnAllLoaded(func() { atomic.AddInt32(&fired, 1) })
	if atomic.LoadInt32(&fired) != 1 {
		t.Fatalf("registering at zero should fire immediately, got %d", fired)
	}
}

func TestTracker_AddLeaf(t *testing.T) {
	src := mapSource{mkHash(1): {}}
	tr := New(src)
	if err := tr.Init(); err != nil {
		t.Fatal(err)
	}

	tr.MarkLoaded(mkHash(1))
	if got, want := tr.UnloadedCount(), 0; got != want {
		t.Fatalf("UnloadedCount = %d, want %d", got, want)
	}

	// Late-arriving leaf re-opens the unloaded count.
	tr.AddLeaf(mkHash(2))
	if got, want := tr.UnloadedCount(), 1; got != want {
		t.Fatalf("after AddLeaf: UnloadedCount = %d, want %d", got, want)
	}
	if !tr.IsKnown(mkHash(2)) {
		t.Fatal("hash(2) should be known")
	}
	if tr.IsLoaded(mkHash(2)) {
		t.Fatal("hash(2) should not yet be loaded")
	}
}

func TestTracker_IterUnloaded(t *testing.T) {
	src := mapSource{
		mkHash(1): {}, mkHash(2): {}, mkHash(3): {}, mkHash(4): {},
	}
	tr := New(src)
	if err := tr.Init(); err != nil {
		t.Fatal(err)
	}

	tr.MarkLoaded(mkHash(2))
	tr.MarkLoaded(mkHash(4))

	var seen []byte
	tr.IterUnloaded(func(kh [32]byte) bool {
		seen = append(seen, kh[0])
		return true
	})

	if len(seen) != 2 {
		t.Fatalf("expected 2 unloaded, got %d", len(seen))
	}
	for _, b := range seen {
		if b != 1 && b != 3 {
			t.Fatalf("unexpected unloaded leaf %d", b)
		}
	}
}
