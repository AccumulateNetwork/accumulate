// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package database

import (
	"sync"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// A node that joined holds the state tree's interior and, for each section it
// has not loaded, one marker leaf (executor.md, "Sync", §3a; database.md, "A
// section not loaded is refused or loaded, never not found"; #4485). The
// accounts behind such a section are not in the store, and a read that
// reported them not found would say an account does not exist that does. So
// every outermost batch checks each read of an account's records against the
// sections not loaded: a batch a block executes in loads the section; every
// other batch is refused.

// A SectionGate knows which sections the node has not loaded, and loads one.
type SectionGate interface {
	// AnyNotLoaded reports whether any section is not loaded. It is called
	// on every read of an account's records, and must be an atomic load:
	// when it is false the read is not checked further, so a node that has
	// loaded every section pays nothing per read (#4485 review, finding 6).
	AnyNotLoaded() bool

	// NotLoaded reports whether the section is not loaded. It is called on
	// every read of an account's records while any section is not loaded,
	// and must be cheap.
	NotLoaded(bpt.Section) bool

	// Load loads the section into the batch a block executes in: the
	// accounts beneath the batch's records (WriteBeneath) and the marker
	// swapped for the leaves in the batch's tree. It may take as long as
	// asking the peers takes. An error holds the block. executing is false
	// for a validation's batch, which is discarded.
	Load(batch *Batch, section bpt.Section, executing bool) error

	// Committed says a batch that loaded the sections committed: they are
	// loaded.
	Committed([]bpt.Section)

	// Opened is told of each batch a block executes in, before anything
	// reads it: what was loaded beside the block is written into it here
	// (WriteBeneath, MarkSectionLoaded).
	Opened(batch *Batch)
}

// sectionGates is the gate a database and its views share. It is a pointer
// so that Deep and Unisolated views, taken before the gate is set, see it.
type sectionGates struct {
	gate atomic.Pointer[gateBox]
}

type gateBox struct{ SectionGate }

// SetSectionGate installs the gate every batch of this database and its
// views is checked against. Nil removes it.
func (d *Database) SetSectionGate(g SectionGate) {
	if g == nil {
		d.gates.gate.Store(nil)
		return
	}
	d.gates.gate.Store(&gateBox{g})
}

// Ungated returns a view of this database whose batches are not checked
// against the sections not loaded: the join's, which are what write them.
func (d *Database) Ungated() *Database {
	v := d.view(d.store)
	v.gates = new(sectionGates)
	return v
}

// gateMode is what an outermost batch does with a read of an account in a
// section not loaded.
type gateMode int

const (
	gateRefuses gateMode = iota
	gateLoads
	gateExecutes
)

// BeginExecuting begins the batch a block executes in. A read of an account
// in a section not loaded loads the section into it, and what was loaded
// beside the blocks is written into it first (SectionGate.Opened).
func (d *Database) BeginExecuting(writable bool) *Batch {
	return d.begin(writable, gateExecutes)
}

// BeginLoading begins a batch that loads a section not loaded when it reads
// an account in it, as a block's does, and is discarded: the batch a
// validation reads, which judges a transaction against the state the next
// block executes on.
func (d *Database) BeginLoading(writable bool) *Batch {
	return d.begin(writable, gateLoads)
}

func (d *Database) gate() SectionGate {
	if d.gates == nil {
		return nil
	}
	b := d.gates.gate.Load()
	if b == nil {
		return nil
	}
	return b.SectionGate
}

// SectionOfAccount is the section the account's leaf falls in.
func SectionOfAccount(u *url.URL) bpt.Section {
	return bpt.SectionOf(record.NewKey("Account", u.StripExtras()).Hash())
}

// gatedChangeSet is the outermost change set of a batch begun while a gate
// is set. Every read of an account's records is checked against the gate.
type gatedChangeSet struct {
	keyvalue.ChangeSet
	gate SectionGate

	// batch is the batch this change set is the store of, when it loads;
	// nil refuses.
	batch     *Batch
	executing bool

	// writable is whether the change set can take what a load writes; a
	// read-only one (a validation's) holds it in overlay instead, and it is
	// discarded with the batch.
	writable   bool
	ovMu       sync.Mutex
	overlay    map[[32]byte]memory.Entry
	hasOverlay atomic.Bool

	// loading serializes loads; mu guards loaded and failed, and is not
	// held while a load reads through this change set.
	loading sync.Mutex

	mu     sync.Mutex
	loaded map[bpt.Section]bool
	failed error
}

func (c *gatedChangeSet) Get(key *record.Key) ([]byte, error) {
	if c.gate.AnyNotLoaded() && key.Len() >= 2 && key.Get(0) == "Account" {
		if u, ok := key.Get(1).(*url.URL); ok {
			s := bpt.SectionOf(key.SliceJ(2).Hash())
			if c.gate.NotLoaded(s) {
				err := c.ensure(s, u)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	return c.raw(key)
}

// raw reads past the gate: what a load wrote into a read-only change set,
// then the change set.
func (c *gatedChangeSet) raw(key *record.Key) ([]byte, error) {
	if !c.hasOverlay.Load() {
		return c.ChangeSet.Get(key)
	}
	c.ovMu.Lock()
	e, ok := c.overlay[key.Hash()]
	c.ovMu.Unlock()
	if ok {
		if e.Delete {
			return nil, (*database.NotFoundError)(key)
		}
		return e.Value, nil
	}
	return c.ChangeSet.Get(key)
}

func (c *gatedChangeSet) putOverlay(entries map[[32]byte]memory.Entry) error {
	c.ovMu.Lock()
	defer c.ovMu.Unlock()
	if c.overlay == nil {
		c.overlay = map[[32]byte]memory.Entry{}
	}
	for k, e := range entries {
		c.overlay[k] = e
	}
	c.hasOverlay.Store(len(c.overlay) > 0)
	return nil
}

func (c *gatedChangeSet) ensure(s bpt.Section, u *url.URL) error {
	if c.batch == nil {
		return errors.NotReady.WithFormat("%v is in section %d of the state tree, which this node has not loaded yet", u, s)
	}
	c.loading.Lock()
	defer c.loading.Unlock()
	c.mu.Lock()
	done, failed := c.loaded[s], c.failed
	c.mu.Unlock()
	switch {
	case done:
		return nil
	case failed != nil:
		return failed
	}
	err := c.gate.Load(c.batch, s, c.executing)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.failed = errors.NotReady.WithFormat("section %d of the state tree, which %v is in, could not be loaded: %w", s, u, err)
		return c.failed
	}
	if c.loaded == nil {
		c.loaded = map[bpt.Section]bool{}
	}
	c.loaded[s] = true
	return nil
}

// Commit refuses a batch a section could not be loaded into: a block that
// read an account it could not load executed against a state it does not
// hold, and is held, not produced.
func (c *gatedChangeSet) Commit() error {
	c.mu.Lock()
	failed := c.failed
	var loaded []bpt.Section
	for s := range c.loaded {
		loaded = append(loaded, s)
	}
	c.mu.Unlock()
	if failed != nil {
		return failed
	}
	err := c.ChangeSet.Commit()
	if err != nil {
		return err
	}
	if len(loaded) > 0 {
		c.gate.Committed(loaded)
	}
	return nil
}

// Loading reports whether the batch is the batch a block executes in, and has
// loaded the section in it.
func (b *Batch) loadedSection(s bpt.Section) bool {
	for b.parent != nil {
		b = b.parent
	}
	c, ok := b.store.(interface{ Unwrap() keyvalue.Store })
	if !ok {
		return false
	}
	g, ok := c.Unwrap().(*gatedChangeSet)
	if !ok {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.loaded[s]
}

// LoadedSection reports whether the section was loaded into this batch's
// outermost batch, by execution or by WriteBeneath's caller.
func (b *Batch) LoadedSection(s bpt.Section) bool { return b.loadedSection(s) }

// MarkSectionLoaded records that the section was written into this
// outermost batch (the fill's sections, written at a block's start), so the
// gate forgets it when the batch commits and does not load it again.
func (b *Batch) MarkSectionLoaded(s bpt.Section) {
	if b.parent != nil {
		return
	}
	c, ok := b.store.(interface{ Unwrap() keyvalue.Store })
	if !ok {
		return
	}
	g, ok := c.Unwrap().(*gatedChangeSet)
	if !ok {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.loaded == nil {
		g.loaded = map[bpt.Section]bool{}
	}
	g.loaded[s] = true
}

// WriteBeneath runs fn on a batch whose writes go into this outermost
// batch's change set beneath its records, and commits it there. The accounts
// it writes are not accounts this batch changed: they are not among its
// updated accounts, and its block ledger does not name them. The batch fn is
// given reads the change set raw, never the gate, and updates no tree: the
// caller swaps the section's marker for its leaves in this batch's tree.
//
// It is for a batch that has read nothing of the accounts fn writes: a record
// this batch holds is not reread.
func (b *Batch) WriteBeneath(fn func(*Batch) error) error {
	if b.parent != nil {
		return errors.InternalError.With("a section is written beneath the outermost batch only")
	}
	under := b.kvs()
	var cs keyvalue.ChangeSet
	switch g, ok := under.(*gatedChangeSet); {
	case ok && !g.writable:
		cs = memory.NewChangeSet(memory.ChangeSetOptions{Get: g.raw, Commit: g.putOverlay})
	case ok:
		cs = g.ChangeSet.Begin(nil, true)
	default:
		cs = under.Begin(nil, true)
	}
	c := NewBatch(b.id+".beneath", cs, true, b.logger.L)
	c.observer = b.observer
	err := fn(c)
	if err != nil {
		c.Discard()
		return err
	}
	// Commit the records into the change set. Not Batch.Commit: that would
	// commit the tree too, and the caller owns the tree.
	err = c.baseCommit()
	if err != nil {
		c.Discard()
		return errors.UnknownError.Wrap(err)
	}
	return cs.Commit()
}
