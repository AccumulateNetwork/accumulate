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

// streamPause is how long a page waits once every peer has answered busy,
// doubling up to streamPauseMost.
var streamPause = struct{ first, most time.Duration }{250 * time.Millisecond, 10 * time.Second}

type peersKey struct{}

// WithPeers names the peers a pull may spread a chain's pages across: the
// source it is pulling from and the others that serve the account.
func WithPeers(ctx context.Context, peers []Source) context.Context {
	return context.WithValue(ctx, peersKey{}, peers)
}

// peersOf is the peers a stream from src spreads its pages across: src
// first, then the others WithPeers named.
func peersOf(ctx context.Context, src Source) []Source {
	out := []Source{src}
	others, _ := ctx.Value(peersKey{}).([]Source)
	for _, p := range others {
		if p != src {
			out = append(out, p)
		}
	}
	return out
}

// fetchEntries takes the entries [start, end) of one of the peer's chains
// into the store -- each element, its index entry, and when bodies is set the
// message behind it, proven by its hash (streamPage) -- parallelPages pages
// at a time, every page of the source first. A page the source answers busy
// (NotReady: a validator admits four queries at a time) is asked of the next
// peer, and once every peer has answered busy it waits a pause that doubles.
// Any other refusal of a page is the source's, and fails the stream from it:
// a peer that lies is refused as a source and the next is tried alone
// (FetchFrom), never mixed with it -- pages of two chains would never replay
// to one head. The entries are written in no order, each page in a batch of
// its own committed before the page is dropped (#4446); replayEntries then
// builds the chain's state over them in order.
func fetchEntries(ctx context.Context, peers []Source, pages Store, u *url.URL, name string, start, end, pageSize uint64, bodies bool) error {
	if start >= end {
		return nil
	}
	if pageSize == 0 {
		pageSize = 256
	}
	type chunk struct{ start, end uint64 }
	var chunks []chunk
	for at := start; at < end; at += pageSize {
		chunks = append(chunks, chunk{at, min(at+pageSize, end)})
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
		workers = min(parallelPages, len(chunks))
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range next {
				if err := fetchChunk(ctx, peers, pages, u, name, c.start, c.end, bodies); err != nil {
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

// fetchChunk takes [start, end) of a chain from the source, peers[0], or
// from the next peer when the source is busy, as fetchEntries says.
func fetchChunk(ctx context.Context, peers []Source, pages Store, u *url.URL, name string, start, end uint64, bodies bool) error {
	pause := streamPause.first
	for ctx.Err() == nil {
		var refusals []error
		for k, src := range peers {
			err := fetchChunkFrom(ctx, src, pages, u, name, start, end, bodies)
			if err == nil {
				return nil
			}
			if ctx.Err() != nil {
				return errors.UnknownError.Wrap(ctx.Err())
			}
			if !errors.Is(err, errors.NotReady) {
				return fmt.Errorf("entries %d..%d of %s: %s: %w", start, end-1, name, sourceName(k, src), err)
			}
			refusals = append(refusals, fmt.Errorf("%s: %w", sourceName(k, src), err))
		}
		// Every peer is busy: wait, and ask the source again.
		_ = stderrors.Join(refusals...)
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
