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

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	pinHash := [32]byte{0xab, 0xcd}

	a := &Artifact{
		PinnedGenesisHash: pinHash,
		Network:           "testnet",
		PinBlock: PinBlock{
			Partition:       "Directory",
			MajorBlockIndex: 42,
			MinorBlockIndex: 12345,
			StateTreeAnchor: [32]byte{0xfe, 0xed},
		},
		State: StateRecord{
			Current:        "ACTIVE",
			EnteredBooting: time.Date(2026, 4, 26, 23, 0, 0, 0, time.UTC),
			EnteredActive:  time.Date(2026, 4, 26, 23, 30, 0, 0, time.UTC),
			BptRootMatched: [32]byte{0x1, 0x2, 0x3},
		},
		Cursors: Cursors{
			BptPageDone:      true,
			TrafficHighWater: 67890,
		},
	}

	if err := Save(dir, a); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(dir, pinHash)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Network != a.Network {
		t.Errorf("Network: got %q want %q", loaded.Network, a.Network)
	}
	if loaded.PinBlock.MajorBlockIndex != 42 {
		t.Errorf("PinBlock.MajorBlockIndex: got %d want 42", loaded.PinBlock.MajorBlockIndex)
	}
	if loaded.State.Current != "ACTIVE" {
		t.Errorf("State.Current: got %q want %q", loaded.State.Current, "ACTIVE")
	}
	if loaded.State.BptRootMatched != ([32]byte{0x1, 0x2, 0x3}) {
		t.Errorf("State.BptRootMatched: got %x want %x", loaded.State.BptRootMatched[:4], []byte{1, 2, 3})
	}
	if !loaded.Cursors.BptPageDone {
		t.Error("Cursors.BptPageDone: got false want true")
	}
	if loaded.FormatMajor != FormatMajor {
		t.Errorf("FormatMajor: got %d want %d", loaded.FormatMajor, FormatMajor)
	}
}

func TestLoad_PinMismatch(t *testing.T) {
	dir := t.TempDir()
	a := &Artifact{
		PinnedGenesisHash: [32]byte{0xab},
		Network:           "testnet",
	}
	if err := Save(dir, a); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir, [32]byte{0xcd}) // different
	if !errors.Is(err, ErrPinMismatch) {
		t.Fatalf("Load with mismatched pin: got %v, want ErrPinMismatch", err)
	}
}

func TestLoad_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir, [32]byte{})
	if !os.IsNotExist(err) {
		t.Fatalf("Load on empty dir: got %v, want os.IsNotExist", err)
	}
}

func TestLoad_FormatMajorMismatch(t *testing.T) {
	dir := t.TempDir()
	pinHash := [32]byte{0xab}
	a := &Artifact{
		PinnedGenesisHash: pinHash,
		Network:           "testnet",
	}
	if err := Save(dir, a); err != nil {
		t.Fatal(err)
	}

	// Tamper the format major. [32]byte marshals as a JSON array of 32
	// numbers; use a zero-array for simplicity.
	path := filepath.Join(dir, FileName)
	tampered := []byte(`{"formatMajor":99,"formatMinor":0,` +
		`"pinnedGenesisHash":[171,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],` +
		`"network":"testnet","pinBlock":{"partition":"","majorBlockIndex":0,"minorBlockIndex":0,` +
		`"stateTreeAnchor":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0]},` +
		`"state":{"current":""},"cursors":{"bptPageDone":false,"trafficHighWater":0}}`)
	if err := os.WriteFile(path, tampered, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir, pinHash)
	if !errors.Is(err, ErrFormatMajor) {
		t.Fatalf("Load with bad format major: got %v, want ErrFormatMajor", err)
	}
}

func TestSave_AtomicViaRename(t *testing.T) {
	// Verify Save uses atomic write by checking that the .tmp file is
	// removed and the final file exists.
	dir := t.TempDir()
	a := &Artifact{
		PinnedGenesisHash: [32]byte{0xab},
		Network:           "testnet",
	}
	if err := Save(dir, a); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Fatalf("final file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName+".tmp")); !os.IsNotExist(err) {
		t.Fatalf("tmp file should be removed; stat err = %v", err)
	}
}
