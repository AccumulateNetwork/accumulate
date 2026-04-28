// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package gossip is the bootstrap-v3 launcher's live-events
// subscriber. It subscribes to a peer's block-event stream and, for
// every account touched in each new block, fetches the new account
// state via pull.Account (ModeStateOnly) and writes it to the local
// DB. The local BPT leaf for that account is implicitly updated on
// commit (via the database observer), and the launcher's tracker
// (v3-4) compares the resulting local BPT root to incoming signed
// anchors to decide ACTIVE.
//
// "Gossip" is loose terminology — what we actually use is
// EventService.Subscribe over the v3 API. The launcher dials a
// peer's v3 endpoint and holds a stream open. It does not register
// services or participate in P2P routing; it consumes only.
package gossip

import (
	"context"
	"fmt"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Options configures the subscriber loop.
type Options struct {
	// Partition restricts the subscription to this partition's
	// blocks. "Directory" for DN, "<bvn>" for a BVN.
	Partition string

	// PageSize for paginated state-pull calls during ingestion.
	// Default 256.
	PageSize uint64

	// OnBlock, if non-nil, is invoked after each block has been
	// processed. Lets the caller observe progress for the tracker
	// or logging.
	OnBlock func(ev *api.BlockEvent, touched []*url.URL)
}

// Run subscribes to the partition's block stream via eventSvc, then
// processes incoming events until the context is canceled or the
// stream closes. Returns nil on clean shutdown via context cancel,
// non-nil on subscription error.
func Run(
	ctx context.Context,
	src pull.Source,
	eventSvc api.EventService,
	db *database.Database,
	opts Options,
) error {
	ch, err := eventSvc.Subscribe(ctx, api.SubscribeOptions{
		Partition: opts.Partition,
	})
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	return RunChannel(ctx, src, ch, db, opts)
}

// RunChannel is the test-friendly variant: caller supplies the event
// channel directly. Lets tests drive synthetic events without
// running a real EventService.
func RunChannel(
	ctx context.Context,
	src pull.Source,
	evCh <-chan api.Event,
	db *database.Database,
	opts Options,
) error {
	if src == nil {
		return fmt.Errorf("gossip.Run: src required")
	}
	if db == nil {
		return fmt.Errorf("gossip.Run: db required")
	}
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = 256
	}

	for {
		select {
		case <-ctx.Done():
			return nil // clean shutdown via cancel
		case ev, ok := <-evCh:
			if !ok {
				return nil // channel closed by EventService
			}
			if err := handleEvent(ctx, src, db, ev, pageSize, opts.OnBlock); err != nil {
				return fmt.Errorf("handle event: %w", err)
			}
		}
	}
}

func handleEvent(
	ctx context.Context,
	src pull.Source,
	db *database.Database,
	ev api.Event,
	pageSize uint64,
	onBlock func(*api.BlockEvent, []*url.URL),
) error {
	blockEv, ok := ev.(*api.BlockEvent)
	if !ok {
		// ErrorEvent / GlobalsEvent / etc. — ignore for now. A
		// production launcher should at least log error events.
		return nil
	}

	touched := dedupAccounts(blockEv.Entries)
	if len(touched) == 0 {
		if onBlock != nil {
			onBlock(blockEv, nil)
		}
		return nil
	}

	batch := db.Begin(true)
	for _, u := range touched {
		if err := pull.Account(ctx, src, batch, u, pull.Options{
			Mode:     pull.ModeStateOnly,
			PageSize: pageSize,
		}); err != nil {
			batch.Discard()
			return fmt.Errorf("pull touched account %s: %w", u, err)
		}
	}
	if err := batch.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if onBlock != nil {
		onBlock(blockEv, touched)
	}
	return nil
}

// dedupAccounts collapses the entries' Account URLs into a unique
// slice. A single block can update many accounts; some accounts can
// have multiple entries per block (different chains).
func dedupAccounts(entries []*api.ChainEntryRecord[api.Record]) []*url.URL {
	if len(entries) == 0 {
		return nil
	}
	seen := make(map[string]*url.URL, len(entries))
	for _, e := range entries {
		if e == nil || e.Account == nil {
			continue
		}
		key := e.Account.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = e.Account
	}
	out := make([]*url.URL, 0, len(seen))
	for _, u := range seen {
		out = append(out, u)
	}
	return out
}
