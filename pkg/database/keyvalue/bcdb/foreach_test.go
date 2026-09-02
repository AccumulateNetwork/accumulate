// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package bcdb

import (
	"crypto/sha256"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// Iteration is a read like any other: it sees the database as of the
// batch's version, not as of now (#4175).
//
// Everything here stays STAGED (a holder batch pins version 0), because
// the store's own ForEach holds its mutex across the callback and a
// callback that reads the store would deadlock -- BlockchainDB#31.
// What this proves is the adapter's half: the version is honoured, and
// the adapter's lock is not held across the callback.
func TestForEach_SeesTheBatchVersion(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer d.Close()

	holder := d.Begin(nil, false) // keeps every later commit staged
	defer holder.Discard()

	key := record.NewKey("Account", "alice", "Main")
	put(t, d, key, "old")

	old := d.Begin(nil, false)
	defer old.Discard()
	put(t, d, key, "new") // newer than `old`

	var saw string
	require.NoError(t, old.ForEach(func(k *record.Key, v []byte) error {
		if k.Hash() == key.Hash() {
			saw = string(v)
		}
		return nil
	}))
	require.Equal(t, "old", saw, "a batch must not iterate over a commit newer than itself")

	// The callback may read the database while iterating: the adapter's
	// lock is not held across it.
	require.NoError(t, old.ForEach(func(k *record.Key, v []byte) error {
		got, err := old.Get(key)
		require.Equal(t, "old", string(got))
		return err
	}))
}

// The tally counts writes by shape and remembers nothing.
//
// It used to remember a digest of the last value written for every key,
// so it could report whether a write changed the record. On a 500 tx/s
// soak that map was 192 MB -- 38% of the live heap and the largest single
// consumer on the node -- spent on a diagnostic (#4165). The store already
// answers the question that matters by refusing a rewrite of a permanent
// record, so the memory bought nothing.
func TestTally_CountsAndRemembersNothing(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer d.Close()

	const n = 2000
	for i := 0; i < n; i++ {
		put(t, d, record.NewKey("Message", sha256.Sum256([]byte{byte(i), byte(i >> 8)}), "Main"), "v")
	}
	// The same key many times over: still just writes.
	same := record.NewKey("Account", sha256.Sum256([]byte("acct")), "Main")
	for i := 0; i < 50; i++ {
		put(t, d, same, "v")
	}

	require.Equal(t, uint64(n), d.Shapes()["Message.(hash).Main"].Writes)
	require.Equal(t, uint64(50), d.Shapes()["Account.(hash).Main"].Writes)

	// Nothing per-key survives a write. The tally is counters, so its
	// cost does not scale with how many distinct keys the node has seen.
	var perKey int
	require.NotPanics(t, func() { perKey = len(d.shapes) })
	require.Less(t, perKey, 8, "one entry per SHAPE, not per key")
}

// The store reports the thing the digests were kept for, exactly and
// across restarts: a permanent record written again with different bytes
// is refused, and the refusal is counted against the shape.
func TestTally_MisroutedComesFromTheStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	d, err := Open(dir)
	require.NoError(t, err)
	defer d.Close()

	// Message(H).Main is classified write-once. Writing it twice with
	// DIFFERENT bytes is the misclassification this counter exists for.
	k := record.NewKey("Message", sha256.Sum256([]byte("m")), "Main")
	put(t, d, k, "one")
	put(t, d, k, "two")

	c := d.Shapes()[keyShape(k)]
	require.Equal(t, "perm", c.Layer)
	require.Equal(t, uint64(2), c.Writes)
	require.Equal(t, uint64(1), c.Misrouted,
		"the permanent layer refused the second write, which is the signal")
}

// A reader must not wait out a commit's write-through (#4175): the puts,
// the per-block fsync of both layers, sealing and compaction run outside
// the adapter's lock. Simulated by holding the write-through mutex while a
// commit is in flight — a Get on another batch must still answer.
func TestRead_DoesNotWaitForAWriteThrough(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	defer d.Close()
	key := record.NewKey("Account", "alice", "Main")
	put(t, d, key, "v1")

	d.writeMu.Lock() // the store is "busy" — nothing can write through
	done := make(chan error, 1)
	go func() {
		b := d.Begin(nil, true)
		_ = b.Put(key, []byte("v2"))
		done <- b.Commit() // stages v2, then blocks in drain on writeMu
	}()
	time.Sleep(50 * time.Millisecond) // let the commit reach the write-through

	got := make(chan string, 1)
	go func() { got <- get(t, d, key) }()
	select {
	case v := <-got:
		require.Equal(t, "v2", v, "a new reader sees the staged commit, without waiting for the store")
	case <-time.After(2 * time.Second):
		t.Fatal("a read waited for a write-through in progress")
	}

	d.writeMu.Unlock()
	require.NoError(t, <-done)
	require.Equal(t, "v2", get(t, d, key))
}

// Maintenance — compacting history, merging block sets — runs off the
// commit path. The store took its lock off maintenance (BlockchainDB#58)
// but the adapter still called it on the committing goroutine, which is
// the block producer's: every node stopped producing blocks for 88–100 s
// at block 400 in run 20260830T054402Z. A commit must not wait for it.
func TestMaintenance_DoesNotBlockCommits(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "db"))
	require.NoError(t, err)
	d.CompressEvery, d.StatsEvery = 2, 0

	started := make(chan struct{})
	release := make(chan struct{})
	d.maintainHook = func() { close(started); <-release } // a long compaction

	key := func(i int) *record.Key { return record.NewKey("Account", "alice", "Main") }
	put(t, d, key(0), "v0")
	put(t, d, key(1), "v1") // version 2: maintenance starts, and blocks in the hook
	<-started

	done := make(chan struct{})
	go func() { put(t, d, key(2), "v2"); put(t, d, key(3), "v3"); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("commits waited for maintenance")
	}
	require.Equal(t, "v3", get(t, d, key(3)))

	// Close waits for the run in flight rather than closing the store
	// under it.
	closed := make(chan error, 1)
	go func() { closed <- d.Close() }()
	select {
	case <-closed:
		t.Fatal("Close returned while maintenance was still running")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-closed)
}
