// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bptproof

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// fillBPT inserts n synthetic (key, value) leaves into the local
// BPT. Each key is the sha256 of the index; each value is a
// distinct 32-byte pattern. Returns the in-database BPT root after
// commit.
func fillBPT(t *testing.T, db *database.Database, n int) [32]byte {
	t.Helper()
	batch := db.Begin(true)
	for i := 0; i < n; i++ {
		var keyHash [32]byte
		keyHash[0] = byte(i)
		keyHash[1] = byte(i >> 8)
		keyHash[31] = 0xab
		var value [32]byte
		value[0] = byte(i)
		value[31] = 0xcd
		if err := batch.BPT().Insert(record.KeyFromHash(keyHash), value[:]); err != nil {
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

// TestGetPage_Empty returns Done with no entries.
func TestGetPage_Empty(t *testing.T) {
	db := database.OpenInMemory(nil)
	batch := db.Begin(false)
	defer batch.Discard()

	page, err := GetPage(batch, FullScanStart(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Done {
		t.Error("empty BPT page should be Done")
	}
	if len(page.Entries) != 0 {
		t.Errorf("got %d entries, want 0", len(page.Entries))
	}
}

// TestGetPage_SinglePage fits everything in one page.
func TestGetPage_SinglePage(t *testing.T) {
	db := database.OpenInMemory(nil)
	expectedRoot := fillBPT(t, db, 5)

	batch := db.Begin(false)
	defer batch.Discard()
	page, err := GetPage(batch, FullScanStart(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Done {
		t.Error("expected Done when pageSize > leaf count")
	}
	if len(page.Entries) != 5 {
		t.Errorf("got %d entries, want 5", len(page.Entries))
	}
	if page.BptRoot != expectedRoot {
		t.Errorf("BptRoot = %x, want %x", page.BptRoot, expectedRoot)
	}
}

// TestGetPage_MultipleConsecutivePages — the central scenario
// the enumeration consumer will exercise. Walk the BPT in pages
// of 3, accumulate all leaves, verify count and uniqueness.
func TestGetPage_MultipleConsecutivePages(t *testing.T) {
	const total = 20
	db := database.OpenInMemory(nil)
	expectedRoot := fillBPT(t, db, total)

	batch := db.Begin(false)
	defer batch.Discard()

	seen := make(map[[32]byte]bool)
	start := FullScanStart()
	pages := 0
	for {
		page, err := GetPage(batch, start, 3)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, e := range page.Entries {
			if seen[e.KeyHash] {
				t.Errorf("duplicate key %x across pages", e.KeyHash)
			}
			seen[e.KeyHash] = true
		}
		if page.BptRoot != expectedRoot {
			t.Errorf("page %d BptRoot drifted; got %x want %x", pages, page.BptRoot, expectedRoot)
		}
		if page.Done {
			break
		}
		start = page.NextStart
	}
	if len(seen) != total {
		t.Errorf("collected %d unique keys, want %d", len(seen), total)
	}
	if pages < 6 {
		t.Errorf("expected >=6 pages of 3 over %d entries, got %d", total, pages)
	}
}

// TestGetPage_LocalRootReconstructs — central round-trip: scan
// the source BPT page-by-page, insert into a target BPT, the
// target's root must equal the source's. This is the model the
// launcher's enumeration step relies on.
func TestGetPage_LocalRootReconstructs(t *testing.T) {
	const total = 50
	src := database.OpenInMemory(nil)
	srcRoot := fillBPT(t, src, total)

	srcBatch := src.Begin(false)
	defer srcBatch.Discard()

	// Pull every page from src; insert each leaf into dst.
	dst := database.OpenInMemory(nil)
	dstBatch := dst.Begin(true)

	start := FullScanStart()
	for {
		page, err := GetPage(srcBatch, start, 7)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range page.Entries {
			if err := dstBatch.BPT().Insert(record.KeyFromHash(e.KeyHash), e.ValueHash[:]); err != nil {
				t.Fatal(err)
			}
		}
		if page.Done {
			break
		}
		start = page.NextStart
	}
	if err := dstBatch.Commit(); err != nil {
		t.Fatal(err)
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

// TestGetPage_RejectsZeroPageSize — input guard.
func TestGetPage_RejectsZeroPageSize(t *testing.T) {
	db := database.OpenInMemory(nil)
	batch := db.Begin(false)
	defer batch.Discard()

	_, err := GetPage(batch, FullScanStart(), 0)
	if err == nil {
		t.Fatal("expected error for pageSize=0")
	}
}
