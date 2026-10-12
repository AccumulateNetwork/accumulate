// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/leveldb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/overlay"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// newTestInstance is the minimum an Instance needs for Storage.open: a root
// directory to resolve relative paths against, and the running/context pair
// cleanup and Stop rely on. It does not stand up p2p, services, or anything
// else Start does.
func newTestInstance(t *testing.T) *Instance {
	ctx, cancel := context.WithCancel(context.Background())
	inst := &Instance{
		rootDir:  t.TempDir(),
		running:  new(sync.WaitGroup),
		context:  ctx,
		shutdown: cancel,
	}
	t.Cleanup(inst.Stop)
	return inst
}

// Without Sidecar set, LevelDBStorage.open must behave exactly as before:
// the plain store, not wrapped.
func TestLevelDBStorageWithoutSidecar(t *testing.T) {
	inst := newTestInstance(t)
	s := &LevelDBStorage{Path: "primary"}

	db, err := s.open(inst)
	require.NoError(t, err)

	_, isOverlay := db.(*overlay.Database)
	require.False(t, isOverlay, "no Sidecar configured must not produce an overlay")
}

// With Sidecar set, a value only the sidecar holds is served, and the
// primary — reopened directly, after Stop — has it too.
func TestLevelDBStorageWithSidecar(t *testing.T) {
	inst := newTestInstance(t)
	primaryPath := filepath.Join(inst.rootDir, "primary")
	sidecarPath := filepath.Join(inst.rootDir, "sidecar")

	key, value := record.NewKey("foo"), []byte("bar")
	side, err := leveldb.Open(sidecarPath)
	require.NoError(t, err)
	cs := side.Begin(nil, true)
	require.NoError(t, cs.Put(key, value))
	require.NoError(t, cs.Commit())
	require.NoError(t, side.Close())

	s := &LevelDBStorage{Path: "primary", Sidecar: "sidecar"}
	db, err := s.open(inst)
	require.NoError(t, err)
	_, isOverlay := db.(*overlay.Database)
	require.True(t, isOverlay, "a configured Sidecar must produce an overlay")

	cs = db.Begin(nil, false)
	got, err := cs.Get(key)
	cs.Discard()
	require.NoError(t, err)
	require.Equal(t, value, got)

	// Give the background write time to land. Close (below, via t.Cleanup)
	// drops a repair still in flight by design — this is not that race, it
	// is proving a repair that had time to happen is not lost by shutdown.
	time.Sleep(200 * time.Millisecond)
	inst.Stop()

	primary, err := leveldb.Open(primaryPath)
	require.NoError(t, err)
	defer primary.Close()
	cs = primary.Begin(nil, false)
	got, err = cs.Get(key)
	cs.Discard()
	require.NoError(t, err)
	require.Equal(t, value, got)
}

// Shutdown while a read is landing its repair must not hang or panic: the
// cleanup this registers stops the writer before closing either store, and
// the writer itself never blocks on a store that outlives it.
func TestLevelDBStorageStopIsClean(t *testing.T) {
	inst := newTestInstance(t)
	sidecarPath := filepath.Join(inst.rootDir, "sidecar")
	side, err := leveldb.Open(sidecarPath)
	require.NoError(t, err)
	cs := side.Begin(nil, true)
	for i := 0; i < 64; i++ {
		require.NoError(t, cs.Put(record.NewKey(i), []byte("v")))
	}
	require.NoError(t, cs.Commit())
	require.NoError(t, side.Close())

	s := &LevelDBStorage{Path: "primary", Sidecar: "sidecar"}
	db, err := s.open(inst)
	require.NoError(t, err)

	for i := 0; i < 64; i++ {
		cs := db.Begin(nil, false)
		_, err := cs.Get(record.NewKey(i))
		cs.Discard()
		require.NoError(t, err)
	}

	done := make(chan struct{})
	go func() { inst.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return")
	}
}
