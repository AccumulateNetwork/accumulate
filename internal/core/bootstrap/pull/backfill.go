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
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Backfill brings in what the node is missing of an account's chains, as
// each chain's SyncedEntries record describes it: the gaps the pull left
// when it took the chain by its head (executor spec, "Sync", "Past the
// match"; #4493). It asks for exactly the gaps, and never asks the store
// which entries it holds; an account whose chains have no gaps is whole and
// asks for nothing.
//
// It does not touch a chain's head or its open mark set, so it may run while
// the node executes: what it writes lies below the head, where no block
// appends.
//
// A chain is walked from the head it was taken at down to entry 0, one
// perfect subtree of that head at a time -- one for each bit of its count,
// newest and smallest first -- so every entry is proven against the head:
//
//   - entries are asked for a page at a time, a page never crossing a
//     subtree's boundary, each page written and dropped before the next is
//     asked for (#4446);
//   - once a subtree's entries are all held they are replayed from the state
//     at its first entry, writing the mark points and intermediates, and must
//     hash to the subtree's pending root. Only then is its gap closed, in the
//     replay's last commit: until then its pages are held unproven, in the
//     gap, and a retry asks for them again;
//   - a message is proven by hashing to its entry, and an entry whose message
//     alone was missing must be the one the node holds. Those pages are asked
//     for first, newest first, and need not keep to a subtree; each is closed
//     in the commit that writes it.
//
// progress, when not nil, is told after every page how far down a chain the
// walk has got and the chain's height (#4472).
func Backfill(ctx context.Context, srcs []Source, store Store, u *url.URL, pageSize uint64, progress func(chain string, at, total uint64)) error {
	if len(srcs) == 0 {
		return errors.BadRequest.With("pull.Backfill: at least one source required")
	}
	if pageSize == 0 {
		pageSize = backfillPage
	}
	// The records, read in a view closed before any page is written: a store
	// that keeps isolation by pre-image keeps every page's for as long as a
	// view begun before it is open (#4446).
	walks, err := backfillWalks(store, u)
	if err != nil {
		return err
	}
	for _, w := range walks {
		w.progress = progress
		err := w.walk(ctx, srcs, store, pageSize)
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return errors.UnknownError.Wrap(ctx.Err())
		}
		return errors.Conflict.WithFormat("%v: chain %s: %w", u, w.name, err)
	}
	return nil
}

// backfillPage is how many entries the backfill asks for at once (Paul,
// 2026-10-05).
const backfillPage = 200

// replayChunk is how many entries the replay writes in one batch.
const replayChunk = 4096

// chainWalk is one walk of a chain from the head it was taken at down.
type chainWalk struct {
	u        *url.URL
	name     string
	bodies   bool
	markFreq uint64
	rec      *database.SyncedEntries
	head     *merkle.State
	next     int    // The source asked first
	at       uint64 // How far down the walk has got

	progress func(chain string, at, total uint64)
}

// backfillWalks reads the record of every chain of u, and returns a walk for
// each that is missing something.
func backfillWalks(store Store, u *url.URL) ([]*chainWalk, error) {
	batch := store.Begin(false)
	defer batch.Discard()
	metas, err := batch.Account(u).Chains().Get()
	if err != nil {
		return nil, errors.UnknownError.WithFormat("%v: load the chain index: %w", u, err)
	}
	var out []*chainWalk
	for _, meta := range metas {
		rec, err := batch.Account(u).SyncedEntries(meta.Name).Get()
		if err != nil {
			return nil, errors.UnknownError.WithFormat("%v: load the synced entries of %s: %w", u, meta.Name, err)
		}
		if len(rec.Gaps) == 0 {
			continue
		}
		c, err := batch.Account(u).ChainByName(meta.Name)
		if err != nil {
			return nil, errors.UnknownError.WithFormat("%v: chain %s: %w", u, meta.Name, err)
		}
		rec = rec.Copy()
		out = append(out, &chainWalk{
			u:        u,
			name:     meta.Name,
			bodies:   carriesMessages(&api.ChainRecord{Name: meta.Name, Type: meta.Type}),
			markFreq: uint64(c.Inner().MarkFreq()),
			rec:      rec,
			head:     headOf(rec),
		})
	}
	return out, nil
}

