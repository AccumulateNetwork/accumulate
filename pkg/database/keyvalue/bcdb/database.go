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
	"encoding/json"
	stderrors "errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	kv      *bcdb.KVShard
	prefix  *record.Key
	closed  bool

	// version counts commits.  A batch records the version it began at
	// and reads as of that version; a commit produces the next one.
	version uint64

	// undo holds, for each committed version, the value every key it
	// rewrote had BEFORE that commit -- what a reader begun at an older
	// version must go on seeing (database invariant 2). A commit reaches
	// the store at commit (invariant 5); this is only the memory of what
	// it replaced, kept while a reader that predates it is open and
	// dropped when the last such reader closes (D5). An empty pre-image
	// means the key did not exist before.
	undo         map[uint64]map[[32]byte][]byte
	undoVersions []uint64 // keys of undo, ascending

	// views counts the open batches at each version, so flushing knows
	// what the oldest reader can still see. viewOpened is when the first
	// batch at each version was begun, so the age of the oldest open view
	// can be reported: a view older than a block is what holds staged
	// commits in memory and off disk (DIFFERENCES D5).
	views      map[uint64]int
	viewOpened map[uint64]time.Time
	viewOpener map[uint64]string // who begun the first view at each version

	// ViewWarnAfter is how old the oldest open view may get before a
	// commit says who holds it. Zero disables the warning.
	ViewWarnAfter time.Duration
	lastViewWarn  time.Time

	// metricLabel names this database in the staging gauges: the directory
	// two above the store, "dnn" or "bvnn" on a node.
	metricLabel string

	// CompressEvery seals and compacts the dynamic layer after this
	// many commits.  Zero leaves compaction to the caller.
	CompressEvery uint64

	// PackEvery is how many blocks pass between cross-shard packs; see
	// the constant of the same name.  Zero disables packing.
	PackEvery uint64

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
	// last value written is remembered here instead -- for a SAMPLE of
	// keys, not all of them.  See tally.
	shapes map[string]*ShapeCount

	// urls caches Account(U).Url, chains caches the synthetic and anchor
	// chain records.  Both hold immutable records; see cache.go.
	urls   *immutableCache
	chains *immutableCache
	last   map[[32]byte][32]byte

	// Background maintenance (see maintain).
	// deepFallbacks counts, by record shape, the reads a SHALLOW batch
	// could only answer from history: the evidence for whether the
	// window can be enforced without a fallback (see getAt).  Under
	// fallbackMu, a leaf lock -- the counter is bumped while getAt
	// holds d.mu shared, and it must not reach for that lock.
	deepFallbacks map[string]uint64
	fallbackMu    sync.Mutex

	maintaining  atomic.Bool
	maintWG      sync.WaitGroup
	maintErr     error  // the last maintenance run's outcome
	maintErrs    uint64 // how many runs failed
	maintainHook func() // tests: runs at the start of a maintenance run

	// TallySample is the sampling rate: one key in TallySample is
	// remembered.  One is every key.
	TallySample uint8

	// TallyKeys bounds last as a backstop.  With sampling the bound is
	// not normally reached; if it is, the report says so (tallyCapped)
	// and the sample stops growing.
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

// staged is one committed batch on its way to the store
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

// SealLimit is the number of records ONE SHARD's layer accumulates
// before it seals on its own.  A commit seals the permanent layer
// anyway, so this only bounds a layer that fills between commits.
//
// It is PER SHARD, which is what makes the old single-store figure the
// wrong number to carry across: at the store's eight shards, 100,000
// would be 800,000 records of unsealed tail for the partition, every
// one of them held in a live map and replayed on open.  Dividing keeps
// the partition where it was -- 12,500 a shard is ~100,000 in total --
// and keys route by hash, so each shard fills at its own eighth of the
// rate.
const SealLimit = 12_500

// PackEvery is how many blocks pass between cross-shard packs.
//
// A block boundary seals a segment in every shard that took writes,
// and a per-shard merge folds 20 blocks into one.  A shard pays ~58
// files whatever share of the rate it sees, so the total is the shard
// count times that, and it grows with the chain until something folds
// the merges away.  Packing folds every shard's into ONE file, against
// inodes that cannot be grown after mkfs (BlockchainDB#47).
//
// A thousand blocks rather than a day: the same bytes move either way
// -- ~2 MB/s at 5,000 entries a block -- so a longer period buys only
// fewer set files, and the per-day group filters make that worth
// almost nothing (451 probes a year against 369). What it costs is
// real: 11x the files on disk, 20 GB written in one pass instead of
// 1 GB, and 5 1/2 hours of blocks unpacked instead of 17 minutes.
const PackEvery = 1000

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
const DefaultMergeLag = 20

