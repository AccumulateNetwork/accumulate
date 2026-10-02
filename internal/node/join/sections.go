// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	keys "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// A joining node takes the state tree by its top and loads its sections as
// they are needed (executor spec, "Sync", §3a; #4485). A section the node has
// not loaded is one marker leaf in its tree, whose value is the section's
// hash; the markers are the flags, persisted with the tree. sections is the
// set in memory, and the database's gate: a block executing reads an account
// in a section not loaded, and the section is loaded into the block's batch,
// checked against the marker, before the read returns.
type sections struct {
	partition *url.URL
	label     string
	db        *database.Database // ungated
	log       *slog.Logger

	// peers is the partition's peers, in the order they are asked.
	peers func(ctx context.Context) []pull.Source

	// loaded is told the accounts a committed block's sections brought in:
	// they are taken by their chain heads, and backfilled (§2).
	loaded func([]*url.URL)

	// timeout is how long a load inside a block asks the peers before it
	// holds the block.
	timeout time.Duration

	notLoaded  [bpt.Sections]atomic.Bool
	nNotLoaded atomic.Int64 // how many of notLoaded are set
	turn       atomic.Uint64

	mu   sync.Mutex
	hash map[bpt.Section][32]byte

	// opening is the sections the join has opened and not loaded yet: no
	// marker holds them and the store does not hold them whole, so they are
	// not loaded either (opened, pulled).
	opening map[bpt.Section]bool
	pending map[bpt.Section][]*url.URL
	ready   map[bpt.Section]*loadedSection
	said    time.Time
	done    bool
}

// loadTimeout is how long a section's load inside a block asks every peer
// before it holds the block (executor spec, "Sync", §3a).
const loadTimeout = 30 * time.Second

// fillReady is how many loaded sections the fill holds for the next block
// before it waits for one to be written.
const fillReady = 64

// mSectionsNotLoaded is how many sections of the state tree this node has not
// loaded: zero once the fill is done (#4485).
var mSectionsNotLoaded = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "accumulate",
	Subsystem: "join",
	Name:      "sections_not_loaded",
	Help:      "Sections of the state tree this node holds by their hash alone, not yet loaded",
}, []string{"partition"})

func newSections(partition *url.URL, label string, db *database.Database, log *slog.Logger) *sections {
	return &sections{
		partition: partition,
		label:     label,
		db:        db,
		log:       log,
		timeout:   loadTimeout,
		hash:      map[bpt.Section][32]byte{},
		pending:   map[bpt.Section][]*url.URL{},
		ready:     map[bpt.Section]*loadedSection{},
	}
}

