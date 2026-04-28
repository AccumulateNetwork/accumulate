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
		Network:                  "devnet",
		BVN:                      "Apollo",
		DNGenesisStateTreeAnchor: [32]byte{0xaa, 0xbb},
		DNVerifiedAnchor:         [32]byte{0x11, 0x22, 0x33},
		DNVerifiedMajorBlock:     150,
		BVNVerifiedAnchor:        [32]byte{0x44, 0x55, 0x66},
		BVNVerifiedMajorBlock:    150,
		State: StateRecord{
			Current:        "ACTIVE",
			EnteredBooting: time.Unix(1700000000, 0).UTC(),
			EnteredActive:  time.Unix(1700001000, 0).UTC(),
		},
		Cursors: Cursors{
			WalkLastVerified: 150,
			AccountsPulled:   42,
		},
	}
	if err := Save(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir, [32]byte{0xaa, 0xbb})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Network != want.Network ||
		got.BVN != want.BVN ||
		got.DNGenesisStateTreeAnchor != want.DNGenesisStateTreeAnchor ||
		got.DNVerifiedAnchor != want.DNVerifiedAnchor ||
		got.DNVerifiedMajorBlock != want.DNVerifiedMajorBlock ||
		got.BVNVerifiedAnchor != want.BVNVerifiedAnchor ||
		got.BVNVerifiedMajorBlock != want.BVNVerifiedMajorBlock ||
		got.State.Current != want.State.Current ||
		got.Cursors.WalkLastVerified != want.Cursors.WalkLastVerified ||
		got.Cursors.AccountsPulled != want.Cursors.AccountsPulled {
		t.Errorf("round-trip drift:\n  want=%+v\n  got=%+v", want, got)
	}
}

func TestLoad_MissingFile_ReturnsNotExist(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir, [32]byte{0xaa})
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want os.ErrNotExist chain", err)
	}
}

func TestLoad_PinMismatch_ReturnsErrPinMismatch(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, &Artifact{DNGenesisStateTreeAnchor: [32]byte{0xaa}}); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir, [32]byte{0xbb})
	if !errors.Is(err, ErrPinMismatch) {
		t.Errorf("err = %v, want ErrPinMismatch chain", err)
	}
}

func TestPeek_SkipsPinCheck(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, &Artifact{Network: "devnet", DNGenesisStateTreeAnchor: [32]byte{0xaa}}); err != nil {
		t.Fatal(err)
	}
	a, err := Peek(dir)
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if a.Network != "devnet" {
		t.Errorf("Network = %q, want devnet", a.Network)
	}
}

func TestSave_AtomicReplace(t *testing.T) {
	dir := t.TempDir()
	a := &Artifact{Network: "first"}
	if err := Save(dir, a); err != nil {
		t.Fatal(err)
	}

	// Overwrite with new content.
	a.Network = "second"
	if err := Save(dir, a); err != nil {
		t.Fatal(err)
	}
	got, err := Peek(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Network != "second" {
		t.Errorf("Save did not overwrite; Network = %q, want second", got.Network)
	}

	// .tmp shouldn't be left behind on success.
	if _, err := os.Stat(filepath.Join(dir, FileName+".tmp")); !os.IsNotExist(err) {
		t.Errorf(".tmp file leaked after successful Save")
	}
}

func TestLoad_FormatMajorMismatch(t *testing.T) {
	dir := t.TempDir()
	// Hand-craft an artifact with a future major.
	path := filepath.Join(dir, FileName)
	bad := []byte(`{"formatMajor": 99, "formatMinor": 0}`)
	if err := os.WriteFile(path, bad, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir, [32]byte{})
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
	got, err := Peek(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.FormatMajor != FormatMajor {
		t.Errorf("FormatMajor not forced on Save; got %d, want %d", got.FormatMajor, FormatMajor)
	}
}
