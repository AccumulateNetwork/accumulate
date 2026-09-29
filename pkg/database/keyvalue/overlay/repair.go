// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package overlay

import (
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// queueSize bounds how many repairs can be pending before new ones are
// dropped. A drop only delays retiring b's contribution for that key — the
// next read still finds it there and serves it correctly.
const queueSize = 4096

// batchSize bounds how many pending repairs one commit writes, so a burst of
// misses does not hold a's writer for one unbounded batch.
const batchSize = 256

// OpenWithRepair is [Open], except a value read from b is also written into a,
// in the background. This lets b's contribution shrink over time, without
// ever slowing, blocking, or being able to fail a read: a write that does not
// keep up is dropped, not queued to grow without bound, and a failure anywhere
// in the write path is swallowed. A repair still pending when [Database.Close]
// is called is also dropped — the value stays correctly served from b.
//
// The reader in [database.md, "Recovered history: the sidecar"] is what makes
// this safe to do unconditionally, with no gating in this package: a and b
// hold no record that feeds hashChains, so what this writes changes only what
// can be read, never what the network agrees on.
func OpenWithRepair(a, b keyvalue.Beginner) *Database {
	d := Open(a, b)
	d.repair = newRepairer(a)
	return d
}

// Close stops the background repair started by [OpenWithRepair]. It does not
// close a or b, and is safe to call more than once. Close on a *Database
// returned by [Open] is a no-op.
func (d *Database) Close() error {
	d.repair.stop()
	return nil
}

type entry struct {
	key   *record.Key
	value []byte
}

type repairer struct {
	a       keyvalue.Beginner
	pending chan entry // never closed: enqueue must never risk a send-on-closed panic
	stopCh  chan struct{}
	stopped chan struct{}
	once    sync.Once
}

func newRepairer(a keyvalue.Beginner) *repairer {
	r := &repairer{
		a:       a,
		pending: make(chan entry, queueSize),
		stopCh:  make(chan struct{}),
		stopped: make(chan struct{}),
	}
	go r.run()
	return r
}

// enqueue is nil-safe: [Open] passes a nil *repairer, so a plain overlay
// costs nothing here.
func (r *repairer) enqueue(key *record.Key, value []byte) {
	if r == nil {
		return
	}
	select {
	case r.pending <- entry{key, value}:
	default:
	}
}

// stop is nil-safe for the same reason.
func (r *repairer) stop() {
	if r == nil {
		return
	}
	r.once.Do(func() { close(r.stopCh) })
	<-r.stopped
}

func (r *repairer) run() {
	defer close(r.stopped)
	for {
		select {
		case <-r.stopCh:
			return
		case e := <-r.pending:
			r.write(r.drainBatch(e))
		}
	}
}

// drainBatch returns first plus whatever else is immediately available on
// pending, up to batchSize, without blocking further.
func (r *repairer) drainBatch(first entry) []entry {
	batch := []entry{first}
	for len(batch) < batchSize {
		select {
		case e := <-r.pending:
			batch = append(batch, e)
		default:
			return batch
		}
	}
	return batch
}

// write commits batch to a. A failure here — a's own error, or a panic inside
// it — is swallowed: repair is strictly best-effort, and must never be a way
// for a to affect a read.
func (r *repairer) write(batch []entry) {
	defer func() { recover() }()
	cs := r.a.Begin(nil, true)
	defer cs.Discard()
	for _, e := range batch {
		if cs.Put(e.key, e.value) != nil {
			return
		}
	}
	_ = cs.Commit()
}
