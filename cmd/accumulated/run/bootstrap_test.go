// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"io"
	"log/slog"
	"strings"
	"testing"

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
		Network:                "devnet",
		Partition:              "Directory",
		PinnedValidatorSetHash: [32]byte{},
		PinnedHeight:           100,
		State:                  bootpersist.StateRecord{Current: "BOOTING"},
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
	if inst.BootArtifact() == nil || inst.BootArtifact().PinnedHeight != 100 {
		t.Error("BootArtifact pinnedHeight not preserved")
	}
}

func TestDetectBootstrapState_ActiveArtifact(t *testing.T) {
	dir := t.TempDir()
	anchor := [32]byte{0xab, 0xcd}
	art := &bootpersist.Artifact{
		Network:        "devnet",
		Partition:      "Directory",
		PinnedHeight:   100,
		VerifiedAnchor: anchor,
		VerifiedHeight: 150,
		State:          bootpersist.StateRecord{Current: "ACTIVE"},
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
	ad := inst.BootMachine().Get()
	if ad.VerifiedAnchor != anchor {
		t.Errorf("VerifiedAnchor not propagated: got %x, want %x", ad.VerifiedAnchor, anchor)
	}
	if ad.SinceBlock != 150 {
		t.Errorf("SinceBlock = %d, want 150 (VerifiedHeight)", ad.SinceBlock)
	}
}

func TestDetectBootstrapState_PinMismatchFails(t *testing.T) {
	dir := t.TempDir()
	const network = "test-network-with-pin"
	expected := pinned.Pin{ValidatorSetHash: [32]byte{0xaa}, PinnedHeight: 100}
	t.Cleanup(pinned.RegisterForTest(network, expected))

	art := &bootpersist.Artifact{
		Network:                network,
		Partition:              "Directory",
		PinnedValidatorSetHash: [32]byte{0xbb}, // doesn't match registered
		State:                  bootpersist.StateRecord{Current: "BOOTING"},
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
		ValidatorSetHash: hash,
		PinnedHeight:     50,
	}))

	art := &bootpersist.Artifact{
		Network:                network,
		Partition:              "Directory",
		PinnedValidatorSetHash: hash,
		State:                  bootpersist.StateRecord{Current: "BOOTING"},
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
