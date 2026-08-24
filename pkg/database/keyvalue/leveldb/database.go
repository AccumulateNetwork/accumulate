// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package leveldb

import (
	"container/list"
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

	// records is a write-through cache of latest-committed record values,
	// consulted ONLY by writable batches. Record keys are hashes, so reads
	// have no locality and the block cache cannot hold a working set — at
	// ~250 tx/s, 45% of validator CPU became random sstable walks (#4164).
	// The executor re-reads the same hot records (system ledger, anchor
	// pools, signers) every block; serving them from memory removes that
	// entire read class.
	//
	// SAFETY: exactly one writable batch exists at a time (the executor
	// produces blocks sequentially), and commit() updates the cache before
	// returning, so a writable batch always sees latest-committed state —
	// which is exactly what its snapshot would show. Read-only batches
	// (queries, healers) NEVER touch the cache: they keep pure snapshot
	// semantics.
	records recordCache
}

// recordCache is a byte-conscious LRU of committed record values.
type recordCache struct {
	mu    sync.Mutex
	items map[[32]byte]*list.Element
	order *list.List // front = most recent
	bytes int
}

const (
	// recordCacheMaxBytes bounds the cache; recordCacheMaxValue skips
	// oversized values so one giant record cannot evict the working set.
	recordCacheMaxBytes = 64 << 20
	recordCacheMaxValue = 8 << 10
)

type recordCacheEntry struct {
	key     [32]byte
	value   []byte // nil = known-deleted (negative entry)
	deleted bool
}

func (c *recordCache) get(key [32]byte) (v []byte, deleted, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		return nil, false, false
	}
	c.order.MoveToFront(e)
	ent := e.Value.(*recordCacheEntry)
	return ent.value, ent.deleted, true
}

func (c *recordCache) put(key [32]byte, value []byte, deleted bool) {
	if len(value) > recordCacheMaxValue {
		c.drop(key) // a big value replaces whatever was cached
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = make(map[[32]byte]*list.Element)
		c.order = list.New()
	}
	// Per-entry overhead is ~160B measured (map bucket + list element +
	// entry struct + key) — the first estimate of 64 let the cache hold
	// nearly 2x its budget in small entries.
	const entryOverhead = 160
	if e, ok := c.items[key]; ok {
		ent := e.Value.(*recordCacheEntry)
		c.bytes += len(value) - len(ent.value)
		ent.value, ent.deleted = value, deleted
		c.order.MoveToFront(e)
	} else {
		e := c.order.PushFront(&recordCacheEntry{key: key, value: value, deleted: deleted})
		c.items[key] = e
		c.bytes += len(value) + entryOverhead
	}
	for c.bytes > recordCacheMaxBytes {
		back := c.order.Back()
		if back == nil {
			break
		}
		ent := back.Value.(*recordCacheEntry)
		c.order.Remove(back)
		delete(c.items, ent.key)
		c.bytes -= len(ent.value) + entryOverhead
	}
}

func (c *recordCache) drop(key [32]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		ent := e.Value.(*recordCacheEntry)
		c.order.Remove(e)
		delete(c.items, key)
		c.bytes -= len(ent.value) + 160
	}
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
	// The block cache is the STATE-SCALING knob: at ~20k accounts the hot
	// working set outgrew 64MB and 47% of node CPU became positive lookups
	// walking sstables (version.walkOverlapping -> Reader.find, mostly under
	// the API query handler serving healers and trackers) — rounds
	// stretched, heals stormed, runs collapsed on a state-size clock, not a
	// rate clock (bloom filters only short-circuit NEGATIVE lookups). 256MB
	// holds the working set of a ~100k-account universe.
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
		BlockCacheCapacity:     256 * opt.MiB,
		WriteBuffer:            16 * opt.MiB,
		OpenFilesCacheCapacity: 512,
		// The buffer pool is DISABLED — final answer after being burned in
		// both directions. Enabled, it grows toward the largest buffers any
		// path ever needed and never shrinks; the "16MB write buffers keep
		// it small" theory was wrong because the pool also serves TABLE READ
		// buffers, and under read-heavy load it held 887MB (40% of heap) on
		// the sinking node in run 20260824T170628Z. Disabled, compaction and
		// read buffers are ordinary allocations — a churn tax (was 21% of
		// allocation) the GC can afford, especially now that the write-path
		// record cache keeps most hot reads out of leveldb entirely. Bounded
		// memory beats cheap allocation.
		DisableBufferPool: true,
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

	// Read from the transaction. Writable batches read through the record
	// cache (latest-committed == their snapshot, by the single-writer
	// discipline documented on Database.records); read-only batches go
	// straight to their snapshot.
	get := func(key *record.Key) ([]byte, error) {
		return d.get(snap, err, key)
	}
	if writable {
		get = func(key *record.Key) ([]byte, error) {
			kh := key.Hash()
			if v, deleted, ok := d.records.get(kh); ok {
				if deleted {
					return nil, (*database.NotFoundError)(key)
				}
				u := make([]byte, len(v))
				copy(u, v)
				return u, nil
			}
			v, err := d.get(snap, err, key)
			switch err.(type) {
			case nil:
				d.records.put(kh, v, false)
				// The cache holds its own reference; hand the caller a copy
				// so a caller mutation cannot poison the cache.
				u := make([]byte, len(v))
				copy(u, v)
				return u, nil
			case *database.NotFoundError:
				d.records.put(kh, nil, true)
				return nil, err
			default:
				return nil, err
			}
		}
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

	err := d.leveldb.Write(batch, nil)
	if err != nil {
		return err
	}

	// Write-through AFTER the write succeeds: the next writable batch (the
	// single writer proceeds sequentially) sees exactly what leveldb holds.
	for kh, e := range entries {
		if e.Delete {
			d.records.put(kh, nil, true)
		} else {
			d.records.put(kh, e.Value, false)
		}
	}
	return nil
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
