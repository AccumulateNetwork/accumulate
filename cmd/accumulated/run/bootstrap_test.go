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
	if inst.bootArtifact != nil {
		t.Error("expected nil bootArtifact when no artifact present")
	}
}

func TestDetectBootstrapState_BootingArtifact(t *testing.T) {
	dir := t.TempDir()
	art := &bootpersist.Artifact{
		Network:           "devnet",
		PinnedGenesisHash: [32]byte{},
		PinBlock: bootpersist.PinBlock{
			Partition:       "Directory",
			MinorBlockIndex: 12345,
		},
		State: bootpersist.StateRecord{Current: "BOOTING"},
	}
	if err := bootpersist.Save(dir, art); err != nil {
		t.Fatal(err)
	}

	inst := instanceForTest(t, dir)
	if err := inst.detectBootstrapState(); err != nil {
		t.Fatalf("detectBootstrapState: %v", err)
	}
	if inst.bootMachine == nil {
		t.Fatal("expected non-nil bootMachine")
	}
	if got := inst.bootMachine.State(); got != nodestate.StateBooting {
		t.Errorf("State = %v, want BOOTING", got)
	}
	if inst.BootArtifact() == nil || inst.BootArtifact().PinBlock.MinorBlockIndex != 12345 {
		t.Error("BootArtifact pin block not preserved")
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
	if ad.State.String() != "booting" {
		// BootstrapState.String is generated from the lower-cased enum
		// member; just confirm it's the booting variant.
		t.Errorf("State = %v, want booting", ad.State)
	}
	if ad.BptRootMatched != ([32]byte{}) {
		t.Errorf("BOOTING advertisement should have zero BptRootMatched")
	}
}

func TestAdvertisementFromMachine_ActiveMachine(t *testing.T) {
	m := nodestate.New()
	root := [32]byte{0xab}
	if !m.PromoteToActive(root, 12345) {
		t.Fatal("PromoteToActive failed")
	}
	ad := advertisementFromMachine(m)
	if ad.SinceBlock != 12345 {
		t.Errorf("SinceBlock = %d, want 12345", ad.SinceBlock)
	}
	if ad.BptRootMatched != root {
		t.Errorf("BptRootMatched mismatch")
	}
}

func TestDetectBootstrapState_PinMismatchFails(t *testing.T) {
	dir := t.TempDir()
	// Use a network name that has a pinned hash in the binary. Since
	// pinned/pinned.go is empty by default, simulate the mismatch by
	// patching the package's table for the duration of the test.
	const network = "test-network-with-pin"
	hashA := [32]byte{0xaa}
	hashB := [32]byte{0xbb}

	// Inject a pin we control. Restore on test exit.
	t.Cleanup(pinned.RegisterForTest(network, hashA))

	art := &bootpersist.Artifact{
		Network:           network,
		PinnedGenesisHash: hashB, // different from registered pin
		State:             bootpersist.StateRecord{Current: "BOOTING"},
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
