// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package overlay

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/kvtest"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

func openRepair(*testing.T) kvtest.Opener {
	db := OpenWithRepair(memory.New(nil), memory.New(nil))
	return func() (keyvalue.Beginner, error) {
		return db, nil
	}
}

// A repairing overlay must still meet the plain backend contract.
func TestRepairDatabase(t *testing.T) { kvtest.TestDatabase(t, openRepair(t)) }
func TestRepairSubBatch(t *testing.T) { kvtest.TestSubBatch(t, openRepair(t)) }
func TestRepairPrefix(t *testing.T)   { kvtest.TestPrefix(t, openRepair(t)) }
func TestRepairDelete(t *testing.T)   { kvtest.TestDelete(t, openRepair(t)) }

// waitFor polls until fn returns true or the deadline passes, so a test does
// not have to guess how long the background writer takes.
func waitFor(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !fn() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for repair")
		}
		time.Sleep(time.Millisecond)
	}
}

func has(t *testing.T, db keyvalue.Beginner, key *record.Key) bool {
	t.Helper()
	cs := db.Begin(nil, false)
	defer cs.Discard()
	_, err := cs.Get(key)
	switch {
	case err == nil:
		return true
	case errors.Is(err, errors.NotFound):
		return false
	default:
		t.Fatalf("Get: %v", err)
		return false
	}
}

// A value the primary is missing, that the sidecar has, is both served
// correctly right away and, shortly after, written into the primary.
func TestRepairOnHit(t *testing.T) {
	a, b := memory.New(nil), memory.New(nil)
	key, value := record.NewKey("foo"), []byte("bar")

	cs := b.Begin(nil, true)
	require.NoError(t, cs.Put(key, value))
	require.NoError(t, cs.Commit())

	d := OpenWithRepair(a, b)
	defer d.Close()

	cs = d.Begin(nil, false)
	got, err := cs.Get(key)
	cs.Discard()
	require.NoError(t, err)
	require.Equal(t, value, got)

	require.False(t, has(t, a, key), "repair must not be visible before the write lands")
	waitFor(t, func() bool { return has(t, a, key) })
}

// A value the primary already has is never queued for repair: nothing
// observable changes, but this also means a hot key that is always found in
// a costs nothing extra.
func TestNoRepairOnPrimaryHit(t *testing.T) {
	a, b := memory.New(nil), memory.New(nil)
	key, value := record.NewKey("foo"), []byte("bar")

	cs := a.Begin(nil, true)
	require.NoError(t, cs.Put(key, value))
	require.NoError(t, cs.Commit())

	d := OpenWithRepair(a, b)
	defer d.Close()

	cs = d.Begin(nil, false)
	got, err := cs.Get(key)
	cs.Discard()
	require.NoError(t, err)
	require.Equal(t, value, got)

	// b was never asked, so there is nothing to have repaired b's way, and a
	// still holds exactly what was put there.
	require.False(t, has(t, b, key))
}

// A key present in neither store is still a clean not-found, and queues no
// repair.
func TestNoRepairOnMiss(t *testing.T) {
	a, b := memory.New(nil), memory.New(nil)
	d := OpenWithRepair(a, b)
	defer d.Close()

	cs := d.Begin(nil, false)
	_, err := cs.Get(record.NewKey("missing"))
	cs.Discard()
	require.ErrorIs(t, err, errors.NotFound)
}

// Repair for a key never queues twice: reading the same fallback value
// repeatedly still results in exactly one value in the primary, not an error
// from a second, conflicting write attempt.
func TestRepairIsIdempotent(t *testing.T) {
	a, b := memory.New(nil), memory.New(nil)
	key, value := record.NewKey("foo"), []byte("bar")

	cs := b.Begin(nil, true)
	require.NoError(t, cs.Put(key, value))
	require.NoError(t, cs.Commit())

	d := OpenWithRepair(a, b)
	defer d.Close()

	for i := 0; i < 5; i++ {
		cs := d.Begin(nil, false)
		got, err := cs.Get(key)
		cs.Discard()
		require.NoError(t, err)
		require.Equal(t, value, got)
	}

	waitFor(t, func() bool { return has(t, a, key) })
	require.True(t, has(t, a, key))
}