// read sets the set from the markers in the tree: 4,096 point reads.
func (x *sections) read() error {
	if x == nil {
		return nil
	}
	batch := x.db.Begin(false)
	defer batch.Discard()
	hash := map[bpt.Section][32]byte{}
	for i := 0; i < bpt.Sections; i++ {
		s := bpt.Section(i)
		v, err := batch.BPT().Get(s.MarkerKey())
		switch {
		case err == nil:
			if len(v) != 32 {
				return errors.InternalError.WithFormat("the marker of section %d holds %d bytes", s, len(v))
			}
			hash[s] = [32]byte(v)
		case errors.Is(err, errors.NotFound):
		default:
			return errors.UnknownError.WithFormat("read the marker of section %d: %w", s, err)
		}
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	x.hash = hash
	x.opening = map[bpt.Section]bool{}
	for i := range x.notLoaded {
		_, ok := hash[bpt.Section(i)]
		x.setNotLoaded(bpt.Section(i), ok)
	}
	x.done = len(hash) == 0
	mSectionsNotLoaded.WithLabelValues(x.label).Set(float64(len(hash)))
	return nil
}

// NotLoaded implements [database.SectionGate].
func (x *sections) NotLoaded(s bpt.Section) bool {
	return x != nil && x.notLoaded[s].Load()
}

// AnyNotLoaded implements [database.SectionGate].
func (x *sections) AnyNotLoaded() bool {
	return x != nil && x.nNotLoaded.Load() > 0
}

// setNotLoaded sets whether a section is not loaded, and the count with it.
func (x *sections) setNotLoaded(s bpt.Section, v bool) {
	if x.notLoaded[s].Swap(v) == v {
		return
	}
	if v {
		x.nNotLoaded.Add(1)
	} else {
		x.nNotLoaded.Add(-1)
	}
}

// count is how many sections are not loaded.
func (x *sections) count() int {
	if x == nil {
		return 0
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	return len(x.hash)
}

// expected is the hash a section not loaded holds, and whether it is not
// loaded.
func (x *sections) expected(s bpt.Section) ([32]byte, bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	h, ok := x.hash[s]
	return h, ok
}

// opened says the pull wrote into a section not loaded: its marker is gone,
// and the pull loads it.
func (x *sections) opened(s bpt.Section) {
	if x == nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	delete(x.hash, s)
	delete(x.ready, s)
	if x.opening == nil {
		x.opening = map[bpt.Section]bool{}
	}
	x.opening[s] = true
	mSectionsNotLoaded.WithLabelValues(x.label).Set(float64(len(x.hash)))
}

// pulled says the join has loaded a section it opened.
func (x *sections) pulled(s bpt.Section) {
	if x == nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	delete(x.opening, s)
	_, marked := x.hash[s]
	x.setNotLoaded(s, marked)
}

// forgetOpening forgets what an abandoned pull opened.
func (x *sections) forgetOpening() {
	if x == nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	for s := range x.opening {
		_, marked := x.hash[s]
		x.setNotLoaded(s, marked)
	}
	x.opening = map[bpt.Section]bool{}
}

// committed block is the block the node last committed: what a load inside
// the next block is as of.
func (x *sections) committedBlock() (uint64, error) {
	return readExecutedBlock(x.db, x.partition)
}

// A loadedSection is a section's leaves as of a block, and the accounts
// behind them, checked against the section's hash.
type loadedSection struct {
	section bpt.Section
	block   uint64
	hash    [32]byte
	leaves  []bpt.SectionLeaf
	states  []*api.AccountState
}

// Load implements [database.SectionGate]: the section is loaded as of the
// block the node last committed, checked against its marker, and written
// into the block's batch. It asks every peer until one answers or the time is
// up, and then holds the block.
func (x *sections) Load(batch *database.Batch, s bpt.Section, executing bool) error {
	if !executing {
		return x.loadForCheck(batch, s)
	}
	want, ok := x.expected(s)
	if !ok {
		x.mu.Lock()
		opening := x.opening[s]
		x.mu.Unlock()
		if opening {
			return errors.NotReady.WithFormat("section %d is being loaded by the join", s)
		}
		return nil
	}
	if err := checkMarker(batch, s, want); err != nil {
		return err
	}
	h, err := x.committedBlock()
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}
	// Beside the join, not on its goroutine: a block is executing.
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), backfillKey{}, true), x.timeout)
	defer cancel()
	var last error
	for ctx.Err() == nil {
		var ld *loadedSection
		ld, last = x.fetch(ctx, s, h, want)
		if last == nil {
			return x.apply(batch, ld)
		}
		if !pause(ctx, 200*time.Millisecond) {
			break
		}
	}
	x.log.Warn("A section of the state tree a block reads could not be loaded; the block is held",
		"partition", x.partition, "section", s, "block", h+1, "error", last)
	return errors.NotReady.WithFormat("section %d as of block %d: %w", s, h, last)
}

// checkTimeout is how long a validation's load asks the peers.
const checkTimeout = 5 * time.Second

// loadForCheck loads a section into a validation's batch, from a peer's
// current state, which the validators judge the transaction against too. It
// is not checked against the marker -- the section may have changed since
// the block this node last committed -- and it is discarded with the batch:
// it decides no block (executor spec, "Sync", §3a).
func (x *sections) loadForCheck(batch *database.Batch, s bpt.Section) error {
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), backfillKey{}, true), checkTimeout)
	defer cancel()
	var last error = errors.NotFound.With("no peer to load from")
	for _, src := range x.peers(ctx) {
		ld, err := x.fetchAt(ctx, src, s, 0, nil)
		if err == nil {
			return x.write(batch, ld)
		}
		last = err
	}
	return errors.NotReady.WithFormat("section %d for a validation: %w", s, last)
}

// checkMarker is whether the batch's tree still holds the section's marker
// with the hash the set says: what the section is checked against.
func checkMarker(batch *database.Batch, s bpt.Section, want [32]byte) error {
	v, err := batch.BPT().Get(s.MarkerKey())
	if err != nil {
		return errors.UnknownError.WithFormat("the marker of section %d: %w", s, err)
	}
	if [32]byte(v) != want {
		return errors.Conflict.WithFormat("the marker of section %d holds %x, not %x", s, v[:8], want[:8])
	}
	return nil
}

