// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tracker

import (
	"context"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestCheck_IgnoresAMachineAlreadyActive — the states are two (#4368), so
// BOOTING is the one state Check promotes out of. A machine already ACTIVE
// has nothing left to do, and Check must say so rather than promote again.
//
// This replaces TestCheck_PromotesFromWaiting, which pinned the WAITING step
// nothing ever took: nodestate documented BOOTING → WAITING → ACTIVE and no
// production caller ever moved a machine to WAITING (callers_test.go).
func TestCheck_IgnoresAMachineAlreadyActive(t *testing.T) {
	db := newTrackerDB(t)
	root := fillN(t, db, 5)

	m := machine()
	tr, err := New(db, m)
	if err != nil {
		t.Fatal(err)
	}

	tr.Observe(part(), 42, root)
	promoted, err := tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !promoted {
		t.Fatal("a BOOTING node was never promoted, although the anchored root matched")
	}
	if m.State() != nodestate.StateActive {
		t.Fatalf("state = %v, want ACTIVE", m.State())
	}
	if got := m.Get().SinceBlock; got != 42 {
		t.Fatalf("SinceBlock = %d, want 42", got)
	}

	// A second look promotes nothing and moves nothing.
	tr.Observe(part(), 99, root)
	promoted, err = tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if promoted {
		t.Fatal("an ACTIVE node was promoted again")
	}
	if got := m.Get().SinceBlock; got != 42 {
		t.Fatalf("SinceBlock moved to %d after the machine was already ACTIVE", got)
	}
}

// TestObserve_IgnoresAnotherPartition — the caller feeds the tracker every
// anchor the Directory executes, and block numbers collide across partitions
// (#4205). An anchor for a partition this tracker is not watching is not an
// observation of it.
func TestObserve_IgnoresAnotherPartition(t *testing.T) {
	db := newTrackerDB(t)
	root := fillN(t, db, 5)

	m := machine()
	tr, _ := New(db, m)
	tr.Observe(protocol.DnUrl(), 42, root)
	if got := tr.ObservedCount(); got != 0 {
		t.Fatalf("ObservedCount = %d after another partition's anchor", got)
	}
	promoted, err := tr.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if promoted {
		t.Fatal("another partition's anchor promoted the node")
	}

	tr.Observe(part(), 42, root)
	if promoted, err = tr.Check(context.Background()); err != nil || !promoted {
		t.Fatalf("the partition's own anchor did not promote: %v %v", promoted, err)
	}
}

// TestObserve_IsBounded — a join runs for as long as it takes to catch up, and
// an unbounded observation set is an unbounded heap on a node that never
// converges.
func TestObserve_IsBounded(t *testing.T) {
	db := newTrackerDB(t)
	tr, _ := New(db, machine())
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
	if _, err := New(db, nodestate.New(nil)); err == nil {
		t.Fatal("a machine with no partition was accepted")
	}
}
