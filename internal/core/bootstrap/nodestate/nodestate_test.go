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
	}
	for _, c := range cases {
		if c.s.CanServeCurrent() != c.curOK {
			t.Errorf("%v.CanServeCurrent = %v, want %v", c.s, c.s.CanServeCurrent(), c.curOK)
		}
		if c.s.String() != c.stringForm {
			t.Errorf("%v.String = %q, want %q", c.s, c.s.String(), c.stringForm)
		}
	}
}

func TestMachine_PromotionIsOnceUntilDemoted(t *testing.T) {
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

	// A second promotion is not one: ACTIVE leaves only by demotion.
	if m.PromoteToActive(anchor, 200) {
		t.Fatal("repeat PromoteToActive should fail")
	}
}

// A node is ACTIVE only while it executes in agreement (executor.md, "Sync",
// steps 4-6; #4385). A re-sync after a root mismatch or a failed handoff sends
// the machine back to BOOTING: it refuses what BOOTING refuses, advertises no
// verified anchor, says so to every OnChange listener (the gauge is one), and
// is promoted again by the same promotion as the first time.
func TestMachine_DemotionReturnsToBootingUntilTheNextMatch(t *testing.T) {
	m := New(bvn0())
	var ads []Advertisement
	m.OnChange(func(ad Advertisement) { ads = append(ads, ad) })

	if m.Demote(10) {
		t.Fatal("a BOOTING machine has nothing to demote")
	}
	if len(ads) != 0 {
		t.Fatalf("a refused demotion fired %d change(s)", len(ads))
	}

	first := [32]byte{1}
	if !m.PromoteToActive(first, 100) {
		t.Fatal("PromoteToActive should succeed from BOOTING")
	}
	if !m.Demote(104) {
		t.Fatal("Demote should succeed from ACTIVE")
	}
	if m.State() != StateBooting {
		t.Fatalf("state = %v after Demote, want BOOTING", m.State())
	}
	if m.CanServeCurrent() {
		t.Fatal("a demoted machine still serves")
	}
	ad := m.Get()
	if ad.VerifiedAnchor != ([32]byte{}) {
		t.Fatalf("a demoted machine still advertises the anchor %x it no longer agrees with", ad.VerifiedAnchor)
	}
	if ad.SinceBlock != 104 {
		t.Fatalf("SinceBlock = %d, want the block it was demoted at, 104", ad.SinceBlock)
	}
	if err := ad.Validate(); err != nil {
		t.Fatalf("a demoted machine's advertisement is malformed: %v", err)
	}
	if len(ads) != 2 || ads[1].State != StateBooting {
		t.Fatalf("OnChange saw %+v, want the promotion then the demotion", ads)
	}

	if m.Demote(105) {
		t.Fatal("a second demotion is not one")
	}

	second := [32]byte{2}
	if !m.PromoteToActive(second, 120) {
		t.Fatal("a demoted machine is promoted again by the same promotion")
	}
	if ad := m.Get(); ad.State != StateActive || ad.VerifiedAnchor != second || ad.SinceBlock != 120 {
		t.Fatalf("re-promoted as %+v, want ACTIVE at 120 with the new anchor", ad)
	}
	if len(ads) != 3 {
		t.Fatalf("OnChange fired %d times, want 3", len(ads))
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
}

func TestParseState(t *testing.T) {
	cases := map[string]State{
		"BOOTING": StateBooting,
		"ACTIVE":  StateActive,

		// Retired (#4368), mapped: a WAITING root was never matched to an
		// anchored one, and COMPLETE was ACTIVE plus a backfill.
		"WAITING":  StateBooting,
		"COMPLETE": StateActive,
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
	m, err := Restore(bvn0(), StateActive, 42, anchor)
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

	if _, err := Restore(bvn0(), StateActive, 1, [32]byte{}); err == nil {
		t.Error("expected error for ACTIVE with zero anchor")
	}
	if _, err := Restore(bvn0(), StateUnknown, 0, [32]byte{}); err == nil {
		t.Error("expected error for StateUnknown")
	}
	if _, err := Restore(bvn0(), StateBooting, 0, [32]byte{}); err != nil {
		t.Errorf("BOOTING restore failed: %v", err)
	}
}

// TestRestore_RetiredStates pins how a record persisted before #4368 comes
// back: WAITING as BOOTING without its claimed root, COMPLETE as ACTIVE with
// its verified one. The numbers are the ones the retired states held.
func TestRestore_RetiredStates(t *testing.T) {
	anchor := [32]byte{0xaa}

	m, err := Restore(bvn0(), State(2), 42, anchor)
	if err != nil {
		t.Fatalf("WAITING restore failed: %v", err)
	}
	if ad := m.Get(); ad.State != StateBooting || ad.VerifiedAnchor != ([32]byte{}) {
		t.Errorf("WAITING restored as %v with anchor %x, want BOOTING with none", ad.State, ad.VerifiedAnchor)
	}

	m, err = Restore(bvn0(), State(4), 42, anchor)
	if err != nil {
		t.Fatalf("COMPLETE restore failed: %v", err)
	}
	if ad := m.Get(); ad.State != StateActive || ad.VerifiedAnchor != anchor || ad.SinceBlock != 42 {
		t.Errorf("COMPLETE restored as %+v, want ACTIVE at 42 with its anchor", ad)
	}

	if _, err := Restore(bvn0(), State(4), 42, [32]byte{}); err == nil {
		t.Error("expected error for COMPLETE with zero anchor")
	}
}

// TestNumber_ActiveIsTwo pins the gauge contract (#4364): retiring WAITING and
// COMPLETE did not renumber ACTIVE.
func TestNumber_ActiveIsTwo(t *testing.T) {
	if got := Number(StateBooting); got != 0 {
		t.Errorf("Number(BOOTING) = %v, want 0", got)
	}
	if got := Number(StateActive); got != 2 {
		t.Errorf("Number(ACTIVE) = %v, want 2", got)
	}
	if StateActive != 3 {
		t.Errorf("StateActive = %d, want 3: a persisted ACTIVE must still read as ACTIVE", StateActive)
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
		{"retired state", Advertisement{State: 4, Partition: bvn0(), VerifiedAnchor: [32]byte{1}}, false},
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
