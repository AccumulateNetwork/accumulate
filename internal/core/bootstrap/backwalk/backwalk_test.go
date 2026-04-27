// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package backwalk

import (
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func TestNew_Defaults(t *testing.T) {
	pin := [32]byte{0xab}
	w := New(Options{PinnedGenesisHash: pin})
	if w.PinnedGenesisHash() != pin {
		t.Fatal("pinned hash not stored")
	}
	if w.MemoSize() != 0 {
		t.Fatal("memo should start empty")
	}
}

func TestWalk_NilBatch(t *testing.T) {
	w := New(Options{PinnedGenesisHash: [32]byte{1}})
	u, _ := url.Parse("dn.acme/operators")
	_, err := w.Walk(nil, u, time.Now())
	if err == nil || !contains(err.Error(), "nil batch") {
		t.Fatalf("expected nil-batch error, got %v", err)
	}
}

func TestWalk_NilUrl(t *testing.T) {
	w := New(Options{PinnedGenesisHash: [32]byte{1}})
	_, err := w.Walk(nil, nil, time.Now())
	if err == nil {
		t.Fatal("expected error for nil url")
	}
}

func TestMemoize_AndCacheHit(t *testing.T) {
	w := New(Options{PinnedGenesisHash: [32]byte{1}})
	u, _ := url.Parse("dn.acme/operators")
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	entry := &VerifiedEntry{
		Account:     u,
		BlockTime:   t0,
		GenesisTerm: true,
	}
	w.Memoize(entry)
	if w.MemoSize() != 1 {
		t.Fatalf("expected 1 memo, got %d", w.MemoSize())
	}

	got, err := w.Walk(nil, u, t0)
	if err != nil {
		t.Fatalf("expected cache hit, got %v", err)
	}
	if got != entry {
		t.Fatal("expected cached entry")
	}
}
