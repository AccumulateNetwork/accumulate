// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package orchestrator wires the bootstrap-v3 phases together:
//
//   - Spine pull (DN only): pulls the four DN-side spine accounts in
//     ModeFullSpine — anchor pool, ledger, operators, operators/1.
//     These are the cryptographic spine the launcher needs full chain
//     history for.
//
//   - Enumerate: paginates the partition's BPT and inserts every leaf
//     locally, reconstructing the full state map up to the source's
//     current root.
//
//   - Steady-state: subscribes to the partition's BlockEvents via
//     gossip and applies on-demand state pulls for each touched
//     account, keeping the local DB in sync. Concurrently polls an
//     AnchorSource for signed major-block anchors and feeds them to a
//     Tracker that flips nodestate.Machine BOOTING → ACTIVE on first
//     local-root match.
//
// The orchestrator is sequential by phase: spine → enumerate →
// steady-state. The "concurrent enumeration+gossip" idea from the
// design notes is an optimization deferred to a future revision —
// keeping enumeration ahead of live ingestion is fine for an initial
// sync, since the tracker will catch up once steady-state begins.
package orchestrator

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/enumerate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/tracker"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// AnchorSource hands the orchestrator verified major-block anchors.
// Production wires this to a peer's anchor pool query; tests fake it.
// Returning a zero anchor with no error means "no anchor yet,
// retry" — the orchestrator does not treat this as failure.
type AnchorSource interface {
	LatestAnchor(ctx context.Context, partition string) (block uint64, anchor [32]byte, err error)
}

// Source is the combined surface the orchestrator needs from the
// network. Production satisfies all three with a single Querier2 +
// EventService client; tests can build a single fake.
type Source interface {
	pull.Source
	enumerate.Source
	AnchorSource
}

// Options configures Run.
type Options struct {
	// Partition is the partition this orchestrator is syncing —
	// "Directory" for DN, "<bvn-name>" for a BVN.
	Partition string

	// PartitionURL is the scope passed to BPT enumeration. Production
	// uses protocol.PartitionUrl(Partition); tests can override.
	PartitionURL *url.URL

	// IsDirectory toggles spine-pull. True for DN (full chain entries
	// pulled for the four spine accounts), false for BVN (skip spine,
	// rely on enumeration + gossip alone).
	IsDirectory bool

	// PageSize for paginated state-pull and BPT enumeration calls.
	// Default 256.
	PageSize uint64

	// AnchorPollInterval is how often the steady-state loop queries
	// AnchorSource. Default 5s. Tests override to a short tick.
	AnchorPollInterval time.Duration

	// OnPhase, if non-nil, is invoked at phase boundaries with a short
	// human-readable status. Useful for CLI progress output.
	OnPhase func(phase string, msg string)
}

// Run executes the bootstrap-v3 phases. Returns when the tracker
// promotes the machine to StateActive, the event channel closes, or
// the context is canceled.
//
// On clean ACTIVE, returns nil. On context cancel, returns nil. On
// other errors (failed spine pull, failed enumeration page, etc.),
// returns the error and leaves the machine in StateBooting — the
// caller can retry from scratch (the local DB has partial state and
// would be discarded by a real launcher).
func Run(
	ctx context.Context,
	src Source,
	eventCh <-chan api.Event,
	db *database.Database,
	machine *nodestate.Machine,
	opts Options,
) error {
	if src == nil {
		return fmt.Errorf("orchestrator.Run: src required")
	}
	if db == nil {
		return fmt.Errorf("orchestrator.Run: db required")
	}
	if machine == nil {
		return fmt.Errorf("orchestrator.Run: machine required")
	}
	if opts.PartitionURL == nil {
		return fmt.Errorf("orchestrator.Run: PartitionURL required")
	}
	if opts.Partition == "" {
		return fmt.Errorf("orchestrator.Run: Partition required")
	}
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = 256
	}
	pollEvery := opts.AnchorPollInterval
	if pollEvery == 0 {
		pollEvery = 5 * time.Second
	}

	tr, err := tracker.New(db, machine)
	if err != nil {
		return fmt.Errorf("tracker: %w", err)
	}

	phase := func(name, msg string) {
		if opts.OnPhase != nil {
			opts.OnPhase(name, msg)
		}
	}

	// Phase 1: spine pull (DN only).
	if opts.IsDirectory {
		phase("spine", "pulling DN spine accounts in full")
		if err := pullSpine(ctx, src, db, pageSize); err != nil {
			return fmt.Errorf("spine: %w", err)
		}
	}

	// Phase 2: enumerate the partition BPT.
	phase("enumerate", "scanning partition BPT")
	if err := runEnumerate(ctx, src, db, opts.PartitionURL, pageSize, opts.OnPhase); err != nil {
		return fmt.Errorf("enumerate: %w", err)
	}

	// Phase 3: steady-state — gossip ingestion + anchor polling.
	// Exits when tracker flips machine to ACTIVE.
	phase("steady", "applying gossip and watching anchors")
	return runSteady(ctx, src, eventCh, db, tr, opts.Partition, pageSize, pollEvery, opts.OnPhase)
}