// A read is never slowed by repair: even when a is a store whose writes
// block until this test lets them through, Get still returns immediately.
func TestRepairDoesNotBlockTheRead(t *testing.T) {
	block := make(chan struct{})
	a := &blockingWrites{Beginner: memory.New(nil), block: block}
	b := memory.New(nil)
	key, value := record.NewKey("foo"), []byte("bar")

	cs := b.Begin(nil, true)
	require.NoError(t, cs.Put(key, value))
	require.NoError(t, cs.Commit())

	d := OpenWithRepair(a, b)
	defer func() {
		close(block)
		d.Close()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		cs := d.Begin(nil, false)
		got, err := cs.Get(key)
		cs.Discard()
		require.NoError(t, err)
		require.Equal(t, value, got)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Get blocked on the repair write")
	}
}

// A queue at capacity drops the newest repair rather than blocking the
// reader that triggered it, or growing without bound.
func TestRepairQueueFullDropsRatherThanBlocks(t *testing.T) {
	block := make(chan struct{})
	a := &blockingWrites{Beginner: memory.New(nil), block: block}
	b := memory.New(nil)

	cs := b.Begin(nil, true)
	for i := 0; i < queueSize+batchSize+8; i++ {
		require.NoError(t, cs.Put(record.NewKey(fmt.Sprint(i)), []byte("v")))
	}
	require.NoError(t, cs.Commit())

	d := OpenWithRepair(a, b)
	defer func() {
		close(block)
		d.Close()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < queueSize+batchSize+8; i++ {
			cs := d.Begin(nil, false)
			_, err := cs.Get(record.NewKey(fmt.Sprint(i)))
			cs.Discard()
			require.NoError(t, err)
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reads blocked instead of the excess repairs being dropped")
	}
}

// Close stops the background writer; a repair still in flight is abandoned,
// and Close is safe to call more than once.
func TestClose(t *testing.T) {
	a, b := memory.New(nil), memory.New(nil)
	d := OpenWithRepair(a, b)
	require.NoError(t, d.Close())
	require.NoError(t, d.Close())
}

// A concurrent reader must never see enqueue panic against a repairer that a
// second goroutine is simultaneously closing — this is what a node shutting
// down while requests are still in flight looks like.
func TestCloseDuringConcurrentReads(t *testing.T) {
	a, b := memory.New(nil), memory.New(nil)
	key, value := record.NewKey("foo"), []byte("bar")
	cs := b.Begin(nil, true)
	require.NoError(t, cs.Put(key, value))
	require.NoError(t, cs.Commit())

	d := OpenWithRepair(a, b)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	var started sync.WaitGroup
	started.Add(8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				cs := d.Begin(nil, false)
				_, _ = cs.Get(key)
				cs.Discard()
			}
		}()
	}

	// Close races the readers deliberately: the point of this test is that
	// enqueue never sends on a channel Close has closed.
	started.Wait()
	d.Close()
	close(stop)
	wg.Wait()
}

// blockingWrites wraps a Beginner so every writable change set's Commit
// blocks until block is closed, without affecting reads. It is used to prove
// the repair write cannot delay a Get.
type blockingWrites struct {
	keyvalue.Beginner
	block chan struct{}
}

func (s *blockingWrites) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	cs := s.Beginner.Begin(prefix, writable)
	if !writable {
		return cs
	}
	return &blockingChangeSet{ChangeSet: cs, block: s.block}
}

type blockingChangeSet struct {
	keyvalue.ChangeSet
	block chan struct{}
}

func (cs *blockingChangeSet) Commit() error {
	<-cs.block
	return cs.ChangeSet.Commit()
}

// A keep predicate limits what is written back: a hit it refuses is served
// exactly as before, but never lands in a.
func TestRepairKeepsOnlyWhatKeepAccepts(t *testing.T) {
	a, b := memory.New(nil), memory.New(nil)
	kept, refused := record.NewKey("history"), record.NewKey("state")

	cs := b.Begin(nil, true)
	require.NoError(t, cs.Put(kept, []byte("1")))
	require.NoError(t, cs.Put(refused, []byte("2")))
	require.NoError(t, cs.Commit())

	d := OpenWithRepairWhere(a, b, func(k *record.Key) bool { return k.Equal(kept) })
	defer d.Close()

	for _, k := range []*record.Key{kept, refused} {
		cs = d.Begin(nil, false)
		_, err := cs.Get(k)
		cs.Discard()
		require.NoError(t, err)
	}
	waitFor(t, func() bool { return has(t, a, kept) })
	require.False(t, has(t, b, record.NewKey("never")))
	require.False(t, has(t, a, refused), "a refused key was written back")
}
