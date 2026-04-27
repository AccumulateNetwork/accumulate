// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package backfill

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
)

type stubFetcher struct {
	calls atomic.Int64
}

func (f *stubFetcher) FetchAndStore(_ context.Context, _ string, _ uint64) (int, error) {
	f.calls.Add(1)
	return 3, nil // pretend each block had 3 transactions
}

type failFetcher struct{}

func (failFetcher) FetchAndStore(_ context.Context, _ string, _ uint64) (int, error) {
	return 0, errors.New("stub failure")
}

func mkActive() *nodestate.Machine {
	m := nodestate.New()
	m.PromoteToActive([32]byte{1}, 100)
	return m
}

func TestNew_RejectsBadOpts(t *testing.T) {
	_, err := New(Options{})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions, got %v", err)
	}
	_, err = New(Options{State: nodestate.New(), Fetcher: &stubFetcher{}})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("missing partition: expected ErrInvalidOptions, got %v", err)
	}
}

func TestBackfill_ReachesTargetAndPromotesComplete(t *testing.T) {
	st := mkActive()
	f := &stubFetcher{}
	b, err := New(Options{
		State:        st,
		Fetcher:      f,
		Partition:    "Directory",
		StartFrom:    105,
		TargetDepth:  100,
		PollInterval: 1 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	b.Start(ctx)
	defer b.Stop()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if st.State() == nodestate.StateComplete {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if st.State() != nodestate.StateComplete {
		t.Fatalf("expected COMPLETE, got %v after %d fetches", st.State(), f.calls.Load())
	}
	stats := b.Stats()
	if stats.BlocksFetched < 5 {
		t.Errorf("expected ~5 blocks fetched, got %d", stats.BlocksFetched)
	}
}

func TestBackfill_FailedFetchesAreCounted(t *testing.T) {
	st := mkActive()
	b, err := New(Options{
		State:        st,
		Fetcher:      failFetcher{},
		Partition:    "Directory",
		StartFrom:    105,
		TargetDepth:  100,
		PollInterval: 1 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	b.Start(ctx)
	defer b.Stop()

	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
		if b.Stats().Failed > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if b.Stats().Failed == 0 {
		t.Fatal("expected at least one failure")
	}
	// Should remain ACTIVE since target wasn't reached.
	if st.State() != nodestate.StateActive {
		t.Fatalf("state = %v, want ACTIVE", st.State())
	}
}
