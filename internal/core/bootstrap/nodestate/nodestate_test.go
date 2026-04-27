// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package nodestate

import (
	"sync/atomic"
	"testing"
)

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
	m := New()
	if got, want := m.State(), StateBooting; got != want {
		t.Fatalf("initial state = %v, want %v", got, want)
	}

	root := [32]byte{1, 2, 3}
	if !m.PromoteToActive(root, 100) {
		t.Fatal("PromoteToActive should succeed from BOOTING")
	}
	if m.State() != StateActive {
		t.Fatalf("state = %v, want ACTIVE", m.State())
	}
	if m.Get().BptRootMatched != root {
		t.Fatal("bpt root not recorded")
	}

	// Cannot regress.
	if m.PromoteToActive(root, 200) {
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

func TestMachine_Active_RequiresNonZeroRoot(t *testing.T) {
	m := New()
	if m.PromoteToActive([32]byte{}, 100) {
		t.Fatal("zero root should be rejected")
	}
	if m.State() != StateBooting {
		t.Fatalf("state = %v, want BOOTING after rejected promotion", m.State())
	}
}

func TestMachine_OnChange(t *testing.T) {
	m := New()
	var fired int32
	var lastAd Advertisement
	m.OnChange(func(ad Advertisement) {
		atomic.AddInt32(&fired, 1)
		lastAd = ad
	})

	root := [32]byte{0xab}
	m.PromoteToActive(root, 50)
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

func TestAdvertisement_Validate(t *testing.T) {
	cases := []struct {
		name   string
		ad     Advertisement
		wantOK bool
	}{
		{"booting valid", Advertisement{State: StateBooting}, true},
		{"active no root", Advertisement{State: StateActive}, false},
		{"active with root", Advertisement{State: StateActive, BptRootMatched: [32]byte{1}}, true},
		{"complete with root", Advertisement{State: StateComplete, BptRootMatched: [32]byte{1}}, true},
		{"unknown state", Advertisement{State: StateUnknown}, false},
		{"out-of-range state", Advertisement{State: 99}, false},
	}
	for _, c := range cases {
		err := c.ad.Validate()
		if (err == nil) != c.wantOK {
			t.Errorf("%s: Validate err = %v, wantOK = %v", c.name, err, c.wantOK)
		}
	}
}