// DefaultTallyKeys is the backstop bound on the tally's sample.
const DefaultTallyKeys = 1 << 16

// DefaultTallySample is how many keys the tally sees for each one it
// remembers.
//
// The tally answers a RATIO -- of the writes to this shape, how many
// changed the value -- and a ratio does not need a census.  Measured on
// a 500 tx/s soak, remembering every key cost 192 MB, 38% of the live
// heap and the largest single consumer on the node, for a statistic
// that is a ratio (#4165).
//
// Sampling by KEY HASH rather than by arrival is what makes the sample
// unbiased and is the real fix.  The previous bound admitted keys until
// it was full, after which an unremembered key counted New and stayed
// unremembered -- so every later rewrite of it counted New too, and the
// longer a run went the more New inflated and Rewritten collapsed.  A
// hash-selected key is in the sample for the life of the process, so
// its rewrites are always classified, and the estimate stays true.
const DefaultTallySample = 128

// permManifest is the file whose presence means a database is already
// here: the permanent layer's segment manifest of the first shard.
// Every shard is created together, so shard zero answers for all of
// them.
var permManifest = filepath.Join("Shard0000", "perm", "segments.json")

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

	var kv *bcdb.KVShard
	var err error
	switch _, statErr := os.Stat(filepath.Join(path, permManifest)); {
	case statErr == nil:
		kv, err = bcdb.OpenKVShard(path)
	case stderrors.Is(statErr, fs.ErrNotExist):
		kv, err = bcdb.NewKVShard(path, SealLimit)
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
		urls:           newImmutableCache(cacheGenSize),
		chains:         newImmutableCache(cacheGenSize),
		last:           map[[32]byte][32]byte{},
		dyna:           map[[32]byte]bool{},
		statsPath:      filepath.Join(path, "stats.json"),
		exceptionsPath: filepath.Join(path, "dyna-exceptions"),
		CompressEvery:  20,
		PackEvery:      PackEvery,
		StatsEvery:     50,
		TallyKeys:      DefaultTallyKeys,
		TallySample:    DefaultTallySample,
		MergeLag:       DefaultMergeLag,
		metricLabel:    metricLabelFor(path),
		ViewWarnAfter:  10 * time.Second,
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
	return d.begin(prefix, writable, false)
}

// BeginDeep is Begin for a reader that knowingly reaches back past the
// store's window: the API serving an explorer, healing re-proving an
// old range, a tool walking history.
//
// The permanent layer answers a protocol read from the last N to 2N
// blocks and calls anything older absent (BlockchainDB spec 1.3):
// probing per segment on every miss was 23% of a validator's CPU and
// grew with the segment count.  The data is still there; reaching it
// is a deeper read, and this is how a caller asks for one.  The cost
// lands on the callers that need it instead of on every block.
func (d *Database) BeginDeep(prefix *record.Key, writable bool) keyvalue.ChangeSet {
	return d.begin(prefix, writable, true)
}

