// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package bcdb backs a key-value store with BlockchainDB's two-layer
// segment store.
//
// BlockchainDB separates what a chain writes once from what it
// rewrites: a permanent layer of sealed, immutable segments that a
// peer syncs by copying files, and a dynamic layer that is compacted.
// Accumulate's record keys are already 32-byte hashes, which is what
// the store is keyed by, so the mapping is direct.
//
// Two things the store does not have, and how they are supplied here:
//
//   - Deletion.  A delete writes a zero-length value, which the store
//     keeps like any other and which this package reports as absent.
//     That matches what Accumulate already does one layer up:
//     RecordStore.GetValue treats a zero-length value as not found, so
//     no caller can distinguish an empty value from a missing one.
//
//   - Ordering.  Iteration is unordered, and yields keys as hashes:
//     the store holds the hash, not the key path it came from, so
//     ForEach reports record.KeyFromHash the way the badger backend
//     does when it is not storing plain keys.
//
//   - Snapshot isolation.  A batch must not see writes committed after
//     it began, and BlockchainDB has no notion of a version: a write is
//     visible to the next read.  So a commit lands in an in-memory
//     staging area first and is written through only once every batch
//     still open would have seen it anyway -- at which point "read the
//     store" and "read the snapshot" give the same answer.  This is the
//     same shape as the block backend's versioned map, with the staging
//     depth bounded by how long a batch is held open rather than by the
//     write rate.
package bcdb

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	bcdb "github.com/AccumulateNetwork/BlockchainDB/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// Database is a key-value store backed by a BlockchainDB KV2.
type Database struct {
	mu       sync.Mutex
	kv       *bcdb.KV2
	prefix   *record.Key
	closed   bool
	flushErr error // A write-through that failed with no caller to tell

	// version counts commits.  A batch records the version it began at
	// and reads as of that version; a commit produces the next one.
	version uint64

	// staged holds committed batches that have not been written
	// through yet, oldest first.  A batch older than the oldest open
	// view cannot be written through without becoming visible to that
	// view, which is the thing isolation forbids.
	staged []*staged

	// views counts the open batches at each version, so flushing knows
	// what the oldest reader can still see.
	views map[uint64]int

	// CompressEvery seals and compacts the dynamic layer after this
	// many commits.  Zero leaves compaction to the caller.
	CompressEvery uint64

	// StatsEvery writes the layer counters after this many commits.
	// Zero disables the report.
	StatsEvery uint64

	statsPath string // Where reportStats writes

	// shapes tallies writes by key shape.  Classifying a rewrite means
	// knowing what the key held before, and asking the store would
	// perturb the very counters being measured, so the last value
	// written for each key is remembered here instead.
	shapes map[string]*shapeCount
	last   map[[32]byte][]byte
}

// staged is one committed batch that has not reached the store yet
type staged struct {
	version uint64
	entries map[[32]byte][]byte // A zero-length value is a deletion
}

var _ keyvalue.Beginner = (*Database)(nil)

// SealLimit is the number of records a layer accumulates before it
// seals on its own.  A commit seals the permanent layer anyway, so this
// only bounds a layer that fills between commits.
const SealLimit = 100_000

// permManifest is the file whose presence means a database is already
// here: the permanent layer's segment manifest.
var permManifest = filepath.Join("perm", "segments.json")

// Open opens a BlockchainDB-backed store at path, creating one if there
// is nothing there.
//
// "Nothing there" is decided by looking, not by trying to open and
// treating any failure as absence.  NewKV2 begins by deleting the
// directory, so falling back to it on an unexplained error would answer
// "this database will not open" by destroying it.
func Open(path string) (*Database, error) {
	// A node's data directory is created by whichever service gets
	// there first, and that may be this one
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, errors.UnknownError.WithFormat("create %s: %w", filepath.Dir(path), err)
	}

	var kv *bcdb.KV2
	var err error
	switch _, statErr := os.Stat(filepath.Join(path, permManifest)); {
	case statErr == nil:
		kv, err = bcdb.OpenKV2(path)
	case stderrors.Is(statErr, fs.ErrNotExist):
		kv, err = bcdb.NewKV2(path, SealLimit)
	default:
		err = statErr
	}
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open BlockchainDB at %s: %w", path, err)
	}
	return &Database{
		kv:            kv,
		views:         map[uint64]int{},
		shapes:        map[string]*shapeCount{},
		last:          map[[32]byte][]byte{},
		statsPath:     filepath.Join(path, "stats.json"),
		CompressEvery: 128,
		StatsEvery:    50,
	}, nil
}

