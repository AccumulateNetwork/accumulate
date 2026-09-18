// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package nodestate

import (
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// bvn0 is the partition the machines under test belong to. A machine is per
// partition: a node serves two, and every block number it advertises is a
// block of one of them (#4205).
func bvn0() *url.URL { return protocol.PartitionUrl("BVN0") }

func TestState_Capabilities(t *testing.T) {
	cases := []struct {
		s          State
		curOK      bool
		historyOK  bool
		stringForm string
	}{
		{StateUnknown, false, false, "UNKNOWN"},
		{StateBooting, false, false, "BOOTING"},
		{StateActive, true, false, "ACTIVE"},
		{StateComplete, true, true, "COMPLETE"},
	}
	for _, c := range cases {
		if c.s.CanServeCurrent() != c.curOK {
			t.Errorf("%v.CanServeCurrent = %v, want %v", c.s, c.s.CanServeCurrent(), c.curOK)
		}
		if c.s.CanServeHistory() != c.historyOK {
			t.Errorf("%v.CanServeHistory = %v, want %v", c.s, c.s.CanServeHistory(), c.historyOK)
		}
		if c.s.String() != c.stringForm {
			t.Errorf("%v.String = %q, want %q", c.s, c.s.String(), c.stringForm)
		}
	}
}

func TestMachine_ForwardOnlyTransitions(t *testing.T) {
	m := New(bvn0())
	if got, want := m.State(), StateBooting; got != want {
		t.Fatalf("initial state = %v, want %v", got, want)
	}

	anchor := [32]byte{1, 2, 3}
	if !m.PromoteToActive(anchor, 100) {
		t.Fatal("PromoteToActive should succeed from BOOTING")
	}
	if m.State() != StateActive {
		t.Fatalf("state = %v, want ACTIVE", m.State())
	}
	if m.Get().VerifiedAnchor != anchor {
		t.Fatal("anchor not recorded")
	}

	// Cannot regress.
	if m.PromoteToActive(anchor, 200) {
		t.Fatal("repeat PromoteToActive should fail")
	}

	if !m.PromoteToComplete(0, 200) {
		t.Fatal("PromoteToComplete should succeed from ACTIVE")
	}
	if m.State() != StateComplete {
		t.Fatalf("state = %v, want COMPLETE", m.State())
	}

	// Cannot regress further.
	if m.PromoteToComplete(0, 300) {
		t.Fatal("repeat PromoteToComplete should fail")
	}
}

func TestMachine_Active_RequiresNonZeroAnchor(t *testing.T) {
	m := New(bvn0())
	if m.PromoteToActive([32]byte{}, 100) {
		t.Fatal("zero anchor should be rejected")
	}
	if m.State() != StateBooting {
		t.Fatalf("state = %v, want BOOTING after rejected promotion", m.State())
	}
}

func TestMachine_OnChange(t *testing.T) {
	m := New(bvn0())
	var fired int32
	var lastAd Advertisement
	m.OnChange(func(ad Advertisement) {
		atomic.AddInt32(&fired, 1)
		lastAd = ad
	})

	anchor := [32]byte{0xab}
	m.PromoteToActive(anchor, 50)
	if got, want := atomic.LoadInt32(&fired), int32(1); got != want {
		t.Fatalf("fired = %d, want %d", got, want)
	}
	if lastAd.State != StateActive {
		t.Fatalf("ad.State = %v, want ACTIVE", lastAd.State)
	}

	m.PromoteToComplete(1000, 75)
	if got, want := atomic.LoadInt32(&fired), int32(2); got != want {
		t.Fatalf("fired = %d, want %d", got, want)
	}
	if lastAd.State != StateComplete {
		t.Fatalf("ad.State = %v, want COMPLETE", lastAd.State)
	}
	if lastAd.HistoryDepth != 1000 {
		t.Fatalf("ad.HistoryDepth = %d, want 1000", lastAd.HistoryDepth)
	}
}

func TestParseState(t *testing.T) {
	cases := map[string]State{
		"BOOTING":  StateBooting,
		"ACTIVE":   StateActive,
		"COMPLETE": StateComplete,
	}
	for s, want := range cases {
		got, err := ParseState(s)
		if err != nil {
			t.Errorf("ParseState(%q) err = %v", s, err)
		}
		if got != want {
			t.Errorf("ParseState(%q) = %v, want %v", s, got, want)
		}
	}
	if _, err := ParseState("nonsense"); err == nil {
		t.Error("expected error for unknown state")
	}
}

func TestRestore(t *testing.T) {
	anchor := [32]byte{0xaa}
	m, err := Restore(bvn0(), StateActive, 42, anchor, 0)
	if err != nil {
		t.Fatal(err)
	}
	if m.State() != StateActive {
		t.Errorf("State = %v, want ACTIVE", m.State())
	}
	ad := m.Get()
	if ad.SinceBlock != 42 || ad.VerifiedAnchor != anchor {
		t.Errorf("Get = %+v, missing restored fields", ad)
	}

	if _, err := Restore(bvn0(), StateActive, 1, [32]byte{}, 0); err == nil {
		t.Error("expected error for ACTIVE with zero anchor")
	}
	if _, err := Restore(bvn0(), StateUnknown, 0, [32]byte{}, 0); err == nil {
		t.Error("expected error for StateUnknown")
	}
	if _, err := Restore(bvn0(), StateBooting, 0, [32]byte{}, 0); err != nil {
		t.Errorf("BOOTING restore failed: %v", err)
	}
}

func TestAdvertisement_Validate(t *testing.T) {
	cases := []struct {
		name   string
		ad     Advertisement
		wantOK bool
	}{
		{"booting valid", Advertisement{State: StateBooting, Partition: bvn0()}, true},
		{"active no anchor", Advertisement{State: StateActive, Partition: bvn0()}, false},
		{"active with anchor", Advertisement{State: StateActive, Partition: bvn0(), VerifiedAnchor: [32]byte{1}}, true},
		{"complete with anchor", Advertisement{State: StateComplete, Partition: bvn0(), VerifiedAnchor: [32]byte{1}}, true},
		{"unknown state", Advertisement{State: StateUnknown, Partition: bvn0()}, false},
		{"out-of-range state", Advertisement{State: 99, Partition: bvn0()}, false},
		{"no partition", Advertisement{State: StateBooting}, false},
	}
	for _, c := range cases {
		err := c.ad.Validate()
		if (err == nil) != c.wantOK {
			t.Errorf("%s: Validate err = %v, wantOK = %v", c.name, err, c.wantOK)
		}
	}
}

func TestHeartbeat_AdvancesLastUpdated(t *testing.T) {
	m := New(bvn0())
	t0 := m.Get().LastUpdated
	time.Sleep(2 * time.Millisecond)
	t1 := m.Heartbeat().LastUpdated
	if !t1.After(t0) {
		t.Errorf("Heartbeat did not advance LastUpdated: t0=%v t1=%v", t0, t1)
	}
}