// pullSpine pulls the four DN-side spine accounts in ModeFullSpine
// into a single batch and commits it. UpdateBPT is called before
// Commit so the observer's per-account hashes land as BPT leaves —
// without this, account writes never reach the BPT and the tracker's
// local-root check never matches.
func pullSpine(ctx context.Context, src pull.Source, db *database.Database, pageSize uint64) error {
	batch := db.Begin(true)
	defer batch.Discard()
	for _, u := range pull.DnSpineAccounts() {
		if err := pull.Account(ctx, src, batch, u, pull.Options{
			Mode:     pull.ModeFullSpine,
			PageSize: pageSize,
		}); err != nil {
			return fmt.Errorf("pull spine %s: %w", u, err)
		}
	}
	if err := batch.UpdateBPT(); err != nil {
		return fmt.Errorf("update BPT: %w", err)
	}
	if err := batch.Commit(); err != nil {
		return fmt.Errorf("commit spine: %w", err)
	}
	return nil
}

// runEnumerate runs enumerate.Run inside a single commit. The local
// BPT after this returns reflects the source's BPT state at the end
// of the scan (modulo the source advancing during enumeration).
func runEnumerate(
	ctx context.Context,
	src enumerate.Source,
	db *database.Database,
	scope *url.URL,
	pageSize uint64,
	onPhase func(phase, msg string),
) error {
	batch := db.Begin(true)
	defer batch.Discard()
	res, err := enumerate.Run(ctx, src, scope, batch, enumerate.Options{
		PageSize: pageSize,
	})
	if err != nil {
		return err
	}
	if err := batch.Commit(); err != nil {
		return fmt.Errorf("commit enumerate: %w", err)
	}
	if onPhase != nil {
		onPhase("enumerate", fmt.Sprintf("scanned %d leaves across %d pages",
			res.LeavesInserted, res.PagesPulled))
	}
	return nil
}

// runSteady runs the gossip+anchor loop until tracker promotes,
// eventCh closes, or ctx is canceled.
func runSteady(
	ctx context.Context,
	src Source,
	eventCh <-chan api.Event,
	db *database.Database,
	tr *tracker.Tracker,
	partition string,
	pageSize uint64,
	pollEvery time.Duration,
	onPhase func(phase, msg string),
) error {
	// Anchor polling: select-driven, not a separate goroutine. Keeps
	// concurrency model trivial. We poll once at start for fast
	// promotion when the BPT is already current after enumerate.
	if err := pollAnchor(ctx, src, tr, partition); err != nil {
		// Anchor poll failure during steady-state isn't fatal — the
		// peer might be momentarily unavailable. Log via onPhase if
		// configured and continue.
		if onPhase != nil {
			onPhase("steady", fmt.Sprintf("initial anchor poll: %v", err))
		}
	}
	if promoted, err := tr.Check(ctx); err != nil {
		return fmt.Errorf("initial check: %w", err)
	} else if promoted {
		if onPhase != nil {
			onPhase("active", fmt.Sprintf("matched at block %d", tr.LatestObservedBlock()))
		}
		return nil
	}

	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := pollAnchor(ctx, src, tr, partition); err != nil && onPhase != nil {
				onPhase("steady", fmt.Sprintf("anchor poll: %v", err))
			}
			if promoted, err := tr.Check(ctx); err != nil {
				return fmt.Errorf("check: %w", err)
			} else if promoted {
				if onPhase != nil {
					onPhase("active", fmt.Sprintf("matched at block %d", tr.LatestObservedBlock()))
				}
				return nil
			}
		case ev, ok := <-eventCh:
			if !ok {
				// Stream closed before we promoted. Treat as
				// non-error so the caller can retry — same shape as
				// gossip.RunChannel.
				return nil
			}
			if err := applyEvent(ctx, src, db, ev, pageSize); err != nil {
				return fmt.Errorf("apply event: %w", err)
			}
			if promoted, err := tr.Check(ctx); err != nil {
				return fmt.Errorf("check: %w", err)
			} else if promoted {
				if onPhase != nil {
					onPhase("active", fmt.Sprintf("matched at block %d", tr.LatestObservedBlock()))
				}
				return nil
			}
		}
	}
}

// pollAnchor queries src for the latest signed anchor and records it
// with the tracker. Zero anchors are silently skipped (the source's
// signal that no fresh anchor is available yet).
func pollAnchor(ctx context.Context, src AnchorSource, tr *tracker.Tracker, partition string) error {
	block, anchor, err := src.LatestAnchor(ctx, partition)
	if err != nil {
		return err
	}
	tr.Observe(block, anchor)
	return nil
}

// applyEvent mirrors the gossip package's per-event handler: filter
// to BlockEvent, dedup touched accounts, ModeStateOnly pull each, one
// commit per event. We don't call gossip.RunChannel directly because
// the orchestrator interleaves event processing with anchor polling
// and tracker checks in the same select.
func applyEvent(
	ctx context.Context,
	src pull.Source,
	db *database.Database,
	ev api.Event,
	pageSize uint64,
) error {
	blockEv, ok := ev.(*api.BlockEvent)
	if !ok {
		return nil
	}
	touched := dedupAccounts(blockEv.Entries)
	if len(touched) == 0 {
		return nil
	}
	batch := db.Begin(true)
	for _, u := range touched {
		if err := pull.Account(ctx, src, batch, u, pull.Options{
			Mode:     pull.ModeStateOnly,
			PageSize: pageSize,
		}); err != nil {
			batch.Discard()
			return fmt.Errorf("pull %s: %w", u, err)
		}
	}
	if err := batch.UpdateBPT(); err != nil {
		batch.Discard()
		return fmt.Errorf("update BPT: %w", err)
	}
	if err := batch.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// dedupAccounts collapses chain entries' Account URLs into a unique
// slice. Same logic as gossip.dedupAccounts; lifted here to avoid
// importing the gossip package solely for one helper.
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
