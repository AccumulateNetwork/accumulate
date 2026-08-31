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
	"crypto/sha256"
	"encoding/json"
	stderrors "errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"

	bcdb "github.com/AccumulateNetwork/BlockchainDB/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
)

// Database is a key-value store backed by a BlockchainDB KV2.
type Database struct {
	mu sync.RWMutex

	// writeMu serializes write-throughs. It is taken WITHOUT mu held: the
	// write-through — puts, the per-block fsync of both layers, sealing,
	// compaction — used to run under mu, so every reader waited out every
	// commit. Measured on run 20260829T060833Z at 500 tps: read p50 2 ms,
	// round maxima of 1-2 s, always coinciding with a commit. A staged
	// batch stays readable from `staged` until the store has it, so the
	// lock is only needed to pop it afterwards.
	writeMu sync.Mutex
	kv      *bcdb.KV2
	prefix  *record.Key
	closed  bool

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

	// MergeLag is N, the active window (see DefaultMergeLag).  Set with
	// SetMergeLag so the store's window moves with it.
	MergeLag uint64

	statsPath string // Where reportStats writes

	// shapes tallies writes by key shape.  Classifying a rewrite means
	// knowing what the key held before, and asking the store would
	// perturb the very counters being measured, so a digest of the
	// last value written for each key is remembered here instead --
	// a digest and not the value, because remembering the values is
	// keeping a second copy of the database in memory.
	shapes map[string]*ShapeCount
	last   map[[32]byte][32]byte

	// Background maintenance (see maintain).
	maintaining  atomic.Bool
	maintWG      sync.WaitGroup
	maintErr     error  // the last maintenance run's outcome
	maintErrs    uint64 // how many runs failed
	maintainHook func() // tests: runs at the start of a maintenance run

	// TallyKeys bounds last.  Beyond it new keys are still counted but
	// not remembered, so their later rewrites cannot be classified --
	// the report says so (tallyCapped).  Unbounded, the tally was ~120
	// bytes per key ever written for the life of the process (#4175).
	TallyKeys int

	// dyna holds the keys that must go to the dynamic layer whatever
	// their classification says: keys that have been deleted, and keys
	// the permanent layer refused.  Both leave a value in the dynamic
	// layer, and the dynamic layer is read first, so a later write to
	// the permanent layer would be shadowed by what is already there.
	//
	// This is only ever the exceptions.  A correctly classified
	// database never adds to it.
	//
	// It is persisted (#4173): the shadowing it guards against is a
	// property of the store, so forgetting it across a restart brings
	// the shadowing back -- a deleted key rewritten after a restart
	// would go to the permanent layer and read as deleted.  Each new
	// exception is appended to exceptionsPath before the batch that
	// introduced it is written through, so the file can name a key the
	// store never got (harmless: that key merely stays dynamic) but
	// never the reverse.
	dyna           map[[32]byte]bool
	exceptionsPath string
	pendingDyna    [][32]byte // Exceptions not yet appended to the file
}

// staged is one committed batch that has not reached the store yet
type staged struct {
	version uint64
	entries map[[32]byte]entry
}

// entry is one staged write, and where its key says it belongs.  The
// layer is decided at commit, while the key path is still in hand: by
// write-through the key is a hash and nothing can be told from it.
type entry struct {
	value []byte // A zero-length value is a deletion
	perm  bool   // Write-once: goes to the permanent layer
	shape string // The key's shape, so a refusal can be attributed
}

var _ keyvalue.Beginner = (*Database)(nil)

// SealLimit is the number of records a layer accumulates before it
// seals on its own.  A commit seals the permanent layer anyway, so this
// only bounds a layer that fills between commits.
const SealLimit = 100_000