func (d *Database) begin(prefix *record.Key, writable, deep bool) keyvalue.ChangeSet {
	d.mu.Lock()
	at := d.version
	d.views[at]++
	if d.viewOpened == nil {
		d.viewOpened = map[uint64]time.Time{}
	}
	if _, ok := d.viewOpened[at]; !ok {
		d.viewOpened[at] = time.Now()
		if d.viewOpener == nil {
			d.viewOpener = map[uint64]string{}
		}
		d.viewOpener[at] = viewOpener()
	}
	d.mu.Unlock()

	var once sync.Once
	release := func() { once.Do(func() { d.closeView(at) }) }

	return memory.NewChangeSet(memory.ChangeSetOptions{
		Prefix: prefix,
		Get:    func(key *record.Key) ([]byte, error) { return d.getAt(at, key, deep) },
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
		delete(d.viewOpened, at)
		delete(d.viewOpener, at)
		d.pruneUndo()
	} else {
		d.views[at] = n - 1
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
	if err := d.kv.SealBlock(s.version); err != nil {
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
			// (BlockchainDB#30, #47).  MergeFinalized folds each shard's
			// finished blocks into one segment.  The watermark is held
			// MergeLag blocks back — the active window — below which
			// nothing more arrives and nothing is still being healed.
			if _, err = d.kv.MergeFinalized(version - d.MergeLag); err != nil {
				err = errors.UnknownError.WithFormat("merge sealed segments: %w", err)
			} else if d.PackEvery > 0 && version >= d.PackEvery+d.MergeLag &&
				version%d.PackEvery < d.CompressEvery {
				// And fold every shard's merges into ONE file, on a
				// period of its own.  Per-shard merging alone still
				// leaves ~25 files a block at 512 shards; the pack is
				// what makes the file count survivable, and it takes
				// pins rather than locks, so the shards commit while it
				// runs (BlockchainDB#47).
				//
				// The watermark is the merge's, so a pack never takes a
				// segment the window still holds. The modulus is against
				// the maintenance cadence, not equality, because
				// maintenance runs every CompressEvery commits and a
				// run that lands while another is going is skipped.
				if _, _, err = d.kv.PackFinalized(version - d.MergeLag); err != nil {
					err = errors.UnknownError.WithFormat("pack finalized blocks: %w", err)
				}
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

	d.mu.Unlock()
	d.maintWG.Wait() // a compaction or merge in flight finishes first
	d.mu.Lock()
	return d.kv.Close()
}

// Stats reports what the two layers were asked to do.  The permanent
// layer's counters are the interesting ones: it pays for a lookup on
// every write purely to discover whether the key is already there, so
// PutDuplicate over PutTotal says whether that lookup is earning its
// keep on a real workload.
func (d *Database) Stats() (perm, dyna bcdb.StoreStats) {
	return d.kv.Stats()
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

// DeepFallbacks reports, by key shape, the reads that the permanent
// layer's window could not answer and that GetDeep had to walk history
// for.  It is the read-side counterpart to Shapes: a shape that appears
// here in quantity is one whose PLACEMENT is wrong, whatever its write
// classification says (see route.go's Url case).
func (d *Database) DeepFallbacks() map[string]uint64 {
	return d.fallbackSnapshot()
}

// fallbackSnapshot copies the deep-fallback counters for a report
func (d *Database) fallbackSnapshot() map[string]uint64 {
	d.fallbackMu.Lock()
	defer d.fallbackMu.Unlock()
	if len(d.deepFallbacks) == 0 {
		return nil
	}
	out := make(map[string]uint64, len(d.deepFallbacks))
	for k, v := range d.deepFallbacks {
		out[k] = v
	}
	return out
}

// getAt reads a key as of a version: the newest staged write no later
// than that version, and otherwise the store, which by construction
// holds nothing newer.
func (d *Database) getAt(at uint64, key *record.Key, deep bool) ([]byte, error) {
	key = d.prefix.AppendKey(key)
	h := key.Hash()

	// A read lock: readers only look at staged, and the store has its
	// own lock.  One exclusive mutex here put every API query behind
	// every commit (#4175).
	d.mu.RLock()
	defer d.mu.RUnlock()

	// A commit after this batch began may have rewritten the key. The
	// earliest such commit remembers what the key held before it, which
	// is the value at this batch's version.
	if pre, ok := d.preImageAt(at, h); ok {
		if len(pre) == 0 {
			return nil, (*database.NotFoundError)(key)
		}
		return pre, nil
	}

	// Cached shapes first, and on a miss go straight to the layer that
	// holds them (#4165). Both caches hold records that cannot change,
	// which is why they need no invalidation -- see cache.go.
	if kind := cacheKindOf(key); kind != cacheNone {
		c := d.urls
		if kind == cacheChain {
			c = d.chains
		}
		if v, ok := c.get(h); ok {
			return v, nil
		}

		var v []byte
		var err error
		if kind == cacheURL {
			// Dynamic, not the windowed permanent layer: this is
			// written once at account creation and read on every touch
			// of the account, so in perm it ages out and every read
			// after that is a history walk.
			v, err = d.kv.GetDyna(h)
		} else {
			v, err = d.kv.GetPerm(h)
		}
		if err == nil {
			c.put(h, v)
			return v, nil
		}
		// Fall through: a record not where it was expected is still a
		// record, and refusing it here would turn a placement mistake
		// into a missing read.
	}

	value, err := d.kv.Get(h)
	if err != nil && deep {
		// This reader asked to reach past the window (BeginDeep)
		value, err = d.kv.GetDeep(h)
	} else if err != nil {
		// A shallow reader missed.  The window is the protocol's
		// horizon and a miss here is meant to BE the answer -- but
		// turning that on blind would turn any read this adapter has
		// not accounted for into a silent not-found, which in the
		// executor is a consensus fault. So the fallback still runs,
		// and every use of it is counted and shaped: DeepFallbacks in
		// stats.json says how often a shallow reader needed history,
		// and for which kind of record.
		//
		// Zero over a soak is the evidence that the fallback can be
		// removed and the window enforced; anything else names the
		// call sites that must use BeginDeep first.
		if v2, err2 := d.kv.GetDeep(h); err2 == nil {
			// Its own lock, NOT d.mu: this runs while getAt holds
			// d.mu.RLock, and a Go RWMutex is not reentrant -- taking
			// it exclusively here deadlocked the read path against
			// itself.  A leaf mutex over one map is also what keeps a
			// diagnostic counter off the commit lock, which is what
			// #4175 took the read path off.
			d.fallbackMu.Lock()
			if d.deepFallbacks == nil {
				d.deepFallbacks = map[string]uint64{}
			}
			d.deepFallbacks[keyShape(key)]++
			d.fallbackMu.Unlock()
			value, err = v2, nil
		}
	}
	if err != nil || len(value) == 0 {
		// A zero-length value is a deletion, reported the same way a
		// key that was never written is
		return nil, (*database.NotFoundError)(key)
	}
	return value, nil
}

// commit writes a batch through to the store and seals it, so that when
// it returns the batch is durable (database invariant 5). Readers begun
// before it are kept isolated (invariant 2) by remembering, under this
// commit's version, what every key it rewrites held before -- see undo.
//
// Commits are serialized by writeMu. The version is bumped only after the
// store has the batch, and the pre-images are installed before the store
// is touched, so a reader that begins during the write-through is at the
// previous version and sees a consistent state whichever keys have
// landed. mu is held only around the shared maps, never across I/O.
func (d *Database) commit(entries map[[32]byte]memory.Entry) error {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()

	d.mu.Lock()
	version := d.version + 1
	staged := &staged{version: version, entries: make(map[[32]byte]entry, len(entries))}
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

		shape := d.tally(key, perm)
		staged.entries[h] = entry{value: value, perm: perm, shape: shape}

		// Write through to the caches. They hold records that cannot
		// change, so this should never overwrite a different value --
		// but a cache that goes stale when the assumption is broken
		// turns a wrong classification into wrong DATA, served forever
		// and silently. Writing through costs one map store on a path
		// that is already walking every entry, and means the caches
		// cannot disagree with the store whatever the shapes do.
		if kind := cacheKindOf(key); kind != cacheNone {
			c := d.urls
			if kind == cacheChain {
				c = d.chains
			}
			if len(value) == 0 {
				c.drop(h) // A tombstone is not a value to serve
			} else {
				c.put(h, value)
			}
		}
	}
	readers := len(d.views) > 0
	d.mu.Unlock()

	// Isolation, only when someone needs it: with no reader open there is
	// nobody to keep the old values for.
	if readers {
		pre := d.preImages(staged)
		d.mu.Lock()
		if d.undo == nil {
			d.undo = map[uint64]map[[32]byte][]byte{}
		}
		d.undo[version] = pre
		d.undoVersions = append(d.undoVersions, version)
		d.mu.Unlock()
	}

	// Durability: the store has it, sealed, before this returns.
	err := d.writeThrough(staged)

	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil {
		// The store may hold part of the batch; the version does not move
		// and the overlay stays for its readers. The caller stops the node.
		return err
	}
	d.version = version
	d.pruneUndo()
	d.reportStats()
	d.observeStaging()
	d.warnOldView()
	return nil
}

// preImages reads what the store holds for every key a batch rewrites,
// before the batch is written. A key classified write-once has no
// pre-image by definition -- it is being written for the first time -- so
// only the dynamic layer is consulted, which is where every rewritable key
// lives. Runs under writeMu with mu released; the store is at the previous
// version because commits are serialized.
func (d *Database) preImages(s *staged) map[[32]byte][]byte {
	pre := make(map[[32]byte][]byte, len(s.entries))
	for h, e := range s.entries {
		if e.perm {
			pre[h] = []byte{}
			continue
		}
		v, err := d.kv.GetDyna(h)
		if err != nil || len(v) == 0 {
			pre[h] = []byte{}
			continue
		}
		pre[h] = v
	}
	return pre
}

// preImageAt returns what key h held at version at, if a commit after at
// rewrote it: the pre-image recorded by the EARLIEST such commit. The
// caller must hold the lock (shared is enough).
func (d *Database) preImageAt(at uint64, h [32]byte) ([]byte, bool) {
	for _, v := range d.undoVersions {
		if v <= at {
			continue
		}
		if pre, ok := d.undo[v][h]; ok {
			return pre, true
		}
	}
	return nil, false
}

// pruneUndo drops the overlays no open reader can need: a reader at
// version o needs the pre-images of every commit after o, so an overlay
// for version v is needed only while a reader at some version below v is
// open. The caller must hold the lock.
func (d *Database) pruneUndo() {
	oldest, any := d.oldestView()
	kept := d.undoVersions[:0]
	for _, v := range d.undoVersions {
		if any && v > oldest {
			kept = append(kept, v)
			continue
		}
		delete(d.undo, v)
	}
	d.undoVersions = kept
}

// warnOldView names the reader that has held the oldest open version, once
// it is older than ViewWarnAfter and at most once every 30 s. This is how
// a soak says WHO is holding commits in memory rather than that someone is.
// The caller must hold the lock.
func (d *Database) warnOldView() {
	if d.ViewWarnAfter <= 0 {
		return
	}
	v, ok := d.oldestView()
	if !ok {
		return
	}
	opened, ok := d.viewOpened[v]
	if !ok || time.Since(opened) < d.ViewWarnAfter || time.Since(d.lastViewWarn) < 30*time.Second {
		return
	}
	d.lastViewWarn = time.Now()
	slog.Warn("A reader is holding an old database version",
		"module", "bcdb", "database", d.metricLabel, "version", v, "current", d.version,
		"age", time.Since(opened).Round(time.Second), "opener", d.viewOpener[v],
		"overlays", len(d.undoVersions))
}

// viewOpener names the function that begun a view: the first caller above
// the store and record-model wrappers.
func viewOpener() string {
	pcs := make([]uintptr, 16)
	n := runtime.Callers(3, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		fn := f.Function
		if fn != "" && !isViewWrapper(fn) {
			return fn
		}
		if !more {
			return "unknown"
		}
	}
}

var (
	stagedCommitsGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "accumulate", Subsystem: "bcdb", Name: "staged_commits",
		Help: "Commit overlays held in memory for readers begun before them; every commit is on disk (D5)",
	}, []string{"database"})
	oldestViewAgeGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "accumulate", Subsystem: "bcdb", Name: "oldest_view_age_seconds",
		Help: "Age of the oldest open read view; zero when none is open",
	}, []string{"database"})
)

