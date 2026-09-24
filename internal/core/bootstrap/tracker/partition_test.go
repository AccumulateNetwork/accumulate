// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tracker

import (
	"context"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestObserve_IgnoresAnotherPartition — the caller feeds the tracker every
// anchor the Directory executes, and block numbers collide across partitions
// (#4205). An anchor for a partition this tracker is not watching is not an
// observation of it.
func TestObserve_IgnoresAnotherPartition(t *testing.T) {
	db := newTrackerDB(t)
	root := fillN(t, db, 5)

	tr, _ := New(db, part())
	tr.Observe(protocol.DnUrl(), 42, root)
	if got := tr.ObservedCount(); got != 0 {
		t.Fatalf("ObservedCount = %d after another partition's anchor", got)
	}
	_, ok, err := tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("another partition's anchor matched")
	}

	tr.Observe(part(), 42, root)
	if _, ok, err = tr.Check(context.Background()); err != nil || !ok {
		t.Fatalf("the partition's own anchor did not match: %v %v", ok, err)
	}
}

// TestObserve_IsBounded — a join runs for as long as it takes to catch up, and
// an unbounded observation set is an unbounded heap on a node that never
// converges.
func TestObserve_IsBounded(t *testing.T) {
	db := newTrackerDB(t)
	tr, _ := New(db, part())
	tr.MaxObserved = 8

	for i := 1; i <= 100; i++ {
		var a [32]byte
		a[0], a[1] = byte(i), byte(i>>8)
		tr.Observe(part(), uint64(i), a)
	}
	if got := tr.ObservedCount(); got != 8 {
		t.Fatalf("ObservedCount = %d, want the bound of 8", got)
	}

	// The oldest went first, so the newest are what is held.
	tr.mu.Lock()
	defer tr.mu.Unlock()
	for i := 93; i <= 100; i++ {
		var a [32]byte
		a[0], a[1] = byte(i), byte(i>>8)
		if _, ok := tr.observed[a]; !ok {
			t.Errorf("anchor of block %d was evicted before older ones", i)
		}
	}
}

// TestNew_RequiresAPartition — a tracker matches one partition's anchors.
func TestNew_RequiresAPartition(t *testing.T) {
	db := newTrackerDB(t)
	if _, err := New(db, nil); err == nil {
		t.Fatal("a tracker with no partition was accepted")
	}
}
