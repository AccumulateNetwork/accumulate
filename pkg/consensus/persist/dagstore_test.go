// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package persist

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// TestDAGStoreRetainSparesLaterWrites: a checkpoint's retention runs
// concurrently with the primary writing the rounds above it (#4448). What
// was written after the checkpoint began must survive its Retain, or a
// restart loses the rounds between the checkpoint and the frontier.
func TestDAGStoreRetainSparesLaterWrites(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenDAGStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	old := &types.Batch{Transactions: [][]byte{[]byte("old")}}
	kept := &types.Batch{Transactions: [][]byte{[]byte("kept")}}
	late := &types.Batch{Transactions: [][]byte{[]byte("late")}}
	for _, b := range []*types.Batch{old, kept} {
		if _, err := s.PutBatch(b); err != nil {
			t.Fatal(err)
		}
	}
	mark := s.Mark()
	if _, err := s.PutBatch(late); err != nil {
		t.Fatal(err)
	}
	s.Retain(map[string]bool{BatchName(kept.Digest()): true}, mark)

	if s.HasBatch(old.Digest()) {
		t.Fatal("a batch no checkpoint names survived Retain")
	}
	for _, b := range []*types.Batch{kept, late} {
		if !s.HasBatch(b.Digest()) {
			t.Fatalf("Retain removed %s", b.Digest())
		}
	}

	// Reopened, the store holds what survived, readable.
	s, err = OpenDAGStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(s.Names()); n != 2 {
		t.Fatalf("reopened store holds %d entries, want 2", n)
	}
	if b, err := s.Batch(BatchName(late.Digest())); err != nil || b.Digest() != late.Digest() {
		t.Fatalf("reading back: %v", err)
	}
}