// walk brings the chain in, subtree by subtree, from the head down.
func (w *chainWalk) walk(ctx context.Context, srcs []Source, store Store, pageSize uint64) error {
	count := uint64(w.head.Count)
	w.at = count

	// Messages behind entries the node holds: each is proven by hashing to
	// its entry, so their pages need not keep to a subtree, and each page is
	// closed in the commit that writes it.
	for _, r := range w.within(0, count, true) {
		for top := r[1]; top > r[0]; {
			start := max(r[0], top-min(top, pageSize))
			if err := w.messagesPage(ctx, srcs, store, start, top); err != nil {
				return err
			}
			top = start
			w.reached(top)
		}
	}

	for k := 0; count>>k != 0; k++ {
		if count>>k&1 == 0 {
			continue
		}
		low := count &^ (1<<(k+1) - 1)
		high := low + 1<<k
		if err := w.subtree(ctx, srcs, store, k, low, high, pageSize); err != nil {
			return err
		}
	}
	w.reached(0)
	if len(w.rec.Gaps) != 0 {
		// A gap the head's subtrees do not cover: the record is not one
		// this walk can fill.
		g := w.rec.Gaps[0]
		return errors.InternalError.WithFormat("the record names [%d, %d) missing, outside the head it was taken at (%d)", g.Low, g.High, count)
	}
	return nil
}

// reached tells the caller the walk has got down to at, if that is further
// than it had.
func (w *chainWalk) reached(at uint64) {
	if at >= w.at {
		return
	}
	w.at = at
	if w.progress != nil {
		w.progress(w.name, at, uint64(w.head.Count))
	}
}

// within is the record's gaps of one kind inside [low, high), clipped to it,
// newest first.
func (w *chainWalk) within(low, high uint64, messagesOnly bool) [][2]uint64 {
	var out [][2]uint64
	for _, g := range w.rec.Gaps {
		if g.MessagesOnly != messagesOnly || g.High <= low || g.Low >= high {
			continue
		}
		out = append(out, [2]uint64{max(g.Low, low), min(g.High, high)})
	}
	return out
}

// subtree brings in the entries missing of the head's subtree of height k,
// the entries [low, high), and proves them.
func (w *chainWalk) subtree(ctx context.Context, srcs []Source, store Store, k int, low, high, pageSize uint64) error {
	gaps := w.within(low, high, false)
	if len(gaps) == 0 {
		return nil
	}
	if low%w.markFreq != 0 || (high-low)%w.markFreq != 0 {
		// Entries are missing only below the open mark set, where every
		// subtree starts and ends at a mark point.
		return errors.InternalError.WithFormat("entries [%d, %d) are missing in a subtree that does not start at a mark point", gaps[0][0], gaps[0][1])
	}
	for _, r := range gaps {
		for top := r[1]; top > r[0]; {
			start := max(r[0], top-min(top, pageSize))
			if err := w.entriesPage(ctx, srcs, store, start, top); err != nil {
				return err
			}
			top = start
			w.reached(top)
		}
	}
	return w.prove(store, k, low, high, gaps)
}

// messagesPage brings in the messages behind the entries [start, end), which
// the node holds.
func (w *chainWalk) messagesPage(ctx context.Context, srcs []Source, store Store, start, end uint64) error {
	batch := store.Begin(true)
	defer batch.Discard()
	err := w.fetch(ctx, srcs, start, end-start, func(src Source, start, count uint64) (uint64, error) {
		return streamPage(ctx, src, batch, w.u, w.name, start, count, true, func(page *database.Batch, _ *messages, index uint64, e []byte) error {
			c, err := page.Account(w.u).ChainByName(w.name)
			if err != nil {
				return err
			}
			held, err := c.Inner().Element(index).Get()
			if err != nil {
				return errors.UnknownError.WithFormat("load entry %d: %w", index, err)
			}
			if string(held) != string(e) {
				return errors.Conflict.WithFormat("the entry served at %d is not the node's", index)
			}
			return nil
		})
	})
	if err != nil {
		return err
	}
	closeGap(w.rec, start, end)
	if err := batch.Account(w.u).SyncedEntries(w.name).Put(w.rec); err != nil {
		return err
	}
	return batch.Commit()
}