// apply writes a loaded section into the batch a block executes in: the
// accounts beneath its records, and the marker swapped for the leaves in its
// tree. The root does not move: the leaves hash to the marker's value.
func (x *sections) apply(batch *database.Batch, ld *loadedSection) error {
	if err := x.write(batch, ld); err != nil {
		return err
	}
	urls := make([]*url.URL, len(ld.states))
	for i, st := range ld.states {
		urls[i] = st.Url
	}
	x.mu.Lock()
	x.pending[ld.section] = urls
	x.mu.Unlock()
	return nil
}

// write writes a loaded section into a batch: the accounts beneath its
// records, and the marker swapped for the leaves in its tree.
func (x *sections) write(batch *database.Batch, ld *loadedSection) error {
	err := batch.WriteBeneath(func(c *database.Batch) error {
		return pull.WriteStates(c, ld.states)
	})
	if err != nil {
		return errors.UnknownError.WithFormat("write section %d: %w", ld.section, err)
	}
	tree := batch.BPT()
	if err := tree.Delete(ld.section.MarkerKey()); err != nil {
		return errors.UnknownError.Wrap(err)
	}
	for _, st := range ld.states {
		h := st.LeafHash
		if err := tree.Insert(keys.NewKey("Account", st.Url), h[:]); err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	return nil
}

// Committed implements [database.SectionGate].
func (x *sections) Committed(list []bpt.Section) {
	var urls []*url.URL
	x.mu.Lock()
	for _, s := range list {
		delete(x.hash, s)
		delete(x.ready, s)
		x.setNotLoaded(s, false)
		urls = append(urls, x.pending[s]...)
		delete(x.pending, s)
	}
	left := len(x.hash)
	mSectionsNotLoaded.WithLabelValues(x.label).Set(float64(left))
	say := left > 0 && time.Since(x.said) >= time.Minute
	if say {
		x.said = time.Now()
	}
	done := left == 0 && !x.done
	if done {
		x.done = true
	}
	x.mu.Unlock()
	if say {
		x.log.Info("Loading the sections not loaded", "partition", x.partition, "left", left)
	}
	if done {
		x.log.Info("Every section is loaded", "partition", x.partition)
	}
	if x.loaded != nil && len(urls) > 0 {
		x.loaded(urls)
	}
}

// Opened implements [database.SectionGate]: the sections the fill loaded are
// written into the batch a block executes in, before it reads anything. A
// section whose marker is gone was loaded by a block that read it.
func (x *sections) Opened(batch *database.Batch) {
	x.mu.Lock()
	var list []*loadedSection
	for s, ld := range x.ready {
		if x.notLoaded[s].Load() {
			list = append(list, ld)
		}
		delete(x.ready, s)
	}
	x.mu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].section < list[j].section })
	for _, ld := range list {
		if checkMarker(batch, ld.section, ld.hash) != nil {
			continue
		}
		if err := x.apply(batch, ld); err != nil {
			x.log.Info("A loaded section could not be written; it is loaded again", "partition", x.partition, "section", ld.section, "error", err)
			continue
		}
		batch.MarkSectionLoaded(ld.section)
	}
}

// dropReady forgets what the fill loaded and no block wrote: the node is
// pulling again, and it was loaded as of a state that may not stand.
func (x *sections) dropReady() {
	if x == nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	x.ready = map[bpt.Section]*loadedSection{}
	x.pending = map[bpt.Section][]*url.URL{}
}

// fetch loads a section as of block h from the first peer that answers it,
// checked against want: the section's leaves as of h must hash to want by the
// tree's rule, and each account's state to its leaf.
func (x *sections) fetch(ctx context.Context, s bpt.Section, h uint64, want [32]byte) (*loadedSection, error) {
	var last error = errors.NotFound.With("no peer to load from")
	srcs := x.peers(ctx)
	// Each load starts at the next peer, so no one peer answers every load.
	if n := len(srcs); n > 1 {
		turn := int(x.turn.Add(1) % uint64(n))
		srcs = append(append([]pull.Source(nil), srcs[turn:]...), srcs[:turn]...)
	}
	for _, src := range srcs {
		ld, err := x.fetchFrom(ctx, src, s, h, want)
		if err == nil {
			return ld, nil
		}
		last = errors.UnknownError.WithFormat("%s: %w", sourceKey(src), err)
	}
	return nil, last
}

