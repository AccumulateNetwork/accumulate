// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// parallelPages is how many pages of one chain a stream asks for at once,
// each of the next of its peers (Paul, 2026-10-05, #4529: "fetch in
// parallel/batched"). A validator admits four queries at a time
// (api/v3 queryGate) and a partition's followers join together, so this is
// small and spread, not large and on one peer.
const parallelPages = 4

// streamPause is how long a page waits once every peer has failed it and one
// was busy, doubling from first up to most, and give, how long it waits in
// all: a page every peer still fails then is the chain's failure from these
// peers (#4533).
type streamPauses struct{ first, most, give time.Duration }

var streamPause = streamPauses{250 * time.Millisecond, 10 * time.Second, 15 * time.Second}

type peersKey struct{}

// withPeers is the peers a pull may spread a chain's pages across, and which
// of them it is pulling from.
type withPeers struct {
	peers []Source
	turn  int
}

// WithPeers names the peers a pull may spread a chain's pages across, and
// which of them, by index, is the source it pulls from. A Source is an
// interface and not every one is comparable, so the source is named by its
// place, never compared.
func WithPeers(ctx context.Context, peers []Source, turn int) context.Context {
	return context.WithValue(ctx, peersKey{}, withPeers{peers, turn})
}

// peersOf is the peers a stream from src spreads its pages across: src
// first, then the others WithPeers named.
func peersOf(ctx context.Context, src Source) []Source {
	out := []Source{src}
	w, ok := ctx.Value(peersKey{}).(withPeers)
	if !ok {
		return out
	}
	for i, p := range w.peers {
		if i != w.turn {
			out = append(out, p)
		}
	}
	return out
}

