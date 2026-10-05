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

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// streamChainsWindow takes, of every chain of u, the entries of the last
// window blocks of the peer's partition before block -- and at least the
// mark set that holds the chain's last entry -- with the messages behind
// them, streamed as a whole chain is (#4446). What lies below is left to
// Backfill (executor spec, "Sync"; #4452).
//
// A chain the node already holds from the window up is brought to the peer's
// head from its own, as a whole chain is; any other is taken from the
// window's first mark point, over whatever the node held.
func streamChainsWindow(ctx context.Context, src Source, pages Store, u *url.URL, pageSize, block, window uint64, checkHeld, retakeLonger bool) (*streamedChains, error) {
	chains, err := src.QueryAccountChains(ctx, u, &api.ChainQuery{})
	if err != nil {
		return nil, fmt.Errorf("list chains: %w", err)
	}
	out := &streamedChains{retakeLonger: retakeLonger}
	if chains == nil {
		return out, nil
	}
	out.listed = chains.Records
	byName := map[string]*api.ChainRecord{}
	for _, c := range chains.Records {
		if c != nil && c.Name != "" {
			byName[c.Name] = c
		}
	}
	found := map[string]indexFound{}
	for _, c := range mainFirst(chains.Records) {
		t, err := streamChainWindow(ctx, src, pages, u, c, byName, found, pageSize, block, window, checkHeld, retakeLonger)
		if err != nil {
			return nil, fmt.Errorf("chain %s/%s: %w", u, c.Name, err)
		}
		out.taken = append(out.taken, t)
	}
	return out, nil
}

// mainFirst orders an account's chains with the main chain first: an anchor
// signature's page writes what executing the anchor wrote only for an anchor
// on the main chain, and it looks there for it (messages.isExecuted).
func mainFirst(records []*api.ChainRecord) []*api.ChainRecord {
	ordered := make([]*api.ChainRecord, 0, len(records))
	for _, c := range records {
		if c != nil && c.Name == "main" {
			ordered = append(ordered, c)
		}
	}
	for _, c := range records {
		if c != nil && c.Name != "main" && c.Name != "" {
			ordered = append(ordered, c)
		}
	}
	return ordered
}

// indexFound is where a search of an index chain found the window: the first
// entry in it and the entry before, nil when there is none. A chain and its
// index chain share one search.
type indexFound struct {
	k      uint64
	before *protocol.IndexEntry
}

func streamChainWindow(ctx context.Context, src Source, pages Store, u *url.URL, c *api.ChainRecord, byName map[string]*api.ChainRecord, found map[string]indexFound, pageSize, block, window uint64, checkHeld, retakeLonger bool) (*streamedChain, error) {
	var markFreq, held int64
	err := func() error {
		local := pages.Begin(false)
		defer local.Discard()
		lc, err := local.Account(u).ChainByName(c.Name)
		if err != nil {
			return err
		}
		markFreq = lc.Inner().MarkFreq()
		head, err := lc.Inner().Head().Get()
		if err != nil {
			return fmt.Errorf("load the local head: %w", err)
		}
		held = head.Count
		return nil
	}()
	if err != nil {
		return nil, err
	}

	start, err := windowStart(ctx, src, u, c, byName, found, pageSize, block, window, markFreq)
	if err != nil {
		return nil, fmt.Errorf("find the window: %w", err)
	}

	// Held from the window up: brought to the peer's head from the node's
	// own, with the messages behind the newest held entries an earlier join
	// left without them (Options.CheckHeld).
	if held >= int64(start) {
		t, err := streamChain(ctx, src, pages, u, c, pageSize, checkHeld, false)
		switch {
		case err == nil:
			return t, nil
		case stderrors.Is(err, errNotThePeers), retakeLonger && stderrors.Is(err, errLongerThanThePeers):
		default:
			return nil, err
		}
	}
	return streamChainFrom(ctx, src, pages, u, c, start, pageSize)
}

// windowStart is the position a chain's window starts at: the mark point at
// or below the first entry of the peer's block block-window, found through
// the chain's index chain, and never above the mark set holding the chain's
// last entry. A chain with no index chain, or a window reaching past the
// partition's first block, is taken from the lower of the two.
func windowStart(ctx context.Context, src Source, u *url.URL, c *api.ChainRecord, byName map[string]*api.ChainRecord, found map[string]indexFound, pageSize, block, window uint64, markFreq int64) (uint64, error) {
	if c.Count == 0 {
		return 0, nil
	}
	start := c.Count - 1
	switch {
	case window == 0:
	case block <= window:
		start = 0 // The partition's whole history is inside the window
	default:
		index := c
		if c.Type != merkle.ChainTypeIndex {
			index = byName[c.Name+"-index"]
		}
		if index == nil || index.Count == 0 {
			break
		}
		f, ok := found[index.Name]
		if !ok {
			k, before, err := firstIndexFrom(ctx, src, u, index, block-window, pageSize)
			if err != nil {
				return 0, err
			}
			f = indexFound{k, before}
			found[index.Name] = f
		}
		k, before := f.k, f.before
		var s uint64
		switch {
		case index == c && k > 0:
			s = k - 1 // With the entry before, which a search of it reads
		case index == c, before == nil:
			s = 0
		default:
			s = before.Source // The last entry of the block before the window
		}
		if s < start {
			start = s
		}
	}
	return start &^ uint64(markFreq-1), nil
}

