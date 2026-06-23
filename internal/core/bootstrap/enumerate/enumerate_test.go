// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enumerate

import (
	"context"
	"errors"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bptproof"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// dbSource adapts a *database.Database to the enumerate.Source
// interface by routing each BptPageQuery through bptproof.GetPage.
// Equivalent to a real peer that actually serves the v3 dispatch
// path (covered by tests in internal/api/v3/bpt_page_test.go).
type dbSource struct{ db *database.Database }

func (s *dbSource) QueryBptPage(_ context.Context, _ *url.URL, query *api.BptPageQuery) (*api.BptPageRecord, error) {
	count := int(query.Count)
	if count <= 0 {
		count = 256
	}
	startKey := query.StartHash
	if startKey == ([32]byte{}) {
		startKey = bptproof.FullScanStart()
	}

	roBatch := s.db.Begin(false)
	defer roBatch.Discard()
	page, err := bptproof.GetPage(roBatch, startKey, count)
	if err != nil {
		return nil, err
	}

	out := &api.BptPageRecord{
		NextStart: page.NextStart,
		BptRoot:   page.BptRoot,
		Done:      page.Done,
		Entries:   make([]*api.BptLeafSummary, len(page.Entries)),
	}
	for i, e := range page.Entries {
		out.Entries[i] = &api.BptLeafSummary{
			KeyHash:   e.KeyHash,
			ValueHash: e.ValueHash,
		}
	}
	return out, nil
}

func fillBPT(t *testing.T, db *database.Database, n int) [32]byte {
	t.Helper()
	batch := db.Begin(true)
	for i := 0; i < n; i++ {
		var k [32]byte
		k[0] = byte(i)
		k[1] = byte(i >> 8)
		k[31] = 0x42
		var v [32]byte
		v[0] = byte(i)
		if err := batch.BPT().Insert(record.KeyFromHash(k), v[:]); err != nil {
			t.Fatal(err)
		}
	}
	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}
	roBatch := db.Begin(false)
	defer roBatch.Discard()
	root, err := roBatch.GetBptRootHash()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// TestRun_ReconstructsBptRoot is the central round-trip: scan the
// source via the consumer loop, insert into a target DB, the
// target's BPT root must match the source's. Mirrors the launcher's
// post-enumeration consistency check.
func TestRun_ReconstructsBptRoot(t *testing.T) {
	const total = 100
	src := database.OpenInMemory(nil)
	srcRoot := fillBPT(t, src, total)

	dst := database.OpenInMemory(nil)
	dstBatch := dst.Begin(true)

	scope, _ := url.Parse(protocol.DnUrl().String())
	res, err := Run(context.Background(), &dbSource{db: src}, scope, dstBatch, Options{
		PageSize: 13, // force multiple pages
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := dstBatch.Commit(); err != nil {
		t.Fatal(err)
	}

	if res.LeavesInserted != total {
		t.Errorf("LeavesInserted = %d, want %d", res.LeavesInserted, total)
	}
	if res.PagesPulled < 8 {
		t.Errorf("PagesPulled = %d, want >= 8 over %d entries with pageSize 13", res.PagesPulled, total)
	}
	if res.LastBptRoot != srcRoot {
		t.Errorf("LastBptRoot = %x, want %x (source's current root)", res.LastBptRoot, srcRoot)
	}

	dstRO := dst.Begin(false)
	defer dstRO.Discard()
	dstRoot, err := dstRO.GetBptRootHash()
	if err != nil {
		t.Fatal(err)
	}
	if dstRoot != srcRoot {
		t.Errorf("reconstructed BPT root mismatch:\n  src=%x\n  dst=%x", srcRoot, dstRoot)
	}
}

// TestRun_OnPageCallback fires on every page.
func TestRun_OnPageCallback(t *testing.T) {
	src := database.OpenInMemory(nil)
	fillBPT(t, src, 25)
	dst := database.OpenInMemory(nil)
	dstBatch := dst.Begin(true)
	defer dstBatch.Discard()

	scope, _ := url.Parse(protocol.DnUrl().String())
	calls := 0
	_, err := Run(context.Background(), &dbSource{db: src}, scope, dstBatch, Options{
		PageSize: 5,
		OnPage: func(pageNum int, page *api.BptPageRecord) {
			calls++
			if page == nil {
				t.Error("OnPage called with nil page")
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls < 5 {
		t.Errorf("OnPage called %d times, want >= 5", calls)
	}
}

// TestRun_ContextCancel returns ctx.Err on cancellation.
func TestRun_ContextCancel(t *testing.T) {
	src := database.OpenInMemory(nil)
	fillBPT(t, src, 100)
	dst := database.OpenInMemory(nil)
	dstBatch := dst.Begin(true)
	defer dstBatch.Discard()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled

	scope, _ := url.Parse(protocol.DnUrl().String())
	_, err := Run(ctx, &dbSource{db: src}, scope, dstBatch, Options{PageSize: 1})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled chain", err)
	}
}

// TestRun_RejectsMissingInputs — guards.
func TestRun_RejectsMissingInputs(t *testing.T) {
	scope, _ := url.Parse(protocol.DnUrl().String())
	dst := database.OpenInMemory(nil)
	batch := dst.Begin(true)
	defer batch.Discard()

	cases := []struct {
		name string
		src  Source
		sc   *url.URL
		bt   *database.Batch
	}{
		{"no source", nil, scope, batch},
		{"no scope", &dbSource{db: dst}, nil, batch},
		{"no batch", &dbSource{db: dst}, scope, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Run(context.Background(), c.src, c.sc, c.bt, Options{})
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