// entriesPage brings in the entries [start, end), and on a chain that
// carries messages the message behind each. They are held unproven: the gap
// stays open until the subtree they are in is proven.
func (w *chainWalk) entriesPage(ctx context.Context, srcs []Source, store Store, start, end uint64) error {
	batch := store.Begin(true)
	defer batch.Discard()
	err := w.fetch(ctx, srcs, start, end-start, func(src Source, start, count uint64) (uint64, error) {
		return streamPage(ctx, src, batch, w.u, w.name, start, count, w.bodies, func(page *database.Batch, _ *messages, index uint64, e []byte) error {
			c, err := page.Account(w.u).ChainByName(w.name)
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
	})
	if err != nil {
		return err
	}
	return batch.Commit()
}

// prove replays the subtree of height k, the entries [low, high), from the
// state at low, writing the mark points and intermediates, and holds the
// result to the head's pending root at k. The entry gaps are closed in the
// replay's last commit. A subtree that does not hash to its root is not
// trusted at all -- a peer's page may be wrong, or the node's own entries, as
// where a repaired chain grew wrongly (#4519) -- and is recorded missing
// whole: the next attempt asks for every entry of it, starting with the next
// peer.
func (w *chainWalk) prove(store Store, k int, low, high uint64, gaps [][2]uint64) error {
	st := &merkle.State{Count: int64(low)}
	for j := range w.head.Pending {
		if j > k {
			st.Pending = append(st.Pending, w.head.Pending[j])
		} else {
			st.Pending = append(st.Pending, nil)
		}
	}

	for from := low; from < high; {
		end := min(from+replayChunk, high)
		batch := store.Begin(true)
		c, err := batch.Account(w.u).ChainByName(w.name)
		if err != nil {
			batch.Discard()
			return err
		}
		chain := c.Inner()
		for i := from; i < end; i++ {
			e, err := chain.Element(i).Get()
			if errors.Is(err, errors.NotFound) {
				batch.Discard()
				return w.refute(store, low, high, fmt.Sprintf("entry %d of [%d, %d) is not held", i, low, high))
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
		if end == high {
			if k >= len(st.Pending) || k >= len(w.head.Pending) || string(st.Pending[k]) != string(w.head.Pending[k]) {
				batch.Discard()
				return w.refute(store, low, high, fmt.Sprintf("entries [%d, %d) do not hash to the head's root of that subtree", low, high))
			}
			for _, r := range gaps {
				closeGap(w.rec, r[0], r[1])
			}
			if err := batch.Account(w.u).SyncedEntries(w.name).Put(w.rec); err != nil {
				batch.Discard()
				return err
			}
		}
		if err := batch.Commit(); err != nil {
			return err
		}
		from = end
	}
	return nil
}

// backfillRetry is how long the backfill waits before it asks again for a
// page every peer refused for capacity: first, doubling up to most.
var backfillRetry = struct{ first, most time.Duration }{250 * time.Millisecond, 10 * time.Second}

// fetch brings in the count entries from start, from the first source that
// serves them, starting with the one that served the last. page asks one
// source for entries from start and returns the entry after the last it was
// given: an answer may hold fewer than were asked for (at most 200 entries
// and 50,000 bytes), and the rest are asked for again.
//
// A peer's NotReady -- its query capacity exhausted, as when several
// followers pull from one validator at once -- is not a refusal of the
// entries (#4493): the next peer is asked, and once every peer has been asked
// and one of them was only busy, the page is asked again after a pause that
// doubles. Only when every peer refuses the page otherwise does the walk fail;
// the record keeps what was proven, and the next attempt resumes there.
func (w *chainWalk) fetch(ctx context.Context, srcs []Source, start, count uint64, page func(src Source, start, count uint64) (uint64, error)) error {
	pause := backfillRetry.first
	end := start + count
	for {
		var refusals []error
		busy := false
		for k := range srcs {
			i := (w.next + k) % len(srcs)
			var err error
			for start < end && err == nil {
				var next uint64
				next, err = page(srcs[i], start, end-start)
				if err == nil {
					start = next
				}
			}
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
			return errors.NotFound.WithFormat("no source served entries %d..%d: %w", start, end-1, stderrors.Join(refusals...))
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

// refute records the subtree [low, high) missing whole, entries missing, and
// fails the walk: the next attempt asks for every entry of it, starting with
// the next peer.
func (w *chainWalk) refute(store Store, low, high uint64, why string) error {
	w.next++
	closeGap(w.rec, low, high)
	w.rec.Gaps = mergeGaps(append(w.rec.Gaps, &database.SyncedGap{Low: low, High: high}))
	if err := w.put(store); err != nil {
		return err
	}
	return errors.Conflict.With(why)
}

// put writes the chain's record.
func (w *chainWalk) put(store Store) error {
	batch := store.Begin(true)
	defer batch.Discard()
	if err := batch.Account(w.u).SyncedEntries(w.name).Put(w.rec); err != nil {
		return err
	}
	return batch.Commit()
}
