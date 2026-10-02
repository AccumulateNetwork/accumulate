// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	stderrors "errors"
	"fmt"
	"sort"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Backfill brings the entries of an account the node took by its chain heads
// alone into its store: for every chain the node holds, the entries below the
// open mark set, the mark points that close their sets, and -- on a
// transaction chain -- the message behind every entry, the open set's
// included (executor spec, "Sync",
// "Two mismatches": the node ends holding every account's chains and
// entries, not only a root that matches).
//
// It does not touch a chain's head or its open mark set, so it may run while
// the node executes: what it writes lies below the head, where no block
// appends.
//
// A chain is walked from its head down to its oldest entry, a page at a time,
// and a peer is asked only for the entries of a page the store does not hold
// (executor spec, "Past the match"; #4493). Each page is written to store and
// dropped before the next is asked for (#4446), so memory is bounded by the
// page and not by the chain, and with it the chain's SyncedEntries record:
// how far the walk has got. A retry, or the next process, resumes from it:
// it steps over what an earlier walk brought in and asks for nothing held. A
// walk with no earlier one unfinished below it stops at the first entry that
// exists below what it fetched -- a chain held whole and resynced asks for
// what is new and no more -- and every walk stops at the record's max synced
// entry, below which everything is held.
//
// Once a walk has reached the bottom, the entries are held to the node's own
// head: replayed from the last mark point below where the walk stopped, they
// and the node's open set must reproduce it. The replay is local, and writes
// the mark points and intermediates the elements alone do not. Entries that
// do not reproduce the head are not trusted again: the record says so, and
// the next walk asks for every entry below the head, overwriting position by
// position.
//
// progress, when not nil, is told after every page how far down a chain the
// walk has got and the chain's height (#4472): one account of the Directory
// can take minutes, and a caller that says nothing for that long cannot be
// told from one that is stuck.
func Backfill(ctx context.Context, srcs []Source, store Store, u *url.URL, pageSize uint64, progress func(chain string, at, total uint64)) error {
	if len(srcs) == 0 {
		return errors.BadRequest.With("pull.Backfill: at least one source required")
	}
	if pageSize == 0 {
		pageSize = 256
	}
	// The node's heads, read in a view closed before any page is written: a
	// store that keeps isolation by pre-image keeps every page's for as long
	// as a view begun before it is open (#4446).
	chains, err := backfillHeads(store, u)
	if err != nil {
		return err
	}
	for _, h := range chains {
		err := backfillChain(ctx, srcs, store, u, h, pageSize, progress)
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return errors.UnknownError.Wrap(ctx.Err())
		}
		return errors.Conflict.WithFormat("%v: chain %s: %w", u, h.name, err)
	}
	return nil
}

// backfillHead is a chain's head as the node holds it, with its open mark set.
type backfillHead struct {
	name     string
	typ      merkle.ChainType
	head     *merkle.State
	open     [][]byte
	boundary uint64
	markFreq uint64
}

// backfillHeads reads the head of every chain of u the node holds entries of.
func backfillHeads(store Store, u *url.URL) ([]*backfillHead, error) {
	batch := store.Begin(false)
	defer batch.Discard()
	metas, err := batch.Account(u).Chains().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("%v: load the chain index: %w", u, err)
	}
	var out []*backfillHead
	for _, meta := range metas {
		c, err := batch.Account(u).ChainByName(meta.Name)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("%v: chain %s: %w", u, meta.Name, err)
		}
		chain := c.Inner()
		head, err := chain.Head().Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("%v: chain %s: load the head: %w", u, meta.Name, err)
		}
		if head.Count == 0 {
			continue
		}
		open, err := chain.OpenSet(head)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("%v: chain %s: load the open set: %w", u, meta.Name, err)
		}
		out = append(out, &backfillHead{
			name:     meta.Name,
			typ:      meta.Type,
			head:     head.Copy(),
			open:     open,
			boundary: merkle.BoundaryFor(head.Count, chain.MarkFreq()),
			markFreq: uint64(chain.MarkFreq()),
		})
	}
	return out, nil
}

