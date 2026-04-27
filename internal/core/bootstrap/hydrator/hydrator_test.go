// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package hydrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/loadtrack"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
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

// stubFetcher always succeeds.
type stubFetcher struct {
	calls atomic.Int64
}

func (f *stubFetcher) FetchAndVerify(_ context.Context, _ [32]byte) error {
	f.calls.Add(1)
	return nil
}

// failFetcher always fails.
type failFetcher struct{}

func (failFetcher) FetchAndVerify(_ context.Context, _ [32]byte) error {
	return errors.New("stub failure")
}

func newSetup(t *testing.T, hashes ...byte) (*loadtrack.Tracker, *nodestate.Machine) {
	t.Helper()
	m := mapSource{}
	for _, h := range hashes {
		m[mkHash(h)] = struct{}{}
	}
	tr := loadtrack.New(m)
	if err := tr.Init(); err != nil {
		t.Fatal(err)
	}
	return tr, nodestate.New()
}

func TestNew_RejectsBadOpts(t *testing.T) {
	_, err := New(Options{})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
}

func TestEnqueue_AlreadyLoadedReturnsFalse(t *testing.T) {
	tr, st := newSetup(t, 1)
	h, err := New(Options{Tracker: tr, State: st, Fetcher: &stubFetcher{}})
	if err != nil {
		t.Fatal(err)
	}
	tr.MarkLoaded(mkHash(1))
	if h.Enqueue(mkHash(1), SourceTouch) {
		t.Fatal("Enqueue for already-loaded key should return false")
	}
}

func TestEnqueue_FillsQueueAndDrops(t *testing.T) {
	tr, st := newSetup(t, 1, 2, 3)
	h, err := New(Options{
		Tracker:   tr,
		State:     st,
		Fetcher:   &stubFetcher{},
		Workers:   1,
		QueueSize: 1, // very small
	})
	if err != nil {
		t.Fatal(err)
	}
	if !h.Enqueue(mkHash(1), SourceTouch) {
		t.Fatal("first Enqueue should succeed")
	}
	// Without Start, the queue won't drain; second Enqueue should drop.
	if h.Enqueue(mkHash(2), SourceTouch) {
		t.Fatal("second Enqueue with full queue should drop")
	}
}

func TestStartAndDrain_PromotesToActive(t *testing.T) {
	tr, st := newSetup(t, 1, 2, 3)
	f := &stubFetcher{}
	h, err := New(Options{Tracker: tr, State: st, Fetcher: f, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}

	// Configure ACTIVE trigger before starting.
	root := [32]byte{0xab}
	h.SetActiveTrigger(root, 100)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h.Start(ctx)
	defer h.Stop()

	// Enqueue all three.
	for _, b := range []byte{1, 2, 3} {
		if !h.Enqueue(mkHash(b), SourceEnumerator) {
			t.Fatalf("Enqueue %d", b)
		}
	}

	// Wait for state transition.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if st.State() == nodestate.StateActive {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if st.State() != nodestate.StateActive {
		t.Fatalf("expected ACTIVE, got %v after %d fetches", st.State(), f.calls.Load())
	}
	if got := h.Stats(); got.FetchedEnumerator != 3 {
		t.Errorf("FetchedEnumerator = %d, want 3", got.FetchedEnumerator)
	}
}

func TestStartAndDrain_FailureCounted(t *testing.T) {
	tr, st := newSetup(t, 1)
	h, err := New(Options{Tracker: tr, State: st, Fetcher: failFetcher{}, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	h.Start(ctx)
	defer h.Stop()

	h.Enqueue(mkHash(1), SourceTouch)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if h.Stats().Failed > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if h.Stats().Failed == 0 {
		t.Fatal("expected at least one failure")
	}
	// Tracker should not have marked the entry loaded.
	if tr.IsLoaded(mkHash(1)) {
		t.Fatal("failed fetch should not mark loaded")
	}
	// State should remain BOOTING.
	if st.State() != nodestate.StateBooting {
		t.Fatalf("expected BOOTING, got %v", st.State())
	}
}
