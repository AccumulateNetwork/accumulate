// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package persist

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestCheckpoint_NewCheckpoint(t *testing.T) {
	lastCommitted := map[string]types.Round{
		"author1": 5,
		"author2": 3,
	}

	cp := NewCheckpoint("test-partition", 10, 1, 8, lastCommitted)

	if cp.Version != 1 {
		t.Errorf("expected version 1, got %d", cp.Version)
	}
	if cp.Partition != "test-partition" {
		t.Errorf("expected partition 'test-partition', got %s", cp.Partition)
	}
	if cp.CurrentRound != 10 {
		t.Errorf("expected current round 10, got %d", cp.CurrentRound)
	}
	if cp.CurrentEpoch != 1 {
		t.Errorf("expected current epoch 1, got %d", cp.CurrentEpoch)
	}
	if cp.LastCommitRound != 8 {
		t.Errorf("expected last commit round 8, got %d", cp.LastCommitRound)
	}
	if len(cp.LastCommitted) != 2 {
		t.Errorf("expected 2 last committed entries, got %d", len(cp.LastCommitted))
	}

	// Verify timestamp is recent
	if time.Since(cp.Timestamp) > time.Minute {
		t.Error("timestamp should be recent")
	}
}

func TestCheckpoint_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cp      *Checkpoint
		wantErr bool
	}{
		{
			name: "valid checkpoint",
			cp: &Checkpoint{
				Version:         1,
				Partition:       "test",
				CurrentRound:    10,
				LastCommitRound: 8,
			},
			wantErr: false,
		},
		{
			name: "invalid version",
			cp: &Checkpoint{
				Version:   0,
				Partition: "test",
			},
			wantErr: true,
		},
		{
			name: "empty partition",
			cp: &Checkpoint{
				Version:   1,
				Partition: "",
			},
			wantErr: true,
		},
		{
			name: "last commit exceeds current",
			cp: &Checkpoint{
				Version:         1,
				Partition:       "test",
				CurrentRound:    5,
				LastCommitRound: 10,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cp.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStore_SaveAndLoad(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "checkpoint_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	// Create checkpoint
	lastCommitted := map[string]types.Round{
		"author1": 5,
		"author2": 3,
	}
	cp := NewCheckpoint("test-partition", 10, 1, 8, lastCommitted)

	// Save
	if err := store.Save(cp); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if !store.Exists() {
		t.Error("checkpoint file should exist")
	}

	// Load
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify loaded data
	if loaded.Partition != cp.Partition {
		t.Errorf("partition mismatch: got %s, want %s", loaded.Partition, cp.Partition)
	}
	if loaded.CurrentRound != cp.CurrentRound {
		t.Errorf("current round mismatch: got %d, want %d", loaded.CurrentRound, cp.CurrentRound)
	}
	if loaded.LastCommitRound != cp.LastCommitRound {
		t.Errorf("last commit round mismatch: got %d, want %d", loaded.LastCommitRound, cp.LastCommitRound)
	}
	if len(loaded.LastCommitted) != len(cp.LastCommitted) {
		t.Errorf("last committed length mismatch: got %d, want %d", len(loaded.LastCommitted), len(cp.LastCommitted))
	}
}

func TestStore_LoadNotExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "checkpoint_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	_, err = store.Load()
	if err != ErrNoCheckpoint {
		t.Errorf("expected ErrNoCheckpoint, got %v", err)
	}
}

func TestStore_Delete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "checkpoint_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	// Create and save checkpoint
	cp := NewCheckpoint("test", 10, 1, 8, nil)
	if err := store.Save(cp); err != nil {
		t.Fatal(err)
	}

	// Verify exists
	if !store.Exists() {
		t.Error("checkpoint should exist")
	}

	// Delete
	if err := store.Delete(); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	if store.Exists() {
		t.Error("checkpoint should not exist after delete")
	}
}

func TestStore_AtomicWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "checkpoint_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	// Save checkpoint
	cp := NewCheckpoint("test", 10, 1, 8, nil)
	if err := store.Save(cp); err != nil {
		t.Fatal(err)
	}

	// Verify no temp file remains
	tmpFile := filepath.Join(tmpDir, CheckpointFileName+".tmp")
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("temp file should not exist after save")
	}
}

func TestStateSnapshot_ToCheckpoint(t *testing.T) {
	snapshot := &StateSnapshot{
		Partition:       "test-partition",
		CurrentRound:    10,
		CurrentEpoch:    1,
		LastCommitRound: 8,
		LastCommitted: map[string]types.Round{
			"author1": 5,
		},
	}

	cp := snapshot.ToCheckpoint()

	if cp.Partition != snapshot.Partition {
		t.Errorf("partition mismatch")
	}
	if cp.CurrentRound != snapshot.CurrentRound {
		t.Errorf("current round mismatch")
	}
	if cp.LastCommitRound != snapshot.LastCommitRound {
		t.Errorf("last commit round mismatch")
	}
}