// DefaultMergeLag is N: how many of the newest blocks are the store's
// active tier — its own segments, under the protocol's lock — and the
// watermark below which history is merged and compacted (see
// writeThrough).  The consensus path only ever reads recent blocks, so
// this is the hot window; it was 512, and at one segment per block that
// let a node accumulate 1,052 sealed segments before the first merge —
// long enough for the segment walk on every hit to stretch blocks from
// 3 s to 5 s (run 20260829T060833Z, BlockchainDB#50).  Since #57 the
// store enforces the same N as the boundary maintenance may not cross
// with a lock; it is set at creation (Open) and recorded on disk.
const DefaultMergeLag = 64

// DefaultTallyKeys is how many keys the per-shape tally remembers a
// digest for: about 128 MB at the default.
const DefaultTallyKeys = 1 << 20

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
		if err == nil {
			// N, set once at creation and recorded in the manifests: the
			// store's key-filter window AND the line between its active
			// tier (the protocol's lock) and history (the maintainer's)
			// (BlockchainDB#57). MergeBelow and Compress work on history
			// only, so the adapter's merge watermark and the store's window
			// are the same number, or nothing ever merges.
			err = kv.SetFilterBlocks(DefaultMergeLag)
		}
	default:
		err = statErr
	}
	if err != nil {
		return nil, errors.UnknownError.WithFormat("open BlockchainDB at %s: %w", path, err)
	}
	d := &Database{
		kv:             kv,
		views:          map[uint64]int{},
		shapes:         map[string]*ShapeCount{},
		last:           map[[32]byte][32]byte{},
		dyna:           map[[32]byte]bool{},
		statsPath:      filepath.Join(path, "stats.json"),
		exceptionsPath: filepath.Join(path, "dyna-exceptions"),
		CompressEvery:  128,
		StatsEvery:     50,
		TallyKeys:      DefaultTallyKeys,
		MergeLag:       DefaultMergeLag,
	}

	// A commit seals the permanent layer at its version, and the store
	// refuses to seal a block it has already closed -- so the version
	// must resume where the store stands, not at zero (#4173).  The
	// manifest records the block currently being accumulated, which is
	// one past the last version sealed.
	d.version, err = sealedVersion(filepath.Join(path, permManifest))
	if err != nil {
		_ = kv.Close()
		return nil, errors.UnknownError.WithFormat("read %s: %w", permManifest, err)
	}
	err = d.loadExceptions()
	if err != nil {
		_ = kv.Close()
		return nil, errors.UnknownError.WithFormat("read %s: %w", d.exceptionsPath, err)
	}
	return d, nil
}

// SetMergeLag sets N for the adapter and the store together.  Meant for
// a freshly created database; the store rebuilds its filters and
// commits a manifest per layer, and refuses anything below its minimum.
func (d *Database) SetMergeLag(n uint64) error {
	if err := d.kv.SetFilterBlocks(n); err != nil {
		return err
	}
	d.MergeLag = n
	return nil
}

// sealedVersion reads the version the permanent layer was last sealed
// at.  A store that has never sealed reports zero, as does one that has
// no manifest yet.
func sealedVersion(manifest string) (uint64, error) {
	b, err := os.ReadFile(manifest)
	switch {
	case stderrors.Is(err, fs.ErrNotExist):
		return 0, nil
	case err != nil:
		return 0, err
	}
	var m bcdb.StoreManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return 0, err
	}
	if m.BlockHeight == 0 {
		return 0, nil
	}
	return m.BlockHeight - 1, nil
}

// loadExceptions restores the dynamic-layer exception set.  The file is
// a sequence of 32-byte key hashes; a partial trailing record -- a crash
// mid-append -- is ignored, which errs towards the store's own state.
func (d *Database) loadExceptions() error {
	b, err := os.ReadFile(d.exceptionsPath)
	switch {
	case stderrors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	}
	for ; len(b) >= 32; b = b[32:] {
		var h [32]byte
		copy(h[:], b)
		d.dyna[h] = true
	}
	return nil
}