func (x *sections) fetchFrom(ctx context.Context, src pull.Source, s bpt.Section, h uint64, want [32]byte) (*loadedSection, error) {
	ld, err := x.fetchAt(ctx, src, s, h, &want)
	if err == nil {
		return ld, nil
	}
	// A peer that retains no state as of h, or has moved on past it, may
	// still hold the section as it was at h: a section no block after h
	// changed. Its current state is checked against the same hash, so it is
	// taken only if it is h's.
	cur, err2 := x.fetchAt(ctx, src, s, 0, &want)
	if err2 == nil {
		cur.block = h
		return cur, nil
	}
	return nil, err
}

// fetchAt loads a section from one peer as of block h, zero being its current
// state, checked against want unless it is nil.
func (x *sections) fetchAt(ctx context.Context, src pull.Source, s bpt.Section, h uint64, want *[32]byte) (*loadedSection, error) {
	leaves, urls, err := sectionLeaves(ctx, src, x.partition, s, h)
	if err != nil {
		return nil, err
	}
	got, ok := bpt.SectionHash(leaves)
	if want != nil && (!ok || got != *want) {
		return nil, errors.Conflict.WithFormat("the leaves of section %d as of %d hash to %x, not to its marker %x", s, h, got[:8], want[:8])
	}
	states, err := statesAt(ctx, src, x.partition, urls, h)
	if err != nil {
		return nil, err
	}
	for i, st := range states {
		if err := pull.CheckState(st); err != nil {
			return nil, err
		}
		if st.LeafHash != leaves[i].Hash {
			return nil, errors.Conflict.WithFormat("%v: the state hashes to leaf %x, the section's leaf is %x", st.Url, st.LeafHash[:8], leaves[i].Hash[:8])
		}
	}
	return &loadedSection{section: s, block: h, hash: got, leaves: leaves, states: states}, nil
}

// sectionPage is how many leaves one page of a section asks for.
const sectionPage = 256

// sectionLeaves pages the leaves of one section of the peer's tree as of
// block h (zero is the current tree), with the account each names, in the
// page's order, and the block the pages were read at.
func sectionLeaves(ctx context.Context, src pull.Source, partition *url.URL, s bpt.Section, h uint64) ([]bpt.SectionLeaf, []*url.URL, error) {
	leaves, urls, _, err := sectionLeavesAt(ctx, src, partition, s, h)
	return leaves, urls, err
}

func sectionLeavesAt(ctx context.Context, src pull.Source, partition *url.URL, s bpt.Section, h uint64) ([]bpt.SectionLeaf, []*url.URL, uint64, error) {
	var leaves []bpt.SectionLeaf
	var urls []*url.URL
	start := s.ScanStart()
	var block uint64
	for {
		page, err := src.QueryBptPage(ctx, partition, &api.BptPageQuery{StartHash: start, Count: sectionPage, ForHeight: h})
		if err != nil {
			return nil, nil, 0, errors.UnknownError.WithFormat("page section %d: %w", s, err)
		}
		if block != 0 && page.Block != block {
			return nil, nil, 0, errors.Conflict.WithFormat("the pages of section %d are of blocks %d and %d", s, block, page.Block)
		}
		block = page.Block
		if h != 0 && page.Block > h {
			return nil, nil, 0, errors.Conflict.WithFormat("the peer answered section %d as of block %d, not %d", s, page.Block, h)
		}
		past := false
		for _, e := range page.Entries {
			if e == nil {
				continue
			}
			es := bpt.SectionOf(e.KeyHash)
			switch {
			case es > s:
				// A leaf above the section's depth that the scan met on
				// its way down.
				continue
			case es < s:
				past = true
			}
			if past {
				break
			}
			if e.Account == nil || [32]byte(keys.NewKey("Account", e.Account).Hash()) != e.KeyHash {
				return nil, nil, 0, errors.Conflict.WithFormat("a leaf of section %d names %v, which is not its key %x", s, e.Account, e.KeyHash[:8])
			}
			leaves = append(leaves, bpt.SectionLeaf{KeyHash: e.KeyHash, Hash: e.ValueHash})
			urls = append(urls, e.Account)
		}
		if past || page.Done || len(page.Entries) == 0 {
			return leaves, urls, block, nil
		}
		start = page.NextStart
	}
}