// observeStaging publishes how many commit overlays open readers are holding
// and the oldest view's age. The caller must hold the lock.
func (d *Database) observeStaging() {
	stagedCommitsGauge.WithLabelValues(d.metricLabel).Set(float64(len(d.undoVersions)))
	age := 0.0
	if v, ok := d.oldestView(); ok {
		if t, ok := d.viewOpened[v]; ok {
			age = time.Since(t).Seconds()
		}
	}
	oldestViewAgeGauge.WithLabelValues(d.metricLabel).Set(age)
}

// metricLabelFor names a database for the staging gauges by the directory
// two above the store: /.../bvnn/data/accumulate.db -> "bvnn".
func metricLabelFor(path string) string {
	up := filepath.Dir(filepath.Dir(filepath.Clean(path)))
	if b := filepath.Base(up); b != "." && b != string(filepath.Separator) && b != "" {
		return b
	}
	return path
}

// tally counts a write against its key's shape.  It returns the shape,
// which the write path carries so that a refusal by the permanent layer
// can be attributed to it.  The caller must hold the lock.
//
// A counter, and nothing more.  Whether a write CHANGED the record is
// the store's to answer -- the permanent layer refuses a rewrite, and
// that refusal is the exact signal, free and durable across restarts.
// Answering it here instead meant remembering a digest of the last value
// written for every key: 192 MB on a 500 tx/s soak, 38% of the live heap
// and the largest single consumer on the node, spent on a diagnostic
// (#4165).
func (d *Database) tally(key *record.Key, perm bool) string {
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
	c.Writes++
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
		err := d.kv.PutDyna(key, e.value)
		return err
	}

	err := d.kv.PutPerm(key, e.value)
	switch {
	case err == nil:
		return nil
	case !isRefusal(err):
		return err
	}

	// The store says this record is not write-once.  Counted against
	// the shape, and the FIRST one is logged: a shape classified
	// write-once that the store refuses is a defect in isWriteOnce, and
	// it should not have to be noticed by reading a stats file.
	d.mu.Lock()
	d.except(key)
	first := false
	if c := d.shapes[e.shape]; c != nil {
		c.Misrouted++
		first = c.Misrouted == 1
	}
	d.mu.Unlock()
	if first {
		slog.Warn("Permanent layer refused a write: this shape is not write-once",
			"module", "bcdb", "shape", e.shape)
	}
	err = d.kv.PutDyna(key, e.value)
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
	perm, dyna := d.kv.Stats()
	report := struct {
		Commits      uint64                 `json:"commits"`
		Perm         bcdb.StoreStats        `json:"perm"`
		Dyna         bcdb.StoreStats        `json:"dyna"`
		DuplicatePct float64                `json:"permDuplicatePct"`
		ConflictPct  float64                `json:"permConflictPct"`
		WalkPct      float64                `json:"permWalkPct"`
		Misrouted    []string               `json:"misroutedShapes"`
		Shapes       map[string]*ShapeCount `json:"shapes"`

		// Staged is how many commits' pre-images are held for readers
		// that predate them. Every commit is on disk; growing without
		// bound means a reader was never closed (D5).
		Staged int `json:"stagedCommits"`

		// TallySample is the rate New/Duplicate/Rewritten were sampled
		// at: multiply by it to estimate, or read them as ratios.
		TallySample uint8 `json:"tallySample"`
		TallyKeys   int   `json:"tallyKeys"`
		TallyCapped bool  `json:"tallyCapped"`

		// Maintenance runs in the background; a failure is recorded here
		// rather than failing a commit.
		MaintenanceErrors uint64 `json:"maintenanceErrors"`
		MaintenanceLast   string `json:"maintenanceLastError,omitempty"`

		// DeepFallbacks is what a SHALLOW batch -- the executor's --
		// could only answer from history, by record shape.  The store
		// answers a permanent read from its window and calls anything
		// older absent; a reader that means to look back takes a deep
		// batch (BeginDeep).  Empty means every deep reader has one and
		// the fallback in getAt can go, leaving the window enforced.
		// Anything here NAMES the call sites that still need one.
		DeepFallbacks map[string]uint64 `json:"deepFallbacks,omitempty"`
	}{Commits: d.version, Perm: perm, Dyna: dyna, Shapes: d.shapes,
		DeepFallbacks: d.fallbackSnapshot(),
		Staged:        len(d.undoVersions), TallySample: d.TallySample,
		TallyKeys: len(d.last), TallyCapped: len(d.last) >= d.TallyKeys,
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

// forEachAt iterates the store as it was at version at: keys rewritten by
// a later commit yield the pre-image the earliest such commit recorded,
// keys created later are skipped, and keys deleted later are yielded from
// their pre-image.
func (d *Database) forEachAt(at uint64, fn func(*record.Key, []byte) error) error {
	// Pre-images a reader at this version must see, earliest commit first
	// so the first recorded value for a key wins.
	d.mu.RLock()
	pre := map[[32]byte][]byte{}
	for _, v := range d.undoVersions {
		if v <= at {
			continue
		}
		for h, p := range d.undo[v] {
			if _, seen := pre[h]; !seen {
				pre[h] = p
			}
		}
	}
	d.mu.RUnlock()

	yielded := map[[32]byte]bool{}
	err := d.kv.ForEach(func(key [32]byte, value []byte) error {
		if p, ok := pre[key]; ok {
			yielded[key] = true
			if len(p) == 0 {
				return nil // Did not exist at this version
			}
			return fn(record.KeyFromHash(key), p)
		}
		if len(value) == 0 {
			return nil
		}
		return fn(record.KeyFromHash(key), value)
	})
	if err != nil {
		return err
	}
	// Keys that existed at this version and have since been deleted are
	// not in the store's iteration; they are here.
	for h, p := range pre {
		if yielded[h] || len(p) == 0 {
			continue
		}
		if err := fn(record.KeyFromHash(h), p); err != nil {
			return err
		}
	}
	return nil
}

// isViewWrapper reports whether fn is one of the layers a view passes
// through on its way from the caller that wanted it: this store, the
// keyvalue adapters, and the record model's Begin/View/Update.
func isViewWrapper(fn string) bool {
	for _, w := range []string{
		"/keyvalue/bcdb.(*Database).",
		"/keyvalue.deepBeginner",
		"/keyvalue.Deep",
		"/keyvalue/memory.",
		"/internal/database.(*Database).Begin",
		"/internal/database.(*Database).View",
		"/internal/database.(*Database).Update",
	} {
		if strings.Contains(fn, w) {
			return true
		}
	}
	return false
}
