// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enumerate

import (
	"context"
	"fmt"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bptproof"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/tracker"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
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
	page, err := bptproof.GetPage(bptproof.Current(roBatch), startKey, count)
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
			Account:   e.Account,
		}
	}
	return out, nil
}

func observedDB(t *testing.T) *database.Database {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	return db
}

// accountUrl is the i-th account of the fixture partition.
func accountUrl(i int) *url.URL {
	return protocol.DnUrl().JoinPath(fmt.Sprintf("acct-%d", i))
}

// fill writes n accounts, each carrying a chain entry that distinguishes it,
// and returns the resulting BPT root. The leaves are derived from state, which
// is the only way a leaf may enter a BPT.
func fill(t *testing.T, db *database.Database, n int, salt byte) [32]byte {
	t.Helper()
	batch := db.Begin(true)
	defer batch.Discard()
	for i := 0; i < n; i++ {
		u := accountUrl(i)
		if err := batch.Account(u).Main().Put(&protocol.DataAccount{Url: u}); err != nil {
			t.Fatal(err)
		}
		e := make([]byte, 32)
		e[0] = byte(i)
		e[1] = byte(i >> 8)
		e[31] = salt
		if err := batch.Account(u).MainChain().Inner().AddEntry(e, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := batch.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}
	return rootOf(t, db)
}

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

// TestReadPage_LearnsWithoutWriting is the rule: enumeration learns the peer's key
// set and its claimed value hashes and writes nothing. After it, the local
// root is not the peer's root and the tracker must not promote — the local
// root is only allowed to mean something once it is derived from state this
// node holds.
func TestReadPage_LearnsWithoutWriting(t *testing.T) {
	const total = 40
	src := observedDB(t)
	srcRoot := fill(t, src, total, 0x42)

	dst := observedDB(t)
	before := rootOf(t, dst)

	dstBatch := dst.Begin(true)
	scope := protocol.DnUrl()
	res, err := walkAll(context.Background(), &dbSource{db: src}, scope, dstBatch, 7)
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if err := dstBatch.Commit(); err != nil {
		t.Fatal(err)
	}

	if len(res.Accounts) != total {
		t.Errorf("Accounts named = %d, want %d", len(res.Accounts), total)
	}
	if len(res.Stale) != total {
		t.Errorf("Stale = %d, want %d: a node holding nothing must find every account stale", len(res.Stale), total)
	}
	if res.PagesPulled < 6 {
		t.Errorf("PagesPulled = %d, want >= 6 over %d entries with pageSize 7", res.PagesPulled, total)
	}
	if res.LastBptRoot != srcRoot {
		t.Errorf("LastBptRoot = %x, want %x (the peer's word for its own root)", res.LastBptRoot, srcRoot)
	}

	after := rootOf(t, dst)
	if after != before {
		t.Errorf("enumeration changed the local BPT root: %x -> %x", before, after)
	}
	if after == srcRoot {
		t.Fatal("the local BPT root equals the peer's after enumeration alone; " +
			"the peer's leaves were written, and the tracker's proof is defeated")
	}

	// The check that matters: nothing has been pulled, so nothing may match.
	trk, err := tracker.New(dst, scope)
	if err != nil {
		t.Fatal(err)
	}
	trk.Observe(scope, 99, srcRoot)
	_, matched, err := trk.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if matched {
		t.Fatal("the tracker matched on an enumeration alone")
	}
}

// TestReadPage_NamesOnlyWhatMoved — a node holding the peer's state as of its last
// block finds only the leaves that moved since. That is what a restart asks.
func TestReadPage_NamesOnlyWhatMoved(t *testing.T) {
	const total = 12
	src := observedDB(t)
	fill(t, src, total, 0x42)

	// The node holds the same state, so its leaves agree.
	dst := observedDB(t)
	fill(t, dst, total, 0x42)

	scope := protocol.DnUrl()
	batch := dst.Begin(true)
	res, err := walkAll(context.Background(), &dbSource{db: src}, scope, batch, 5)
	batch.Discard()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Stale) != 0 {
		t.Fatalf("a node holding the same state found %d stale accounts: %v", len(res.Stale), res.Stale)
	}

	// The peer moves one account on.
	moved := accountUrl(3)
	b := src.Begin(true)
	e := make([]byte, 32)
	e[0] = 0xee
	if err := b.Account(moved).MainChain().Inner().AddEntry(e, false); err != nil {
		t.Fatal(err)
	}
	if err := b.UpdateBPT(); err != nil {
		t.Fatal(err)
	}
	if err := b.Commit(); err != nil {
		t.Fatal(err)
	}

	batch = dst.Begin(true)
	defer batch.Discard()
	res, err = walkAll(context.Background(), &dbSource{db: src}, scope, batch, 5)
	if err != nil {
		t.Fatal(err)
	}
	stale := res.Stale
	if len(stale) != 1 || !stale[0].Equal(moved) {
		t.Fatalf("stale = %v, want just %v", stale, moved)
	}
}

// walked is a whole walk's pages taken together.
type walked struct {
	PagesPulled int
	LastBptRoot [32]byte
	Accounts    []*url.URL
	Stale       []*url.URL
}

// walkAll reads every page of the peer's BPT, as the join does a few pages a
// round.
func walkAll(ctx context.Context, src Source, scope *url.URL, batch *database.Batch, size uint64) (*walked, error) {
	res := new(walked)
	var start [32]byte
	for {
		page, err := ReadPage(ctx, src, scope, batch, start, size)
		if err != nil {
			return res, err
		}
		res.PagesPulled++
		res.Accounts = append(res.Accounts, page.Accounts...)
		res.Stale = append(res.Stale, page.Stale...)
		res.LastBptRoot = page.Record.BptRoot
		if page.Record.Done {
			return res, nil
		}
		start = page.Record.NextStart
	}
}
