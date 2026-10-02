// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"sync"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// pagedSource serves a database's chain entries and counts what it is asked:
// every page, and every entry by index. before, when set, is called ahead of
// each page and fails it when it returns an error.
type pagedSource struct {
	*dbSource
	mu     sync.Mutex
	pages  int
	asked  map[uint64]int
	before func(page int, start, count uint64) error
}

func newPagedSource(db *database.Database) *pagedSource {
	return &pagedSource{dbSource: &dbSource{db: db}, asked: map[uint64]int{}}
}

func (s *pagedSource) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	s.mu.Lock()
	s.pages++
	page := s.pages
	start, count := q.Range.Start, *q.Range.Count
	before := s.before
	s.mu.Unlock()
	if before != nil {
		if err := before(page, start, count); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	for i := start; i < start+count; i++ {
		s.asked[i]++
	}
	s.mu.Unlock()
	return s.dbSource.QueryChainEntries(ctx, u, q)
}

// count is how many pages were asked for and how many entries were asked
// for more than once.
func (s *pagedSource) count() (pages, twice int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range s.asked {
		if n > 1 {
			twice++
		}
	}
	return s.pages, twice
}

func (s *pagedSource) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pages = 0
	s.asked = map[uint64]int{}
	s.before = nil
}

// growChain appends n entries to u's main chain in src, from entry from.
func growChain(t testing.TB, src *database.Database, u *url.URL, from, n int, salt byte) {
	t.Helper()
	b := src.Begin(true)
	defer b.Discard()
	for i := from; i < from+n; i++ {
		addTransactionEntry(t, b, u, i, salt)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
}

// takeHeads takes u from src by its chain heads and open mark sets alone, as
// the join does (ModeStateOnly).
func takeHeads(t testing.TB, src, dst *database.Database, u *url.URL) {
	t.Helper()
	b := dst.Begin(true)
	defer b.Discard()
	if err := Account(context.Background(), &dbSource{db: src}, b, u, Options{Mode: ModeStateOnly}); err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}
}

// requireWhole fails unless dst holds every entry of u's main chain that src
// holds, each the same, with the message behind it.
func requireWhole(t testing.TB, src, dst *database.Database, u *url.URL) {
	t.Helper()
	s, d := src.Begin(false), dst.Begin(false)
	defer s.Discard()
	defer d.Discard()
	sc, err := s.Account(u).MainChain().Get()
	if err != nil {
		t.Fatal(err)
	}
	dc, err := d.Account(u).MainChain().Get()
	if err != nil {
		t.Fatal(err)
	}
	n := sc.CurrentState().Count
	if dc.CurrentState().Count != n {
		t.Fatalf("the node's chain is %d entries, the peer's %d", dc.CurrentState().Count, n)
	}
	for i := int64(0); i < n; i++ {
		want, err := sc.Entry(i)
		if err != nil {
			t.Fatal(err)
		}
		got, err := dc.Entry(i)
		if err != nil {
			t.Fatalf("entry %d is not held: %v", i, err)
		}
		if string(got) != string(want) {
			t.Fatalf("entry %d is not the peer's", i)
		}
		if _, err := d.Message2(got).Main().Get(); err != nil {
			t.Fatalf("the message behind entry %d is not held: %v", i, err)
		}
	}
	// The mark points are written: a receipt of the oldest entry reaches the
	// head.
	if n > 0 {
		if _, err := dc.Receipt(0, n-1); err != nil {
			t.Fatalf("no receipt from the first entry to the head: %v", err)
		}
	}
}

// BenchmarkBackfill50k is the in-process measurement of #4493: one account
// whose main chain holds 50,000 entries, each with its message, backfilled
// from a node that took it by its heads -- once uninterrupted, and once with
// the source failing at the half-way page and the backfill asked again.
func BenchmarkBackfill50k(b *testing.B) {
	const n = 50_000
	u := protocol.DnUrl().JoinPath("anchors")
	src := newObservedDB(b)
	buildChain(b, src, u, n, 0x50)

	for _, interrupt := range []bool{false, true} {
		name := "whole"
		if interrupt {
			name = "interrupted-at-50pct"
		}
		b.Run(name, func(b *testing.B) {
			var pages int
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				dst := newObservedDB(b)
				takeHeads(b, src, dst, u)
				s := newPagedSource(src)
				if interrupt {
					s.before = func(page int, _, _ uint64) error {
						if page == n/256/2 {
							return errInterrupted
						}
						return nil
					}
				}
				b.StartTimer()
				start := time.Now()
				err := Backfill(context.Background(), []Source{s}, dst, u, 256, nil)
				if interrupt {
					if err == nil {
						b.Fatal("the interruption did not interrupt")
					}
					s.mu.Lock()
					s.before = nil
					s.mu.Unlock()
					err = Backfill(context.Background(), []Source{s}, dst, u, 256, nil)
				}
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(time.Since(start).Milliseconds()), "ms/fill")
				pages, _ = s.count()
				b.StopTimer()
				requireWhole(b, src, dst, u)
				b.StartTimer()
			}
			b.ReportMetric(float64(pages), "pages")
		})
	}
}

var errInterrupted = errorString("interrupted")

type errorString string

func (e errorString) Error() string { return string(e) }
