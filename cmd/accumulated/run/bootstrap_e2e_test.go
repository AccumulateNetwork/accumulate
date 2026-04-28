// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
)

// TestBootstrapLifecycle_E2E exercises the full bootstrap → resume →
// advertise cycle without requiring a live network or Docker stack
// (capstone for issue #3976; the Docker-based E2E lives in
// test/docker/bootstrap-launcher-e2e.sh).
//
// Steps:
//
//  1. A bootstrap pipeline writes an artifact to a data dir.
//  2. accumulated run starts in that data dir and detects the artifact.
//  3. The reconstructed nodestate Machine reflects the recorded state.
//  4. The advertisement projection produces a wire-format payload that
//     a peer would receive in NodeInfo.
//
// This is the "happy path" that ties together #3965 (persistence),
// #3970 (state machine), #3981 (run handoff), and #3982
// (advertisement). Failure here means one of those slices regressed.
func TestBootstrapLifecycle_E2E(t *testing.T) {
	dir := t.TempDir()

	// Step 1: simulate the pipeline writing an artifact at the end of
	// a bootstrap run.
	now := time.Now().UTC().Truncate(time.Second)
	art := &bootpersist.Artifact{
		Network:           "devnet",
		PinnedGenesisHash: [32]byte{},
		PinBlock: bootpersist.PinBlock{
			Partition:       "Directory",
			MinorBlockIndex: 100,
			MajorBlockIndex: 5,
		},
		State: bootpersist.StateRecord{
			Current:        "BOOTING",
			EnteredBooting: now,
		},
	}
	if err := bootpersist.Save(dir, art); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Step 2: accumulated run startup detects the artifact.
	inst := instanceForTest(t, dir)
	if err := inst.detectBootstrapState(); err != nil {
		t.Fatalf("detectBootstrapState: %v", err)
	}
	if inst.BootMachine() == nil {
		t.Fatal("expected non-nil BootMachine after detection")
	}

	// Step 3: machine state matches the artifact.
	if got, want := inst.BootMachine().State().String(), "BOOTING"; got != want {
		t.Errorf("machine state = %q, want %q", got, want)
	}
	if got, want := inst.BootArtifact().PinBlock.MinorBlockIndex, uint64(100); got != want {
		t.Errorf("artifact pin block = %d, want %d", got, want)
	}

	// Step 4: advertisement projection produces a wire payload peers
	// would consume via NodeInfo.
	ad := advertisementFromMachine(inst.BootMachine())
	if ad == nil {
		t.Fatal("expected non-nil advertisement")
	}
	// BOOTING advertisements carry zero BptRootMatched — peers use
	// this to know the node can't yet serve current-state queries.
	if ad.BptRootMatched != ([32]byte{}) {
		t.Error("BOOTING advertisement should have zero BptRootMatched")
	}
	if ad.LastUpdated.IsZero() {
		t.Error("LastUpdated should be set on advertisement")
	}
}

// TestBootstrapLifecycle_PromoteAndAdvertise ties the Machine
// transitions to the wire advertisement, simulating what happens
// when the hydrator (#3964) finishes filling state and promotes the
// node to ACTIVE. The advertisement must carry the matched BPT root
// so peers can spot-check it before routing reads to this node.
func TestBootstrapLifecycle_PromoteAndAdvertise(t *testing.T) {
	dir := t.TempDir()
	art := &bootpersist.Artifact{
		Network: "devnet",
		PinBlock: bootpersist.PinBlock{
			Partition:       "Directory",
			MinorBlockIndex: 50,
		},
		State: bootpersist.StateRecord{Current: "BOOTING"},
	}
	if err := bootpersist.Save(dir, art); err != nil {
		t.Fatal(err)
	}

	inst := instanceForTest(t, dir)
	if err := inst.detectBootstrapState(); err != nil {
		t.Fatal(err)
	}

	root := [32]byte{0xab, 0xcd}
	if !inst.BootMachine().PromoteToActive(root, 200) {
		t.Fatal("PromoteToActive failed")
	}

	ad := advertisementFromMachine(inst.BootMachine())
	if ad.BptRootMatched != root {
		t.Errorf("BptRootMatched not propagated: got %x, want %x", ad.BptRootMatched, root)
	}
	if ad.SinceBlock != 200 {
		t.Errorf("SinceBlock = %d, want 200", ad.SinceBlock)
	}
}