// replayChunk is how many entries the replay writes in one batch.
const replayChunk = 4096

// chainWalk is one walk of a chain from its head down.
type chainWalk struct {
	u        *url.URL
	h        *backfillHead
	bodies   bool
	rec      *database.SyncedEntries
	top      uint64 // The head count the walk started from
	floor    uint64 // Where the walk stops at the latest
	at       uint64 // Every entry from at up to top is held
	fetching bool   // The walk has found an entry the store does not hold
	fetched  bool   // The walk has brought in an entry
	next     int    // The source asked first
}

// backfillChain walks one chain from its head down, bringing in the entries
// the store does not hold, then holds them to the head.
func backfillChain(ctx context.Context, srcs []Source, store Store, u *url.URL, h *backfillHead, pageSize uint64, progress func(chain string, at, total uint64)) error {
	w := &chainWalk{
		u:      u,
		h:      h,
		bodies: carriesMessages(&api.ChainRecord{Name: h.name, Type: h.typ}),
		top:    uint64(h.head.Count),
	}
	if err := w.load(store); err != nil {
		return err
	}
	w.at = w.top
	if w.floor >= w.top {
		return nil // Held whole up to the head
	}

	for w.at > w.floor {
		if s := w.spanAt(w.at); s != nil {
			// What an earlier walk brought in: step over it.
			w.at = s.Low
			w.fetching = true
			w.fetched = true
			continue
		}
		stop, err := w.page(ctx, srcs, store, pageSize)
		if err != nil {
			return err
		}
		if progress != nil {
			progress(h.name, w.at, w.top)
		}
		if stop {
			break
		}
	}
	return w.replay(store)
}

// load reads the chain's record and sets where the walk stops at the latest.
func (w *chainWalk) load(store Store) error {
	batch := store.Begin(false)
	defer batch.Discard()
	rec, err := batch.Account(w.u).SyncedEntries(w.h.name).Get()
	if err != nil {
		return errors.UnknownError.WithFormat("load the synced entries of %s: %w", w.h.name, err)
	}
	w.rec = rec.Copy()
	if w.rec.Refetch {
		// What was held did not reproduce the head: trust none of it.
		w.rec.Spans = nil
		w.floor = 0
		return nil
	}
	w.floor = min(w.rec.Synced, w.top)
	var spans []*database.SyncedSpan
	for _, s := range w.rec.Spans {
		if s.Low < s.Top && s.Top <= w.top && s.Low >= w.floor {
			spans = append(spans, s)
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].Top > spans[j].Top })
	w.rec.Spans = spans
	return nil
}

// spanAt is the span an earlier walk recorded that holds the entry below at.
func (w *chainWalk) spanAt(at uint64) *database.SyncedSpan {
	for _, s := range w.rec.Spans {
		if s.Low < at && at <= s.Top {
			return s
		}
	}
	return nil
}

// spanBelow is whether an earlier walk that did not finish lies below at.
func (w *chainWalk) spanBelow(at uint64) bool {
	for _, s := range w.rec.Spans {
		if s.Top <= at {
			return true
		}
	}
	return false
}