// statesAt asks one peer for the accounts' sync states as of block h, every
// page of the answer, in the order asked. An account not served whole is the
// peer's failure.
func statesAt(ctx context.Context, src pull.Source, partition *url.URL, urls []*url.URL, h uint64) ([]*api.AccountState, error) {
	out := make([]*api.AccountState, len(urls))
	at := make(map[[32]byte]int, len(urls))
	for i, u := range urls {
		at[accountKey(u)] = i
	}
	var cursor []byte
	var part []byte
	var partUrl *url.URL
	for {
		rec, err := src.QueryAccountStates(ctx, partition, &api.AccountStatesQuery{Accounts: urls, Cursor: cursor, ForHeight: h})
		if err != nil {
			return nil, errors.UnknownError.WithFormat("the states of %d accounts as of %d: %w", len(urls), h, err)
		}
		if h != 0 && rec.Block > h {
			return nil, errors.Conflict.WithFormat("the peer answered as of block %d, not %d", rec.Block, h)
		}
		states := rec.States
		if p := rec.Part; p != nil {
			if p.Offset == 0 {
				part, partUrl = nil, p.Url
			}
			if partUrl == nil || !partUrl.Equal(p.Url) || uint64(len(part)) != p.Offset {
				return nil, errors.Conflict.WithFormat("a part of %v does not continue what was served", p.Url)
			}
			part = append(part, p.Data...)
			if uint64(len(part)) == p.Total {
				st := new(api.AccountState)
				if err := st.UnmarshalBinary(part); err != nil {
					return nil, errors.Conflict.WithFormat("%v served in parts: %w", partUrl, err)
				}
				states = append(states, st)
				part, partUrl = nil, nil
			}
		}
		for _, st := range states {
			if st == nil || st.Url == nil {
				continue
			}
			i, ok := at[accountKey(st.Url)]
			if !ok {
				return nil, errors.Conflict.WithFormat("the peer served %v, which was not asked for", st.Url)
			}
			out[i] = st
		}
		if len(rec.Cursor) == 0 {
			break
		}
		cursor = rec.Cursor
	}
	for i, st := range out {
		if st == nil {
			return nil, errors.NotFound.WithFormat("the peer did not serve %v", urls[i])
		}
	}
	return out, nil
}

// fill loads every section not loaded, in key order, as of the block the
// node last committed, and hands each to the next block's batch to write
// (Opened). It never commits: a commit beside a block's would write tree
// nodes the block's batch also holds. It runs while the node is ACTIVE, on a
// goroutine of its own, and ends when every section is loaded or ctx ends
// (executor spec, "Sync", §3a, "The fill").
func (x *sections) fill(ctx context.Context, active func() bool) {
	ctx = context.WithValue(ctx, backfillKey{}, true)
	if x == nil {
		return
	}
	for ctx.Err() == nil {
		if !active() {
			if !pause(ctx, backfillPause) {
				return
			}
			continue
		}
		x.mu.Lock()
		var todo []bpt.Section
		for s := range x.hash {
			if _, ok := x.ready[s]; !ok {
				todo = append(todo, s)
			}
		}
		waiting := len(x.ready)
		left := len(x.hash)
		x.mu.Unlock()
		if left == 0 {
			return
		}
		if len(todo) == 0 || waiting >= fillReady {
			// Everything not loaded is loaded and waiting for a block.
			if !pause(ctx, 50*time.Millisecond) {
				return
			}
			continue
		}
		sort.Slice(todo, func(i, j int) bool { return todo[i] < todo[j] })
		h, err := x.committedBlock()
		if err != nil {
			if !pause(ctx, backfillPause) {
				return
			}
			continue
		}
		failed := 0
		for _, s := range todo {
			if ctx.Err() != nil || !active() {
				break
			}
			x.mu.Lock()
			full := len(x.ready) >= fillReady
			x.mu.Unlock()
			if full {
				break
			}
			want, ok := x.expected(s)
			if !ok {
				continue
			}
			ld, err := x.fetch(ctx, s, h, want)
			if err != nil {
				failed++
				x.log.Debug("A section could not be loaded by the fill; it is asked again", "partition", x.partition, "section", s, "block", h, "error", err)
				continue
			}
			x.mu.Lock()
			if x.notLoaded[s].Load() {
				x.ready[s] = ld
			}
			x.mu.Unlock()
		}
		if failed > 0 && !pause(ctx, backfillPause) {
			return
		}
	}
}
