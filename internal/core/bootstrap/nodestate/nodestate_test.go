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
		stringForm string
	}{
		{StateUnknown, false, "UNKNOWN"},
		{StateBooting, false, "BOOTING"},
		{StateActive, true, "ACTIVE"},
		// The two retired values. Nothing produces them; if one is read off
		// a wire it is not a state this node knows, and it does not serve.
		{2, false, "UNKNOWN"},
		{4, false, "UNKNOWN"},
	}
	for _, c := range cases {
		if c.s.CanServeCurrent() != c.curOK {
			t.Errorf("%v.CanServeCurrent = %v, want %v", c.s, c.s.CanServeCurrent(), c.curOK)
		}
		if c.s.String() != c.stringForm {
			t.Errorf("%v.String = %q, want %q", c.s, c.s.String(), c.stringForm)
		}
	}

	// The gauge's numbering is the harness's contract: ACTIVE is 2 (#4364).
	if got := Number(StateActive); got != 2 {
		t.Errorf("Number(ACTIVE) = %v, want 2", got)
	}
	if got := Number(StateBooting); got != 0 {
		t.Errorf("Number(BOOTING) = %v, want 0", got)
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

	// There is no second transition: BOOTING → ACTIVE is the whole machine,
	// so a promotion that is refused fires nothing.
	m.PromoteToActive([32]byte{0xcd}, 75)
	if got, want := atomic.LoadInt32(&fired), int32(1); got != want {
		t.Fatalf("fired = %d after a refused promotion, want %d", got, want)
	}
	if lastAd.SinceBlock != 50 {
		t.Fatalf("ad.SinceBlock = %d, want 50", lastAd.SinceBlock)
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
		{"retired WAITING value", Advertisement{State: 2, Partition: bvn0(), VerifiedAnchor: [32]byte{1}}, false},
		{"retired COMPLETE value", Advertisement{State: 4, Partition: bvn0(), VerifiedAnchor: [32]byte{1}}, false},
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