// page walks the next page down from w.at: it finds the entries the store
// does not hold, brings them in, and records how far the walk has got. It
// reports whether the walk stops: it has met an entry that exists below what
// it fetched, with no unfinished walk below it.
func (w *chainWalk) page(ctx context.Context, srcs []Source, store Store, pageSize uint64) (bool, error) {
	lo := w.floor
	if w.at-w.floor > pageSize {
		lo = w.at - pageSize
	}
	batch := store.Begin(true)
	defer batch.Discard()
	c, err := batch.Account(w.u).ChainByName(w.h.name)
	if err != nil {
		return false, err
	}
	chain := c.Inner()

	bottom, stop := lo, false
	var absent []uint64 // Descending
	for i := w.at; i > lo; i-- {
		index := i - 1
		if w.spanAt(i) != nil {
			bottom = i // Stepped over on the next pass
			break
		}
		held, err := w.held(batch, chain, index)
		if err != nil {
			return false, err
		}
		if !held {
			w.fetching = true
			absent = append(absent, index)
			continue
		}
		if w.fetching && !w.spanBelow(i) {
			// Paul, 2026-10-02: "When resyncing, one stops once the first
			// entry exists." Everything older is held.
			bottom, stop = i, true
			break
		}
	}

	// Ask for the runs of absent entries, oldest first.
	for len(absent) > 0 {
		end := absent[0] + 1
		n := 1
		for n < len(absent) && absent[n] == absent[n-1]-1 {
			n++
		}
		start := absent[n-1]
		absent = absent[n:]
		if err := w.fetch(ctx, srcs, batch, start, end-start); err != nil {
			return false, err
		}
		w.fetched = true
	}

	w.at = bottom
	w.record()
	if err := batch.Account(w.u).SyncedEntries(w.h.name).Put(w.rec); err != nil {
		return false, err
	}
	return stop, batch.Commit()
}

// held is whether the store holds the entry at index -- on a transaction
// chain, with the message behind it. A walk that refetches holds nothing.
func (w *chainWalk) held(batch *database.Batch, chain *merkle.Chain, index uint64) (bool, error) {
	if w.rec.Refetch {
		return false, nil
	}
	h, err := chain.Element(index).Get()
	switch {
	case err == nil:
	case errors.Is(err, errors.NotFound):
		return false, nil
	default:
		return false, errors.UnknownError.WithFormat("load entry %d: %w", index, err)
	}
	if !w.bodies {
		return true, nil
	}
	_, err = batch.Message2(h).Main().Get()
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, errors.NotFound):
		return false, nil
	default:
		return false, errors.UnknownError.WithFormat("load the message behind entry %d: %w", index, err)
	}
}

// record puts the walk's own span first in the record, joined with every
// span it has stepped over.
func (w *chainWalk) record() {
	spans := []*database.SyncedSpan{{Top: w.top, Low: w.at}}
	for _, s := range w.rec.Spans {
		if s.Top <= w.at {
			spans = append(spans, s)
		}
	}
	w.rec.Spans = spans
}

// backfillRetry is how long the backfill waits before it asks again for a
// page every peer refused for capacity: first, doubling up to most.
var backfillRetry = struct{ first, most time.Duration }{250 * time.Millisecond, 10 * time.Second}

// fetch brings in the count entries from start, from the first source that
// serves them, starting with the one that served the last.
//
// A peer's NotReady -- its query capacity exhausted, as when several
// followers pull from one validator at once -- is not a refusal of the
// entries (#4493): the next peer is asked, and once every peer has been asked
// and one of them was only busy, the page is asked again after a pause that
// doubles. Only when every peer refuses the page otherwise does the walk fail;
// what it wrote before is in the record, and the next attempt resumes there.
func (w *chainWalk) fetch(ctx context.Context, srcs []Source, batch *database.Batch, start, count uint64) error {
	pause := backfillRetry.first
	for {
		var refusals []error
		busy := false
		for k := range srcs {
			i := (w.next + k) % len(srcs)
			err := w.fetchFrom(ctx, srcs[i], batch, start, count)
			if err == nil {
				w.next = i
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, errors.NotReady) {
				busy = true
			}
			refusals = append(refusals, fmt.Errorf("%s: %w", sourceName(i, srcs[i]), err))
		}
		if !busy {
			return errors.NotFound.WithFormat("no source served entries %d..%d: %w", start, start+count-1, stderrors.Join(refusals...))
		}
		// The next round starts with the next peer.
		w.next = (w.next + 1) % len(srcs)
		t := time.NewTimer(pause)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
		pause = min(2*pause, backfillRetry.most)
	}
}