// Begin begins a change set that reads the database as it stands now,
// and goes on reading it that way however much is committed elsewhere
// before the batch ends.
func (d *Database) Begin(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	d.mu.Lock()
	at := d.version
	d.views[at]++
	d.mu.Unlock()

	var once sync.Once
	release := func() { once.Do(func() { d.closeView(at) }) }

	return memory.NewChangeSet(memory.ChangeSetOptions{
		Prefix:  prefix,
		Get:     func(key *record.Key) ([]byte, error) { return d.getAt(at, key) },
		Commit:  func(e map[[32]byte]memory.Entry) error { defer release(); return d.commit(e) },
		ForEach: d.forEach,
		Discard: release,
	})
}

// closeView drops a batch's claim on its version and writes through
// whatever that unblocks
func (d *Database) closeView(at uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if n := d.views[at]; n <= 1 {
		delete(d.views, at)
	} else {
		d.views[at] = n - 1
	}
	if err := d.flush(); err != nil {
		// Nothing to return an error to: the caller is discarding a
		// batch.  The data is still staged and the next flush retries.
		d.flushErr = err
	}
}

// oldestView is the earliest version any open batch is reading at, and
// whether there is one.  The caller must hold the lock.
func (d *Database) oldestView() (uint64, bool) {
	var oldest uint64
	var found bool
	for v := range d.views {
		if !found || v < oldest {
			oldest, found = v, true
		}
	}
	return oldest, found
}

// flush writes through every staged batch that is already visible to
// every open reader.  The caller must hold the lock.
func (d *Database) flush() error {
	oldest, any := d.oldestView()
	for len(d.staged) > 0 {
		s := d.staged[0]
		if any && s.version > oldest {
			break // A reader predates this commit and must not see it
		}
		if err := d.writeThrough(s); err != nil {
			return err
		}
		d.staged = d.staged[1:]
	}
	return nil
}

// writeThrough puts a staged batch into the store and seals it.  The
// caller must hold the lock.
func (d *Database) writeThrough(s *staged) error {
	for key, value := range s.entries {
		if _, err := d.kv.Put(key, value); err != nil {
			return errors.UnknownError.WithFormat("put: %w", err)
		}
	}

	// A commit is the durability point, so it is what seals -- and
	// sealing is what makes the permanent layer's segments a unit a
	// peer can copy, which is the reason for the layer.
	if _, err := d.kv.Seal(s.version); err != nil {
		return errors.UnknownError.WithFormat("seal: %w", err)
	}
	if d.CompressEvery > 0 && s.version%d.CompressEvery == 0 {
		if err := d.kv.Compress(); err != nil {
			return errors.UnknownError.WithFormat("compress: %w", err)
		}
	}
	return nil
}

// Close closes the underlying database.
func (d *Database) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	d.StatsEvery, d.version = 1, d.version // Force a final tally
	d.reportStats()

	// Everything staged is now unreachable by any reader
	d.views = map[uint64]int{}
	if err := d.flush(); err != nil {
		return err
	}
	return d.kv.Close()
}

// Stats reports what the two layers were asked to do.  The permanent
// layer's counters are the interesting ones: it pays for a lookup on
// every write purely to discover whether the key is already there, so
// PutDuplicate over PutTotal says whether that lookup is earning its
// keep on a real workload.
func (d *Database) Stats() (perm, dyna bcdb.StoreStats) {
	return d.kv.PermKV.Stats(), d.kv.DynaKV.Stats()
}

// getAt reads a key as of a version: the newest staged write no later
// than that version, and otherwise the store, which by construction
// holds nothing newer.
func (d *Database) getAt(at uint64, key *record.Key) ([]byte, error) {
	key = d.prefix.AppendKey(key)
	h := key.Hash()

	d.mu.Lock()
	defer d.mu.Unlock()

	for i := len(d.staged) - 1; i >= 0; i-- {
		if d.staged[i].version > at {
			continue // Committed after this batch began
		}
		if value, ok := d.staged[i].entries[h]; ok {
			if len(value) == 0 {
				return nil, (*database.NotFoundError)(key)
			}
			return value, nil
		}
	}

	value, err := d.kv.Get(h)
	if err != nil || len(value) == 0 {
		// A zero-length value is a deletion, reported the same way a
		// key that was never written is
		return nil, (*database.NotFoundError)(key)
	}
	return value, nil
}

