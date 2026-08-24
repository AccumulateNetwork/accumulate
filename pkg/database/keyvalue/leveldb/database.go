// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package leveldb

import (
	"os"
	"sync"
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

type Database struct {
	opts
	leveldb *leveldb.DB
	closing atomic.Bool
	open    *sync.WaitGroup
}

type opts struct {
}

type Option func(*opts)

func Open(filepath string, o ...Option) (*Database, error) {
	// Make sure all directories exist
	err := os.MkdirAll(filepath, 0700)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("create %q: %w", filepath, err)
	}

	// Non-default options, measured under load (#4164). With `nil` options —
	// no bloom filter, 8MB block cache, 4MB write buffer — the storage
	// engine owned HALF the CPU profile at 183 tx/s: 24% in reads, every
	// miss walking each level's candidate tables (version.walkOverlapping),
	// and 27% in compaction churned by tiny write buffers. Bloom filters
	// turn the level walk into a bitmap check; a real block cache keeps hot
	// state in RAM.
	//
	// SIZED FOR TWO ENGINES PER CGROUP. A dual validator opens one of these
	// per partition (dnn + bvnn), so every number here is doubled in a 4GiB
	// container. The first sizing (128MB cache, 64MB write buffer, pooled
	// compaction buffers) OOM-killed seven containers in run
	// 20260824T065208Z: ~1GB of engine memory per container plus the
	// BufferPool — which grows toward the largest compaction it ever served
	// and never shrinks (measured 178MB and climbing) — plus GC headroom hit
	// the cgroup limit hours after load DROPPED, because compaction of the
	// high-rate era's debt kept feeding the pool. The budget is ~150MB per
	// engine.
	db, err := leveldb.OpenFile(filepath, &opt.Options{
		Filter:                 filter.NewBloomFilter(10),
		BlockCacheCapacity:     64 * opt.MiB,
		WriteBuffer:            16 * opt.MiB,
		OpenFilesCacheCapacity: 512,
		// The buffer pool stays ENABLED. Disabling it fixed the pool's
		// grow-forever retention when write buffers were 64MB, but converted
		// every compaction/read buffer into a fresh allocation — 68GB of
		// churn (21% of all allocation) in 30 minutes at 400-700 tx/s. With
		// 16MB write buffers the pool's natural size is tens of MB: bounded
		// retention AND no churn.
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open %q: %w", filepath, err)
	}

	return New(db, o...), nil
}

func New(db *leveldb.DB, o ...Option) *Database {
	d := new(Database)
	d.leveldb = db
	d.open = new(sync.WaitGroup)
	for _, o := range o {
		o(&d.opts)
	}

	return d
}

func (d *Database) key(key *record.Key) []byte {
	h := key.Hash()
	return h[:]
}

// Begin begins a change set.
func (d *Database) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	var snap *leveldb.Snapshot
	var err error

	if d.closing.Load() {
		err = errors.Conflict.With("closed")
	} else {
		snap, err = d.leveldb.GetSnapshot()
	}

	// Read from the transaction
	get := func(key *record.Key) ([]byte, error) {
		return d.get(snap, err, key)
	}

	// Commit to the write batch
	var commit memory.CommitFunc
	if writable {
		commit = d.commit
	}

	forEach := func(fn func(*record.Key, []byte) error) error {
		return d.forEach(snap, err, fn)
	}

	discard := func() {}
	if err == nil {
		d.open.Add(1)
		var once sync.Once
		discard = func() {
			defer once.Do(d.open.Done)
			snap.Release()
		}
	}

	// The memory changeset caches entries in a map so Get will see values
	// updated with Put, regardless of the underlying transaction and write
	// batch behavior
	return memory.NewChangeSet(memory.ChangeSetOptions{
		Prefix:  prefix,
		Get:     get,
		Commit:  commit,
		ForEach: forEach,
		Discard: discard,
	})
}

func (d *Database) commit(entries map[[32]byte]memory.Entry) error {
	batch := new(leveldb.Batch)
	for _, e := range entries {
		if e.Delete {
			batch.Delete(d.key(e.Key))
		} else {
			batch.Put(d.key(e.Key), e.Value)
		}
	}

	return d.leveldb.Write(batch, nil)
}

func (d *Database) get(snap *leveldb.Snapshot, err error, key *record.Key) ([]byte, error) {
	if err != nil {
		return nil, err
	}

	v, err := snap.Get(d.key(key), nil)
	switch {
	case err == nil:
		u := make([]byte, len(v))
		copy(u, v)
		return u, nil
	case errors.Is(err, leveldb.ErrNotFound):
		return nil, (*database.NotFoundError)(key)
	default:
		return nil, err
	}
}

func (d *Database) forEach(snap *leveldb.Snapshot, err error, fn func(*record.Key, []byte) error) error {
	if err != nil {
		return err
	}

	it := snap.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		key := record.KeyFromHash(*(*[32]byte)(it.Key()))
		value := make([]byte, len(it.Value()))
		copy(value, it.Value())
		err = fn(key, value)
		if err != nil {
			return err
		}
	}
	it.Release()
	return it.Error()
}

// Close the database.
func (d *Database) Close() error {
	// Stop new batches
	d.closing.Store(true)

	// Wait for existing batches to resolve
	d.open.Wait()

	// Close the database
	return d.leveldb.Close()
}
