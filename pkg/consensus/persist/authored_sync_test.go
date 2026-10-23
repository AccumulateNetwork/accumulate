// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package persist

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// The authored header is broadcast once PutAuthored returns, and a power
// loss that brought the previous file back would have the node author the
// round again (#4592). So the write is synced to the device before it
// returns: the new file's bytes, then the rename that makes it the authored
// file, then the directory that holds the rename. Each PutAuthored syncs
// twice, the file before the directory, and the rename lies between them.
func TestPutAuthored_SyncsFileThenDirectory(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenDAGStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(dir, authoredFile)

	// Each sync, by what was synced and which header the authored file held
	// at that moment: none, the first, or the second.
	type synced struct {
		name string
		held types.Round
	}
	var syncs []synced
	prev := fsync
	fsync = func(f *os.File) error {
		held := types.Round(0)
		if data, err := os.ReadFile(final); err == nil {
			h, err := types.UnmarshalHeader(data)
			if err != nil {
				t.Errorf("the authored file is unreadable at a sync: %v", err)
			} else {
				held = h.Round
			}
		}
		syncs = append(syncs, synced{f.Name(), held})
		return prev(f)
	}
	t.Cleanup(func() { fsync = prev })

	for _, round := range []types.Round{1, 2} {
		if err := s.PutAuthored(types.NewHeader(pub, round, 0, nil, nil)); err != nil {
			t.Fatal(err)
		}
	}
	want := []synced{
		{final + ".tmp", 0}, {dir, 1}, // the first: the file, the rename, the directory
		{final + ".tmp", 1}, {dir, 2}, // the second: the rename replaces the first
	}
	if len(syncs) != len(want) {
		t.Fatalf("two PutAuthored calls synced %d times, want %d: %v", len(syncs), len(want), syncs)
	}
	for i := range want {
		if syncs[i] != want[i] {
			t.Errorf("sync %d: got %v, want %v", i, syncs[i], want[i])
		}
	}
}