func (d *Database) commit(entries map[[32]byte]memory.Entry) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.flushErr != nil {
		return d.flushErr
	}

	staged := &staged{version: d.version + 1, entries: make(map[[32]byte][]byte, len(entries))}
	for _, e := range entries {
		key := d.prefix.AppendKey(e.Key)
		h := key.Hash()
		value := e.Value
		if e.Delete {
			value = []byte{} // Tombstone
		}
		staged.entries[h] = value
		d.tally(e.Key, h, value)
	}
	d.version = staged.version
	d.staged = append(d.staged, staged)
	if err := d.flush(); err != nil {
		return err
	}
	d.reportStats()
	return nil
}

// tally records a write against its key's shape: the first write of a
// key, a rewrite with the same bytes, or a rewrite with different
// bytes.  Only the third kind is data actually changing, and grouping
// it by shape says which records change -- rather than leaving it to be
// inferred from a total.  The caller must hold the lock.
func (d *Database) tally(key *record.Key, h [32]byte, value []byte) {
	shape := keyShape(key)
	c := d.shapes[shape]
	if c == nil {
		c = new(shapeCount)
		d.shapes[shape] = c
	}
	prev, seen := d.last[h]
	switch {
	case !seen:
		c.New++
	case bytes.Equal(prev, value):
		c.Duplicate++
	default:
		c.Rewritten++
	}
	d.last[h] = value
}

// reportStats writes what the two layers have been asked to do to
// stats.json beside the database, often enough to watch a run and
// rarely enough to cost nothing.  A file rather than the log, because
// the node's logging rules decide what survives and a measurement
// should not depend on that.
//
// PutDuplicate over PutTotal on the permanent layer is the number this
// is here for: the layer pays for a lookup on every write, and the only
// thing that lookup can discover is that the key is already present
// with the same value.  If that is rare, the lookup is a tax on every
// write to catch a case that does not happen.
//
// The caller must hold the lock.
func (d *Database) reportStats() {
	if d.StatsEvery == 0 || d.version%d.StatsEvery != 0 {
		return
	}
	perm, dyna := d.kv.PermKV.Stats(), d.kv.DynaKV.Stats()
	report := struct {
		Commits      uint64                 `json:"commits"`
		Perm         bcdb.StoreStats        `json:"perm"`
		Dyna         bcdb.StoreStats        `json:"dyna"`
		DuplicatePct float64                `json:"permDuplicatePct"`
		ConflictPct  float64                `json:"permConflictPct"`
		WalkPct      float64                `json:"permWalkPct"`
		Shapes       map[string]*shapeCount `json:"shapes"`
	}{Commits: d.version, Perm: perm, Dyna: dyna, Shapes: d.shapes}

	pct := func(n, of uint64) float64 {
		if of == 0 {
			return 0
		}
		return 100 * float64(n) / float64(of)
	}
	report.DuplicatePct = pct(perm.PutDuplicate, perm.PutTotal)
	report.ConflictPct = pct(perm.PutConflict, perm.PutTotal)
	report.WalkPct = pct(perm.FilterWalked, perm.LookupTotal)

	b, err := json.MarshalIndent(&report, "", "  ")
	if err != nil {
		return
	}
	// Written via a temporary file so a reader never sees half of it
	tmp := d.statsPath + ".tmp"
	if os.WriteFile(tmp, b, 0600) == nil {
		_ = os.Rename(tmp, d.statsPath)
	}
}

func (d *Database) forEach(fn func(*record.Key, []byte) error) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	seen := map[[32]byte]bool{}
	for i := len(d.staged) - 1; i >= 0; i-- {
		for key, value := range d.staged[i].entries {
			if seen[key] {
				continue
			}
			seen[key] = true
			if len(value) == 0 {
				continue // Deleted
			}
			if err := fn(record.KeyFromHash(key), value); err != nil {
				return err
			}
		}
	}
	return d.kv.ForEach(func(key [32]byte, value []byte) error {
		if seen[key] || len(value) == 0 {
			return nil
		}
		return fn(record.KeyFromHash(key), value)
	})
}
