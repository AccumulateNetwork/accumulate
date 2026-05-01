// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/bootpersist"
)

func writeTestArtifact(t *testing.T, dir string, art *bootpersist.Artifact) {
	t.Helper()
	if err := bootpersist.Save(dir, art); err != nil {
		t.Fatalf("save artifact in %s: %v", dir, err)
	}
}

// TestCollectBootstrapStates_Empty asserts an Instance with no
// artifacts under rootDir returns an empty Partitions map (not nil).
func TestCollectBootstrapStates_Empty(t *testing.T) {
	inst := &Instance{
		rootDir: t.TempDir(),
		logger:  slog.Default(),
	}
	v := inst.collectBootstrapStates()
	if v.Partitions == nil {
		t.Fatal("Partitions should be non-nil even when empty")
	}
	if len(v.Partitions) != 0 {
		t.Fatalf("expected empty, got %d entries", len(v.Partitions))
	}
}

// TestCollectBootstrapStates_OneSubdir asserts artifacts in a
// subdirectory (the dual-node layout) are picked up.
func TestCollectBootstrapStates_OneSubdir(t *testing.T) {
	root := t.TempDir()
	dnnDir := filepath.Join(root, "dnn")

	writeTestArtifact(t, dnnDir, &bootpersist.Artifact{
		Network:   "MainNet",
		Partition: "Directory",
		State: bootpersist.StateRecord{
			Current:        "ACTIVE",
			SinceBlock:     24554165,
			VerifiedAnchor: [32]byte{0xf5, 0x5b},
			EnteredActive:  time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	inst := &Instance{rootDir: root, logger: slog.Default()}
	v := inst.collectBootstrapStates()
	got, ok := v.Partitions["Directory"]
	if !ok {
		t.Fatalf("Directory entry missing; got %+v", v.Partitions)
	}
	if got.State != "ACTIVE" {
		t.Errorf("State = %q, want ACTIVE", got.State)
	}
	if got.SinceBlock != 24554165 {
		t.Errorf("SinceBlock = %d", got.SinceBlock)
	}
	if got.VerifiedAnchor[:4] != "f55b" {
		t.Errorf("VerifiedAnchor = %q, want prefix f55b", got.VerifiedAnchor)
	}
}

// TestCollectBootstrapStates_DualPartition asserts both dnn/ and bvnn/
// artifacts are collected and keyed by their declared Partition.
func TestCollectBootstrapStates_DualPartition(t *testing.T) {
	root := t.TempDir()
	writeTestArtifact(t, filepath.Join(root, "dnn"), &bootpersist.Artifact{
		Partition: "Directory",
		State:     bootpersist.StateRecord{Current: "ACTIVE"},
	})
	writeTestArtifact(t, filepath.Join(root, "bvnn"), &bootpersist.Artifact{
		Partition: "Cyclops",
		State:     bootpersist.StateRecord{Current: "WAITING"},
	})

	inst := &Instance{rootDir: root, logger: slog.Default()}
	v := inst.collectBootstrapStates()
	if len(v.Partitions) != 2 {
		t.Fatalf("expected 2, got %d: %+v", len(v.Partitions), v.Partitions)
	}
	if v.Partitions["Directory"].State != "ACTIVE" {
		t.Error("Directory state mismatch")
	}
	if v.Partitions["Cyclops"].State != "WAITING" {
		t.Error("Cyclops state mismatch")
	}
}

// TestCollectBootstrapStates_RootLevel asserts an artifact directly in
// rootDir (single-partition layout) is also picked up.
func TestCollectBootstrapStates_RootLevel(t *testing.T) {
	root := t.TempDir()
	writeTestArtifact(t, root, &bootpersist.Artifact{
		Partition: "Directory",
		State:     bootpersist.StateRecord{Current: "BOOTING"},
	})

	inst := &Instance{rootDir: root, logger: slog.Default()}
	v := inst.collectBootstrapStates()
	if v.Partitions["Directory"].State != "BOOTING" {
		t.Errorf("expected BOOTING in root layout; got %+v", v.Partitions)
	}
}