// fetchEntries takes the entries [start, end) of one of the peer's chains
// into the store -- each element, its index entry, and when bodies is set the
// message behind it, proven by its hash (streamPage) -- parallelPages pages
// at a time, spread across the peers: each page is asked of the next peer in
// turn (#4532), a page a peer answers busy (NotReady: a validator admits four
// queries at a time) is asked of the one after, and once every peer has
// answered busy it waits a pause that doubles. Any other refusal of a page
// fails the stream. The entries are written in no order, each page in a
// batch of its own committed before the page is dropped (#4446);
// replayEntries then builds the chain's state over them in order, and a
// stream that does not replay to the head is taken again from the source
// alone (fetchAndReplay).
func fetchEntries(ctx context.Context, peers []Source, pages Store, u *url.URL, name string, start, end, pageSize uint64, bodies bool) error {
	if start >= end {
		return nil
	}
	if pageSize == 0 {
		pageSize = 256
	}
	type chunk struct {
		start, end uint64
		turn       int
	}
	var chunks []chunk
	for at, i := start, 0; at < end; at, i = at+pageSize, i+1 {
		chunks = append(chunks, chunk{at, min(at+pageSize, end), i % len(peers)})
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	next := make(chan chunk)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failed error
	fail := func(err error) {
		mu.Lock()
		if failed == nil {
			failed = err
		}
		mu.Unlock()
		cancel()
	}
	// Several pages at once only into a store whose batches are each
	// their own: pages begun from a batch are nested in it, and one batch
	// is not written from two goroutines.
	workers := 1
	if _, ok := pages.(*database.Database); ok {
		workers = min(parallelPages*len(peers), len(chunks))
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range next {
				if err := fetchChunk(ctx, peers, c.turn, pages, u, name, c.start, c.end, bodies); err != nil {
					fail(err)
					return
				}
			}
		}()
	}
	for _, c := range chunks {
		select {
		case next <- c:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(next)
	wg.Wait()
	if failed != nil {
		return failed
	}
	return errors.UnknownError.Wrap(ctx.Err())
}

// busyRefusal is what a validator's query gate says when its slots are taken
// (internal/api/v3, gate.go; executor spec, "Sync"): the one failure of a page
// that clears by itself in a moment.
const busyRefusal = "query capacity exhausted"

func isBusy(err error) bool {
	return errors.Is(err, errors.NotReady) && strings.Contains(err.Error(), busyRefusal)
}

// isUnreachable says the peer could not be dialed: the transport names a
// failed dial by the address it dialed (pkg/api/v3/message,
// RoutedTransport.dial: "dial /p2p/...: connection failed").
func isUnreachable(err error) bool {
	return strings.Contains(err.Error(), "dial /p2p/")
}

// passable says a page's failure says nothing about the entries -- the peer
// was busy, could not be reached, or refused as a node does while it joins
// (NotReady) -- so the page is asked of the next peer. Any other answer is
// the peer's, and wrong: it fails the attempt, and the source is refused as
// it always was.
func passable(err error) bool {
	return errors.Is(err, errors.NotReady) || isUnreachable(err)
}

// fetchChunk takes [start, end) of a chain from the peer at turn, or from
// the ones after it when that one does not serve it, as fetchEntries says.
// A peer that is busy, unreachable or joining is passed for the next
// (passable); once every peer has failed the page, it is asked again only
// when one of them was busy, after a pause that doubles, and for
// streamPause.give in all. Otherwise the page is the chain's failure from
// these peers, and the attempt fails for the caller to ask the next source
// (#4533: a follower's Directory pull waited on one peer's refusal for as
// long as the pull lasted; then, every source spreading its pages across a
// peer that could not be dialed, every source failed the chain).
func fetchChunk(ctx context.Context, peers []Source, turn int, pages Store, u *url.URL, name string, start, end uint64, bodies bool) error {
	pause := streamPause.first
	var waited time.Duration
	for ctx.Err() == nil {
		var failures []error
		busy := false
		for k := range peers {
			i := (turn + k) % len(peers)
			src := peers[i]
			err := fetchChunkFrom(ctx, src, pages, u, name, start, end, bodies)
			if err == nil {
				return nil
			}
			if ctx.Err() != nil {
				return errors.UnknownError.Wrap(ctx.Err())
			}
			if !passable(err) {
				return fmt.Errorf("entries %d..%d of %s: %s: %w", start, end-1, name, sourceName(i, src), err)
			}
			if isBusy(err) {
				busy = true
			}
			failures = append(failures, fmt.Errorf("%s: %w", sourceName(i, src), err))
		}
		if !busy {
			return fmt.Errorf("entries %d..%d of %s: no peer served them: %w", start, end-1, name, stderrors.Join(failures...))
		}
		if waited+pause > streamPause.give {
			return errors.NotReady.WithCauseAndFormat(stderrors.Join(failures...),
				"entries %d..%d of %s: every peer was busy for %v", start, end-1, name, waited)
		}
		waited += pause
		t := time.NewTimer(pause)
		select {
		case <-ctx.Done():
			t.Stop()
			return errors.UnknownError.Wrap(ctx.Err())
		case <-t.C:
		}
		pause = min(2*pause, streamPause.most)
	}
	return errors.UnknownError.Wrap(ctx.Err())
}

// fetchAndReplay takes the entries [start, end) of a chain across the peers
// and builds st over them in order; the result must reach want, the peer's
// head. A stream that does not -- a peer among them served another chain --
// is taken again from the source alone, and only when that does not reach
// the head either is it errNotThePeers: the source is refused, never a peer
// beside it (#4532).
func fetchAndReplay(ctx context.Context, src Source, peers []Source, pages Store, u *url.URL, name string, st, want *merkle.State, start, end, pageSize uint64, bodies bool) error {
	st0 := st.Copy()
	try := func(peers []Source) error {
		if err := fetchEntries(ctx, peers, pages, u, name, start, end, pageSize, bodies); err != nil {
			return err
		}
		return replayEntries(pages, u, name, st, start, end)
	}
	reaches := func() bool {
		return st.Count == want.Count && bytes.Equal(st.Anchor(), want.Anchor())
	}
	if err := try(peers); err != nil {
		return err
	}
	if reaches() || len(peers) == 1 {
		return nil
	}
	*st = *st0.Copy()
	if err := try([]Source{src}); err != nil {
		return err
	}
	return nil
}

// fetchChunkFrom takes [start, end) of a chain from one peer, in as many
// answers as it takes, each written in a batch of its own.
func fetchChunkFrom(ctx context.Context, src Source, pages Store, u *url.URL, name string, start, end uint64, bodies bool) error {
	for start < end {
		page := pages.Begin(true)
		next, err := streamPage(ctx, src, page, u, name, start, end-start, bodies, func(page *database.Batch, _ *messages, index uint64, entry []byte) error {
			pc, err := page.Account(u).ChainByName(name)
			if err != nil {
				return err
			}
			if err := pc.Inner().ElementIndex(entry).Put(index); err != nil {
				return err
			}
			return pc.Inner().Element(index).Put(entry)
		})
		if err == nil {
			err = page.Commit()
		}
		if err != nil {
			page.Discard()
			return err
		}
		start = next
	}
	return nil
}

// replayEntries builds the chain's state over the entries [start, end) the
// store holds, in order, from st: the intermediates and mark points the
// entries imply, exactly what appending them wrote on the peer
// (merkle.Chain.ReplayBelow), replayChunk entries a batch. st ends as the
// state after entry end-1; the caller holds it to the peer's head.
func replayEntries(pages Store, u *url.URL, name string, st *merkle.State, start, end uint64) error {
	for from := start; from < end; {
		to := min(from+replayChunk, end)
		batch := pages.Begin(true)
		c, err := batch.Account(u).ChainByName(name)
		if err != nil {
			batch.Discard()
			return err
		}
		chain := c.Inner()
		for i := from; i < to; i++ {
			e, err := chain.Element(i).Get()
			if err != nil {
				batch.Discard()
				return fmt.Errorf("replay entry %d of %s: %w", i, name, err)
			}
			if err := chain.ReplayBelow(st, e); err != nil {
				batch.Discard()
				return err
			}
		}
		if err := batch.Commit(); err != nil {
			return err
		}
		from = to
	}
	return nil
}