// persistExceptions appends the exceptions recorded since the last
// write-through and syncs them, so a restart cannot forget them.  Takes
// the lock only to hand the pending list over; on failure the list is
// put back so the next write-through retries it.
func (d *Database) persistExceptions() error {
	d.mu.Lock()
	pending := d.pendingDyna
	d.pendingDyna = nil
	d.mu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	err := appendHashes(d.exceptionsPath, pending)
	if err != nil {
		d.mu.Lock()
		d.pendingDyna = append(pending, d.pendingDyna...)
		d.mu.Unlock()
	}
	return err
}

func appendHashes(path string, hashes [][32]byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	for _, h := range hashes {
		if _, err := f.Write(h[:]); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// except marks a key as belonging to the dynamic layer from now on.  The
// caller must hold the lock.
func (d *Database) except(h [32]byte) {
	if d.dyna[h] {
		return
	}
	d.dyna[h] = true
	d.pendingDyna = append(d.pendingDyna, h)
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
		Prefix: prefix,
		Get:    func(key *record.Key) ([]byte, error) { return d.getAt(at, key) },
		// The view is released BEFORE the commit, not after (#4173).
		// While a batch still pins its version, flush stops short of
		// its own commit; it was written through by the deferred
		// release instead, where an error had nowhere to go but the
		// next Commit -- so the executor saw a block committed that had
		// not reached the store, and the error named the wrong block.
		// The batch has finished reading by the time it commits, so its
		// view is not needed.
		Commit:  func(e map[[32]byte]memory.Entry) error { release(); return d.commit(e) },
		ForEach: func(fn func(*record.Key, []byte) error) error { return d.forEachAt(at, fn) },
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
	// Releasing a view may unblock staged commits, but nothing is
	// written here: the next commit (or Close) drains them, and reports
	// the outcome to a caller that can act on it.
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

// drain writes through every staged batch that is already visible to
// every open reader, oldest first. Called WITHOUT the lock held; it takes
// the lock only to look at the queue and to pop a batch once the store has
// it. A batch under write-through is still in staged, so a reader that
// should see it still does — from staging.
func (d *Database) drain() error {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	for {
		d.mu.Lock()
		if len(d.staged) == 0 {
			d.mu.Unlock()
			return nil
		}
		s := d.staged[0]
		if oldest, any := d.oldestView(); any && s.version > oldest {
			d.mu.Unlock()
			return nil // A reader predates this commit and must not see it
		}
		d.mu.Unlock()

		if err := d.writeThrough(s); err != nil {
			return err
		}

		d.mu.Lock()
		d.staged = d.staged[1:]
		d.mu.Unlock()
	}
}

// writeThrough puts a staged batch into the store and seals it.  Runs
// under writeMu, not mu; the shared maps it touches are guarded inside.
func (d *Database) writeThrough(s *staged) error {
	// Exceptions first: a key named here that the store never received
	// is harmless, a key the store received that is forgotten here is
	// the #4173 shadowing bug.
	if err := d.persistExceptions(); err != nil {
		return errors.UnknownError.WithFormat("persist exceptions: %w", err)
	}
	for key, e := range s.entries {
		if err := d.putRouted(key, e); err != nil {
			return errors.UnknownError.WithFormat("put: %w", err)
		}
	}

	// And again after: a refusal discovered by the puts above added an
	// exception that must be on disk before the seal says this version
	// is durable.
	if err := d.persistExceptions(); err != nil {
		return errors.UnknownError.WithFormat("persist exceptions: %w", err)
	}

	// A commit is the durability point, so it is what seals -- and
	// sealing is what makes the permanent layer's segments a unit a
	// peer can copy, which is the reason for the layer.
	if _, err := d.kv.Seal(s.version); err != nil {
		return errors.UnknownError.WithFormat("seal: %w", err)
	}
	if d.CompressEvery > 0 && s.version%d.CompressEvery == 0 {
		d.maintain(s.version)
	}
	return nil
}

// maintain compacts history and merges finished block sets, in the
// background.
//
// It used to run inline, here, on the committing goroutine — which is
// the block producer's.  BlockchainDB#57/#58 took the store lock off
// maintenance so that OTHER readers and writers proceed during a copy,
// and the single-store benchmark showed exactly that; but the caller
// still waited for its own call to return, so every node's block
// production stopped for the length of the compaction: 88–100 s at
// block 400 in run 20260830T054402Z, the goroutine dump reading
// blockProductionLoop → ProduceBlock → commit → drain → writeThrough →
// Compress → CompactHistory.  Maintenance on history needs nothing from
// the protocol path, so the protocol path must not wait for it.
//
// One maintenance run at a time; a cadence that lands while one is
// still running is skipped, not queued (history does not go anywhere).
// Close waits for a run in flight.  An error is recorded for stats.json
// and the next cadence retries.
func (d *Database) maintain(version uint64) {
	if !d.maintaining.CompareAndSwap(false, true) {
		return
	}
	d.maintWG.Add(1)
	go func() {
		defer d.maintWG.Done()
		defer d.maintaining.Store(false)
		if d.maintainHook != nil {
			d.maintainHook()
		}
		var err error
		if err = d.kv.Compress(); err != nil {
			err = errors.UnknownError.WithFormat("compress: %w", err)
		} else if version > d.MergeLag {
			// Every block seal leaves one permanent segment per shard,
			// so a node accumulates a file pair per block forever
			// (BlockchainDB#30, #47).  MergeBelow folds a finished block
			// set into one segment.  The watermark is held MergeLag
			// blocks back — the active window — below which nothing more
			// arrives and nothing is still being healed.
			if _, _, err = d.kv.MergeBelow(version - d.MergeLag); err != nil {
				err = errors.UnknownError.WithFormat("merge sealed segments: %w", err)
			}
		}
		d.mu.Lock()
		d.maintErr = err
		if err != nil {
			d.maintErrs++
		}
		d.mu.Unlock()
	}()
}

// Close closes the underlying database.
func (d *Database) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	d.StatsEvery = 1 // Force a final tally
	d.reportStats()

	// Everything staged is now unreachable by any reader
	d.views = map[uint64]int{}
	d.mu.Unlock()
	err := d.drain()
	d.maintWG.Wait() // a compaction or merge in flight finishes first
	d.mu.Lock()
	if err != nil {
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

// Shapes reports the per-shape tally: for every shape of key the
// database has been asked to write, which layer it was routed to and
// what happened to those writes.  The map is a copy, so a caller can
// read it while the database goes on running.
func (d *Database) Shapes() map[string]ShapeCount {
	d.mu.Lock()
	defer d.mu.Unlock()
	shapes := make(map[string]ShapeCount, len(d.shapes))
	for shape, c := range d.shapes {
		shapes[shape] = *c
	}
	return shapes
}

// getAt reads a key as of a version: the newest staged write no later
// than that version, and otherwise the store, which by construction
// holds nothing newer.
func (d *Database) getAt(at uint64, key *record.Key) ([]byte, error) {
	key = d.prefix.AppendKey(key)
	h := key.Hash()

	// A read lock: readers only look at staged, and the store has its
	// own lock.  One exclusive mutex here put every API query behind
	// every commit (#4175).
	d.mu.RLock()
	defer d.mu.RUnlock()

	for i := len(d.staged) - 1; i >= 0; i-- {
		if d.staged[i].version > at {
			continue // Committed after this batch began
		}
		if e, ok := d.staged[i].entries[h]; ok {
			if len(e.value) == 0 {
				return nil, (*database.NotFoundError)(key)
			}
			return e.value, nil
		}
	}

	value, err := d.kv.Get(h)
	if err != nil {
		// Get stops at the window for the immutable layer (BlockchainDB
		// 5c7b0e5): permanent data older than 2N is history, and probing
		// it per segment was 23% of a validator's CPU. But this adapter
		// is the ONE read path — the API serving an explorer, healing
		// re-proving an old range, and a signature naming an old
		// transaction all come through here and must see deep history.
		// So a miss falls back to GetDeep. The consensus path's hot
		// reads are recent and never reach this line; the cost lands on
		// reads that are genuinely absent or genuinely old, which is
		// the store's history-index problem (BlockchainDB#59) to shrink.
		value, err = d.kv.GetDeep(h)
	}
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

	staged := &staged{version: d.version + 1, entries: make(map[[32]byte]entry, len(entries))}
	for _, e := range entries {
		key := d.prefix.AppendKey(e.Key)
		h := key.Hash()
		value := e.Value
		if e.Delete {
			value = []byte{} // Tombstone
		}

		// A deletion is a mutation whatever the record is, and a key
		// the dynamic layer already holds has to stay there: it is
		// read first, so a later write to the permanent layer would be
		// shadowed by what the dynamic layer already has -- by a
		// tombstone, that means reading as deleted while holding a
		// value.
		if len(value) == 0 {
			d.except(h)
		}
		perm := len(value) > 0 && !d.dyna[h] && isWriteOnce(key)

		shape := d.tally(key, h, value, perm)
		staged.entries[h] = entry{value: value, perm: perm, shape: shape}
	}
	d.version = staged.version
	d.staged = append(d.staged, staged)
	d.reportStats()
	d.mu.Unlock()

	// Write through with the lock RELEASED, so readers proceed against
	// staging while the store does its fsyncs; then re-take it for the
	// deferred unlock.
	err := d.drain()
	d.mu.Lock()
	return err
}

// tally records a write against its key's shape: the first write of a
// key, a rewrite with the same bytes, or a rewrite with different
// bytes.  Only the third kind is data actually changing, and grouping
// it by shape says which records change -- rather than leaving it to be
// inferred from a total.  It returns the shape, which the write path
// carries so that a refusal by the permanent layer can be attributed
// to it.  The caller must hold the lock.
func (d *Database) tally(key *record.Key, h [32]byte, value []byte, perm bool) string {
	shape := keyShape(key)
	c := d.shapes[shape]
	if c == nil {
		c = new(ShapeCount)
		c.Layer = "dyna"
		if perm {
			c.Layer = "perm"
		}
		d.shapes[shape] = c
	}
	digest := sha256.Sum256(value)
	prev, seen := d.last[h]
	switch {
	case !seen:
		c.New++
		if len(d.last) >= d.TallyKeys {
			return shape // Counted, not remembered
		}
	case prev == digest:
		c.Duplicate++
	default:
		c.Rewritten++
	}
	d.last[h] = digest
	return shape
}

// putRouted writes an entry to the layer its key was classified into,
// and treats the permanent layer's refusal as a finding rather than as
// a failure.
//
// The permanent layer refuses to overwrite a key with a different
// value, and that refusal is precisely the evidence that a record
// classified write-once is not.  Failing the commit would take a node
// down over a misclassification and would surface exactly one of them
// per run, when what is wanted is the list.  So the write goes to the
// dynamic layer -- where it belonged -- the key is remembered so its
// later writes go there too, and the shape it came from is counted so
// the report names it.
//
// Only the refusal is handled this way.  Any other error is a failure
// of the store and is returned.
//
// Runs under writeMu; takes mu for the exception bookkeeping only.
func (d *Database) putRouted(key [32]byte, e entry) error {
	if !e.perm {
		_, err := d.kv.PutDyna(key, e.value)
		return err
	}

	_, err := d.kv.PutPerm(key, e.value)
	switch {
	case err == nil:
		return nil
	case !isRefusal(err):
		return err
	}

	// The store says this record is not write-once
	d.mu.Lock()
	d.except(key)
	if c := d.shapes[e.shape]; c != nil {
		c.Misrouted++
	}
	d.mu.Unlock()
	_, err = d.kv.PutDyna(key, e.value)
	return err
}

// isRefusal reports whether err is the permanent layer declining to
// overwrite a key, rather than the store failing.  BlockchainDB#28
// gave the refusal a sentinel; anything else is a store failure and
// the commit fails loudly.
func isRefusal(err error) bool {
	return stderrors.Is(err, bcdb.ErrImmutable)
}

// reportStats writes what the two layers have been asked to do to
// stats.json beside the database, often enough to watch a run and
// rarely enough to cost nothing.  A file rather than the log, because
// the node's logging rules decide what survives and a measurement
// should not depend on that.
//
// Two numbers are what this is for.
//
// PutDuplicate over PutTotal on the permanent layer: now that writes
// are routed by classification rather than discovered, that ratio
// finally means what it says -- how often genuinely write-once data is
// written twice.  The layer pays for a lookup on every write and the
// only thing that lookup can find is such a duplicate, so if the ratio
// is small the lookup is a tax on every write to catch a case that
// does not happen, and the layer can become a pure append with
// duplicates reconciled at merge time.
//
// Misrouted, per shape: the writes the permanent layer refused.  Each
// one is a record isWriteOnce calls write-once and that Accumulate
// rewrites, named by its shape rather than left to be inferred from an
// aggregate.  On a correct classification this list is empty.
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
		Misrouted    []string               `json:"misroutedShapes"`
		Shapes       map[string]*ShapeCount `json:"shapes"`

		// Staged is how many commits are waiting on an open reader
		// before they can reach the store.  Growing without bound means
		// a batch was never closed -- and nothing staged is on disk.
		Staged      int  `json:"stagedCommits"`
		TallyKeys   int  `json:"tallyKeys"`
		TallyCapped bool `json:"tallyCapped"`

		// Maintenance runs in the background; a failure is recorded here
		// rather than failing a commit.
		MaintenanceErrors uint64 `json:"maintenanceErrors"`
		MaintenanceLast   string `json:"maintenanceLastError,omitempty"`
	}{Commits: d.version, Perm: perm, Dyna: dyna, Shapes: d.shapes,
		Staged: len(d.staged), TallyKeys: len(d.last), TallyCapped: len(d.last) >= d.TallyKeys,
		MaintenanceErrors: d.maintErrs}
	if d.maintErr != nil {
		report.MaintenanceLast = d.maintErr.Error()
	}

	// Lifted out of the shape table so a run can be checked by looking
	// at one field
	for shape, c := range d.shapes {
		if c.Misrouted > 0 {
			report.Misrouted = append(report.Misrouted, shape)
		}
	}
	sort.Strings(report.Misrouted)

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

// forEachAt visits every key as of a version: staged writes no later
// than it, then the store, which holds nothing newer than the oldest
// open view -- and the caller's own view is at least that old.
//
// The lock is not held across the callback (#4175).  Staged entries
// are snapshotted under a read lock and visited after it is released,
// so a callback may read the database and does not stall commits.  The
// store's own ForEach does hold the store's mutex across the callback,
// so a callback must not read the STORE (a staged hit is fine, a store
// read deadlocks) -- BlockchainDB#31.
func (d *Database) forEachAt(at uint64, fn func(*record.Key, []byte) error) error {
	type kv struct {
		key   [32]byte
		value []byte
	}
	d.mu.RLock()
	seen := map[[32]byte]bool{}
	var snapshot []kv
	for i := len(d.staged) - 1; i >= 0; i-- {
		if d.staged[i].version > at {
			continue // Committed after this batch began
		}
		for key, e := range d.staged[i].entries {
			if seen[key] {
				continue
			}
			seen[key] = true
			if len(e.value) == 0 {
				continue // Deleted
			}
			snapshot = append(snapshot, kv{key, e.value})
		}
	}
	d.mu.RUnlock()

	for _, e := range snapshot {
		if err := fn(record.KeyFromHash(e.key), e.value); err != nil {
			return err
		}
	}
	return d.kv.ForEach(func(key [32]byte, value []byte) error {
		if seen[key] || len(value) == 0 {
			return nil
		}
		return fn(record.KeyFromHash(key), value)
	})
}
