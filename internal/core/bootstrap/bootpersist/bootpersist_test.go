// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bootpersist

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := &Artifact{
		Network:   "devnet",
		Partition: "Apollo",
		State: StateRecord{
			Current:        "ACTIVE",
			SinceBlock:     150,
			VerifiedAnchor: [32]byte{0x11, 0x22, 0x33},
			EnteredBooting: time.Unix(1700000000, 0).UTC(),
			EnteredActive:  time.Unix(1700001000, 0).UTC(),
		},
		Phases: Phases{
			SpinePullDone:      true,
			EnumerateDone:      false,
			EnumerateNextStart: [32]byte{0xab, 0xcd, 0xef},
		},
		ObservedAnchors: []ObservedAnchor{
			{Block: 100, Anchor: [32]byte{0x01}},
			{Block: 150, Anchor: [32]byte{0x11, 0x22, 0x33}},
		},
	}
	if err := Save(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Network != want.Network ||
		got.Partition != want.Partition ||
		got.State.Current != want.State.Current ||
		got.State.SinceBlock != want.State.SinceBlock ||
		got.State.VerifiedAnchor != want.State.VerifiedAnchor ||
		got.Phases.SpinePullDone != want.Phases.SpinePullDone ||
		got.Phases.EnumerateDone != want.Phases.EnumerateDone ||
		got.Phases.EnumerateNextStart != want.Phases.EnumerateNextStart ||
		len(got.ObservedAnchors) != len(want.ObservedAnchors) {
		t.Errorf("round-trip drift:\n  want=%+v\n  got=%+v", want, got)
	}
	for i := range want.ObservedAnchors {
		if got.ObservedAnchors[i] != want.ObservedAnchors[i] {
			t.Errorf("observed[%d] drift:\n  want=%+v\n  got=%+v",
				i, want.ObservedAnchors[i], got.ObservedAnchors[i])
		}
	}
}

func TestLoad_MissingFile_ReturnsNotExist(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want os.ErrNotExist chain", err)
	}
}

func TestSave_AtomicReplace(t *testing.T) {
	dir := t.TempDir()
	a := &Artifact{Partition: "first"}
	if err := Save(dir, a); err != nil {
		t.Fatal(err)
	}

	a.Partition = "second"
	if err := Save(dir, a); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Partition != "second" {
		t.Errorf("Save did not overwrite; Partition = %q, want second", got.Partition)
	}

	// .tmp shouldn't be left behind on success.
	if _, err := os.Stat(filepath.Join(dir, FileName+".tmp")); !os.IsNotExist(err) {
		t.Errorf(".tmp file leaked after successful Save")
	}
}

func TestLoad_FormatMajorMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	bad := []byte(`{"formatMajor": 99, "formatMinor": 0}`)
	if err := os.WriteFile(path, bad, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if !errors.Is(err, ErrFormatMajor) {
		t.Errorf("err = %v, want ErrFormatMajor chain", err)
	}
}

func TestSave_ForcesFormatVersion(t *testing.T) {
	dir := t.TempDir()
	// Caller passes nonsense versions; Save must overwrite them.
	a := &Artifact{FormatMajor: 999, FormatMinor: 999}
	if err := Save(dir, a); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.FormatMajor != FormatMajor {
		t.Errorf("FormatMajor not forced on Save; got %d, want %d", got.FormatMajor, FormatMajor)
	}
}

// TestSave_AfterCrashTmpLeftBehind — a .tmp file from a prior crashed
// run should not corrupt subsequent saves.
func TestSave_AfterCrashTmpLeftBehind(t *testing.T) {
	dir := t.TempDir()
	// Plant a stale .tmp.
	stale := filepath.Join(dir, FileName+".tmp")
	if err := os.WriteFile(stale, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Save(dir, &Artifact{Partition: "ok"}); err != nil {
		t.Fatalf("Save with stale .tmp present: %v", err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Partition != "ok" {
		t.Errorf("Load got unexpected partition %q", got.Partition)
	}
}