// firstIndexFrom finds, from the newest down a page at a time, the first
// entry of an index chain of the peer's whose block is at least from, and
// the entry before it, nil when there is none.
func firstIndexFrom(ctx context.Context, src Source, u *url.URL, index *api.ChainRecord, from, pageSize uint64) (uint64, *protocol.IndexEntry, error) {
	expand := true
	for end := index.Count; end > 0; {
		begin := uint64(0)
		if end > pageSize {
			begin = end - pageSize
		}
		// The page, in as many answers as the peer takes to serve it: a
		// peer may serve fewer entries than asked for.
		var page []*protocol.IndexEntry
		for at := begin; at < end; {
			count := end - at
			resp, err := src.QueryChainEntries(ctx, u, &api.ChainQuery{
				Name:  index.Name,
				Range: &api.RangeOptions{Start: at, Count: &count, Expand: &expand},
			})
			if err != nil {
				return 0, nil, fmt.Errorf("query %s entries from %d: %w", index.Name, at, err)
			}
			if resp == nil || len(resp.Records) == 0 {
				return 0, nil, fmt.Errorf("the peer served no %s entry at %d", index.Name, at)
			}
			for _, r := range resp.Records {
				if at >= end {
					break
				}
				if r == nil {
					return 0, nil, fmt.Errorf("the peer served entry %d of %s as no record", at, index.Name)
				}
				v, ok := r.Value.(*api.IndexEntryRecord)
				if !ok || v.Value == nil || r.Index != at {
					return 0, nil, fmt.Errorf("the peer served entry %d of %s as no index entry", at, index.Name)
				}
				page = append(page, v.Value)
				at++
			}
		}
		for i := len(page) - 1; i >= 0; i-- {
			if page[i].BlockIndex < from {
				return begin + uint64(i) + 1, page[i], nil
			}
		}
		end = begin
	}
	return 0, nil, nil
}

// streamChainFrom takes the peer's chain from start, a mark point, to its
// head: the state before start is the one the peer serves with entry
// start-1, and the entries from start must replay it to the peer's head.
// The state is written as the mark point the window starts on, with none of
// the hashes of the set it closes: they are not held (merkle Chain.Entry).
func streamChainFrom(ctx context.Context, src Source, pages Store, u *url.URL, c *api.ChainRecord, start, pageSize uint64) (*streamedChain, error) {
	st := new(merkle.State)
	if start > 0 {
		one := uint64(1)
		resp, err := src.QueryChainEntries(ctx, u, &api.ChainQuery{
			Name:  c.Name,
			Range: &api.RangeOptions{Start: start - 1, Count: &one},
		})
		if err != nil {
			return nil, fmt.Errorf("query the state at %d: %w", start-1, err)
		}
		if resp == nil || len(resp.Records) != 1 || resp.Records[0] == nil || resp.Records[0].Index != start-1 {
			return nil, fmt.Errorf("the peer served no entry at %d", start-1)
		}
		st.Count = int64(start)
		for _, h := range resp.Records[0].State {
			if len(h) == 0 {
				h = nil
			}
			st.Pending = append(st.Pending, h)
		}

		page := pages.Begin(true)
		err = func() error {
			pc, err := page.Account(u).ChainByName(c.Name)
			if err != nil {
				return err
			}
			states := pc.Inner().States(start - 1)
			_, err = states.Get()
			switch {
			case err == nil:
				return nil // The node holds the mark point, hashes and all
			case errors.Is(err, errors.NotFound):
				return states.Put(st.Copy())
			default:
				return err
			}
		}()
		if err == nil {
			err = page.Commit()
		}
		if err != nil {
			page.Discard()
			return nil, fmt.Errorf("write the mark point at %d: %w", start-1, err)
		}
	}

	// The window's pages across the peers, then the state built over them
	// in order (#4529, #4532). The signature chain's messages -- every
	// validator's signature on every anchor, three quarters of a pool
	// window's pages -- are left to the backfill: the node needs none of
	// them to join or execute, and serves no anchor without them.
	bodies := carriesMessages(c) && !messagesLeftToBackfill(c)
	msgsFrom := start
	if carriesMessages(c) && !bodies {
		msgsFrom = c.Count
	}
	want := &merkle.State{Count: int64(c.Count), Pending: c.State}
	if err := fetchAndReplay(ctx, src, peersOf(ctx, src), pages, u, c.Name, st, want, start, c.Count, pageSize, bodies); err != nil {
		return nil, err
	}
	if st.Count != want.Count || !bytes.Equal(st.Anchor(), want.Anchor()) {
		return nil, fmt.Errorf("%w: the peer's entries [%d, %d) on the state it served at %d anchor to %x and its head to %x",
			errNotThePeers, start, c.Count, start, st.Anchor(), want.Anchor())
	}
	return &streamedChain{c: c, head: want, open: st.HashList, restore: true, from: start, msgsFrom: msgsFrom}, nil
}