// fetchFrom brings in the count entries from start from one source. An entry
// of the open set must be the node's own.
func (w *chainWalk) fetchFrom(ctx context.Context, src Source, batch *database.Batch, start, count uint64) error {
	end := start + count
	for start < end {
		next, err := streamPage(ctx, src, batch, w.u, w.h.name, start, end-start, w.bodies, func(page *database.Batch, _ *messages, index uint64, e []byte) error {
			if index >= w.h.boundary && string(w.h.open[index-w.h.boundary]) != string(e) {
				return errors.Conflict.WithFormat("the entry served at %d is not the node's", index)
			}
			c, err := page.Account(w.u).ChainByName(w.h.name)
			if err != nil {
				return err
			}
			chain := c.Inner()
			// The index of a hash names where it was last written; one the
			// chain already indexes is left to what the node wrote since.
			_, err = chain.ElementIndex(e).Get()
			switch {
			case err == nil:
			case errors.Is(err, errors.NotFound):
				if err := chain.ElementIndex(e).Put(index); err != nil {
					return err
				}
			default:
				return err
			}
			return chain.Element(index).Put(e)
		})
		if err != nil {
			return err
		}
		start = next
	}
	return nil
}

// replay holds what the walk brought in to the node's head: from the last
// mark point at or below where the walk stopped, the held entries are
// replayed onto the state there, writing the mark points and intermediates,
// and must reproduce the head. The record then says the chain is held whole.
func (w *chainWalk) replay(store Store) error {
	if !w.fetched {
		// Nothing was brought in: every entry from the head down to where
		// the walk stopped is the node's own.
		return w.done(store)
	}

	freq := w.h.markFreq
	from := w.at / freq * freq
	st := new(merkle.State)
	{
		batch := store.Begin(false)
		c, err := batch.Account(w.u).ChainByName(w.h.name)
		if err != nil {
			batch.Discard()
			return err
		}
		for ; from > 0; from -= freq {
			mark, err := c.Inner().States(from - 1).Get()
			if err == nil {
				st = mark.Copy()
				st.HashList = nil
				break
			}
			if !errors.Is(err, errors.NotFound) {
				batch.Discard()
				return errors.UnknownError.WithFormat("load the mark point at %d: %w", from, err)
			}
		}
		batch.Discard()
	}

	for from < w.top {
		end := min(from+replayChunk, w.top)
		batch := store.Begin(true)
		c, err := batch.Account(w.u).ChainByName(w.h.name)
		if err != nil {
			batch.Discard()
			return err
		}
		chain := c.Inner()
		for i := from; i < end; i++ {
			e, err := chain.Element(i).Get()
			if errors.Is(err, errors.NotFound) {
				// Below where the walk stopped the chain was not whole.
				batch.Discard()
				return w.refuse(store, fmt.Sprintf("entry %d is not held", i))
			}
			if err != nil {
				batch.Discard()
				return errors.UnknownError.WithFormat("replay entry %d: %w", i, err)
			}
			if err := chain.ReplayBelow(st, e); err != nil {
				batch.Discard()
				return err
			}
		}
		if err := batch.Commit(); err != nil {
			return err
		}
		from = end
	}

	head := w.h.head
	if uint64(st.Count) != w.top || string(st.Anchor()) != string(head.Anchor()) {
		return w.refuse(store, fmt.Sprintf("the entries held below %d do not reproduce the node's head", w.top))
	}
	return w.done(store)
}

// refuse records that what is held is not trusted again -- the next walk asks
// for every entry below the head -- and fails the walk.
func (w *chainWalk) refuse(store Store, why string) error {
	w.rec.Refetch = true
	w.rec.Spans = nil
	if err := w.put(store); err != nil {
		return err
	}
	return errors.Conflict.With(why)
}

// done records the chain held whole up to the walk's head.
func (w *chainWalk) done(store Store) error {
	w.rec.Synced = w.top
	w.rec.Spans = nil
	w.rec.Refetch = false
	return w.put(store)
}

// put writes the chain's record.
func (w *chainWalk) put(store Store) error {
	batch := store.Begin(true)
	defer batch.Discard()
	if err := batch.Account(w.u).SyncedEntries(w.h.name).Put(w.rec); err != nil {
		return err
	}
	return batch.Commit()
}
