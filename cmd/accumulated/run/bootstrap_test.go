// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pinned"
)

// instanceForTest builds a minimal Instance suitable for exercising
// detectBootstrapState in isolation. The full New() path requires a
// telemetry/p2p/uuid stack we don't need here.
func instanceForTest(t *testing.T, dir string) *Instance {
	t.Helper()
	return &Instance{
		rootDir: dir,
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestDetectBootstrapState_NoArtifact(t *testing.T) {
	inst := instanceForTest(t, t.TempDir())
	if err := inst.detectBootstrapState(); err != nil {
		t.Fatalf("detectBootstrapState: %v", err)
	}
	if inst.bootMachine != nil {
		t.Error("expected nil bootMachine when no artifact present")
	}
}

func TestDetectBootstrapState_BootingArtifact(t *testing.T) {
	dir := t.TempDir()
	art := &bootpersist.Artifact{
		Network:                  "devnet",
		BVN:                      "Apollo",
		DNGenesisStateTreeAnchor: [32]byte{},
		State:                    bootpersist.StateRecord{Current: "BOOTING"},
	}
	if err := bootpersist.Save(dir, art); err != nil {
		t.Fatal(err)
	}

	inst := instanceForTest(t, dir)
	if err := inst.detectBootstrapState(); err != nil {
		t.Fatalf("detectBootstrapState: %v", err)
	}
	if inst.BootMachine() == nil {
		t.Fatal("expected non-nil bootMachine")
	}
	if got := inst.BootMachine().State(); got != nodestate.StateBooting {
		t.Errorf("State = %v, want BOOTING", got)
	}
	if inst.BootArtifact() == nil || inst.BootArtifact().BVN != "Apollo" {
		t.Error("BootArtifact BVN not preserved")
	}
}

func TestDetectBootstrapState_ActiveArtifact(t *testing.T) {
	dir := t.TempDir()
	dnAnchor := [32]byte{0xab, 0xcd}
	bvnAnchor := [32]byte{0xee, 0xff}
	art := &bootpersist.Artifact{
		Network:               "devnet",
		BVN:                   "Apollo",
		DNVerifiedAnchor:      dnAnchor,
		DNVerifiedMajorBlock:  150,
		BVNVerifiedAnchor:     bvnAnchor,
		BVNVerifiedMajorBlock: 150,
		State:                 bootpersist.StateRecord{Current: "ACTIVE"},
	}
	if err := bootpersist.Save(dir, art); err != nil {
		t.Fatal(err)
	}

	inst := instanceForTest(t, dir)
	if err := inst.detectBootstrapState(); err != nil {
		t.Fatalf("detectBootstrapState: %v", err)
	}
	if got := inst.BootMachine().State(); got != nodestate.StateActive {
		t.Errorf("State = %v, want ACTIVE", got)
	}
	// Advertisement carries the DN-side anchor (network-shared).
	ad := inst.BootMachine().Get()
	if ad.VerifiedAnchor != dnAnchor {
		t.Errorf("Advertisement VerifiedAnchor = %x, want DN anchor %x", ad.VerifiedAnchor, dnAnchor)
	}
	if ad.SinceBlock != 150 {
		t.Errorf("SinceBlock = %d, want 150 (DNVerifiedMajorBlock)", ad.SinceBlock)
	}
}

func TestDetectBootstrapState_PinMismatchFails(t *testing.T) {
	dir := t.TempDir()
	const network = "test-network-with-pin"
	expected := pinned.Pin{DNGenesisStateTreeAnchor: [32]byte{0xaa}}
	t.Cleanup(pinned.RegisterForTest(network, expected))

	art := &bootpersist.Artifact{
		Network:                  network,
		BVN:                      "Apollo",
		DNGenesisStateTreeAnchor: [32]byte{0xbb}, // doesn't match registered
		State:                    bootpersist.StateRecord{Current: "BOOTING"},
	}
	if err := bootpersist.Save(dir, art); err != nil {
		t.Fatal(err)
	}

	inst := instanceForTest(t, dir)
	err := inst.detectBootstrapState()
	if err == nil || !strings.Contains(err.Error(), "pin mismatch") {
		t.Fatalf("expected pin mismatch error, got %v", err)
	}
}

func TestStartHeartbeat_AdvancesLastUpdated(t *testing.T) {
	m := nodestate.New()
	t0 := m.Get().LastUpdated

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startHeartbeat(ctx, m, 5*time.Millisecond)

	// Wait long enough for at least one tick.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.Get().LastUpdated.After(t0) {
			return // success
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Errorf("Heartbeat did not advance LastUpdated within timeout")
}

func TestStartHeartbeat_NilMachineNoOp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Must not panic.
	startHeartbeat(ctx, nil, 1*time.Millisecond)
}

func TestStartHeartbeat_StopsOnContextCancel(t *testing.T) {
	m := nodestate.New()
	ctx, cancel := context.WithCancel(context.Background())
	startHeartbeat(ctx, m, 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	t1 := m.Get().LastUpdated
	cancel()
	time.Sleep(20 * time.Millisecond)
	t2 := m.Get().LastUpdated
	// After cancel, no further heartbeats. We can't assert exact
	// equality because there's a tick in flight; instead, take a
	// late reading and require the gap between t2 and a still-later
	// reading to be zero (the goroutine should have stopped).
	time.Sleep(20 * time.Millisecond)
	t3 := m.Get().LastUpdated
	if !t3.Equal(t2) {
		t.Errorf("heartbeat fired after context cancel: t2=%v t3=%v", t2, t3)
	}
	_ = t1
}

func TestAdvertisementFromMachine_NilSafe(t *testing.T) {
	if got := advertisementFromMachine(nil); got != nil {
		t.Errorf("expected nil for nil machine, got %+v", got)
	}
}

func TestAdvertisementFromMachine_BootingMachine(t *testing.T) {
	m := nodestate.New() // starts in BOOTING
	ad := advertisementFromMachine(m)
	if ad == nil {
		t.Fatal("expected non-nil advertisement")
	}
	if ad.VerifiedAnchor != ([32]byte{}) {
		t.Errorf("BOOTING advertisement should have zero VerifiedAnchor")
	}
	if ad.LastUpdated.IsZero() {
		t.Error("LastUpdated should be set on advertisement")
	}
}

func TestAdvertisementFromMachine_ActiveMachine(t *testing.T) {
	m := nodestate.New()
	anchor := [32]byte{0xab}
	if !m.PromoteToActive(anchor, 12345) {
		t.Fatal("PromoteToActive failed")
	}
	ad := advertisementFromMachine(m)
	if ad.SinceBlock != 12345 {
		t.Errorf("SinceBlock = %d, want 12345", ad.SinceBlock)
	}
	if ad.VerifiedAnchor != anchor {
		t.Errorf("VerifiedAnchor mismatch")
	}
}

func TestDetectBootstrapState_PinMatchSucceeds(t *testing.T) {
	dir := t.TempDir()
	const network = "test-network-with-pin-match"
	hash := [32]byte{0xcc, 0xdd}
	t.Cleanup(pinned.RegisterForTest(network, pinned.Pin{
		DNGenesisStateTreeAnchor: hash,
	}))

	art := &bootpersist.Artifact{
		Network:                  network,
		BVN:                      "Apollo",
		DNGenesisStateTreeAnchor: hash,
		State:                    bootpersist.StateRecord{Current: "BOOTING"},
	}
	if err := bootpersist.Save(dir, art); err != nil {
		t.Fatal(err)
	}

	inst := instanceForTest(t, dir)
	if err := inst.detectBootstrapState(); err != nil {
		t.Fatalf("expected pin match to succeed, got %v", err)
	}
	if inst.BootMachine() == nil {
		t.Fatal("expected non-nil bootMachine on pin match")
	}
}
