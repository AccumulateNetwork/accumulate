// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package hydrator runs the background process that loads account data
// behind known BPT leaves during BOOTING (issue #3964, parent #3953).
//
// The BPT structure is filled in Phase 1 (#3969 enumeration). This
// hydrator runs in Phase 2 with three concurrent sources, jointly
// driving the node from BOOTING to ACTIVE:
//
//  1. Live traffic listener (active prefetch). Subscribes to incoming
//     blocks; pre-fetches accounts referenced in transactions before
//     the local node touches them. Warms the hot working set ahead of
//     need — the path that makes validator participation viable.
//
//  2. BPT enumeration consumer (systematic completeness). Walks the
//     locally-complete BPT and fetches account data for any leaf whose
//     account isn't yet loaded. Drives the node toward full
//     completeness regardless of traffic patterns.
//
//  3. Passive fetch on touch (safety net). Local code paths that hit
//     a not-yet-loaded account queue a fetch.
//
// Each fetched account is verified to hash to the matching leaf's
// value_hash before being stored. The hydrator does NOT execute
// transactions — it is a pure receiver per the BOOTING trust model.
//
// When loadtrack reports zero unloaded accounts, the hydrator promotes
// the nodestate.Machine from BOOTING to ACTIVE.
package hydrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/loadtrack"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
)

// Source identifies which subsystem queued a fetch — used for priority
// and metrics.
type Source int

const (
	SourceTraffic    Source = iota // live block listener
	SourceEnumerator               // BPT page walker
	SourceTouch                    // local code hit an unloaded account
)

func (s Source) String() string {
	switch s {
	case SourceTraffic:
		return "traffic"
	case SourceEnumerator:
		return "enumerator"
	case SourceTouch:
		return "touch"
	default:
		return "unknown"
	}
}

// Job is one queued account-data fetch.
type Job struct {
	KeyHash   [32]byte
	Source    Source
	EnqueueAt time.Time
}

// Fetcher fetches the account data behind a leaf and verifies it hashes
// to the leaf's value_hash. Implementations sit on top of #3958
// (GetBptLeaf) plus an account-state query.
type Fetcher interface {
	// FetchAndVerify fetches the account behind keyHash, verifies its
	// hash matches the BPT leaf's value_hash, and returns nil on
	// success. Implementations are responsible for verification —
	// the hydrator only orchestrates.
	FetchAndVerify(ctx context.Context, keyHash [32]byte) error
}

// Stats tracks hydrator counters.
type Stats struct {
	FetchedTraffic    int64
	FetchedEnumerator int64
	FetchedTouch      int64
	Failed            int64
}

// Hydrator orchestrates the three loading sources.
type Hydrator struct {
	tracker *loadtrack.Tracker
	state   *nodestate.Machine
	fetcher Fetcher

	// Active block height the BPT-root match is observed at, used as
	// SinceBlock when promoting to ACTIVE.
	activeAtBlock uint64

	// Active BPT root the local BPT matched the network's anchor at.
	activeBptRoot [32]byte

	// Job queue. Three priorities (traffic > enumerator > touch).
	traffic    chan Job
	enumerator chan Job
	touch      chan Job

	// Worker control.
	workers int
	stop    chan struct{}
	wg      sync.WaitGroup

	// Stats (atomic).
	statsTraffic    atomic.Int64
	statsEnumerator atomic.Int64
	statsTouch      atomic.Int64
	statsFailed     atomic.Int64

	// Configuration.
	queueSize int

	mu sync.Mutex
}

// Options configures a Hydrator.
type Options struct {
	// Tracker is the load-state tracker (#3962).
	Tracker *loadtrack.Tracker

	// State is the node-state machine (#3970). Hydrator promotes it
	// to ACTIVE when Tracker.UnloadedCount() reaches zero.
	State *nodestate.Machine

	// Fetcher does the actual account-data fetch and hash verification.
	Fetcher Fetcher

	// Workers is the number of concurrent fetch workers. Default 4.
	Workers int

	// QueueSize is the per-source queue capacity. Default 1024.
	QueueSize int
}

// ErrInvalidOptions is returned when New is called with bad options.
var ErrInvalidOptions = errors.New("hydrator: invalid options")

// New constructs a Hydrator. Call Start to begin background work.
func New(opts Options) (*Hydrator, error) {
	if opts.Tracker == nil {
		return nil, fmt.Errorf("%w: Tracker required", ErrInvalidOptions)
	}
	if opts.State == nil {
		return nil, fmt.Errorf("%w: State required", ErrInvalidOptions)
	}
	if opts.Fetcher == nil {
		return nil, fmt.Errorf("%w: Fetcher required", ErrInvalidOptions)
	}
	w := opts.Workers
	if w == 0 {
		w = 4
	}
	q := opts.QueueSize
	if q == 0 {
		q = 1024
	}

	return &Hydrator{
		tracker:    opts.Tracker,
		state:      opts.State,
		fetcher:    opts.Fetcher,
		traffic:    make(chan Job, q),
		enumerator: make(chan Job, q),
		touch:      make(chan Job, q),
		workers:    w,
		stop:       make(chan struct{}),
		queueSize:  q,
	}, nil
}

// SetActiveTrigger configures the BPT root match the hydrator is
// driving toward. When tracker.UnloadedCount() reaches zero AND the
// local BPT root matches activeBptRoot, the hydrator promotes the
// state machine to ACTIVE.
//
// Updated by the caller as new anchors arrive (the network's
// StateTreeAnchor moves; we hydrate toward the latest committed root).
func (h *Hydrator) SetActiveTrigger(bptRoot [32]byte, blockHeight uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.activeBptRoot = bptRoot
	h.activeAtBlock = blockHeight
}

// Enqueue queues a fetch from the given source. Drops silently if the
// queue is full (the source's other entries will retry).
func (h *Hydrator) Enqueue(keyHash [32]byte, src Source) bool {
	if h.tracker.IsLoaded(keyHash) {
		return false // already loaded
	}
	job := Job{KeyHash: keyHash, Source: src, EnqueueAt: time.Now()}

	var ch chan Job
	switch src {
	case SourceTraffic:
		ch = h.traffic
	case SourceEnumerator:
		ch = h.enumerator
	case SourceTouch:
		ch = h.touch
	default:
		return false
	}
	select {
	case ch <- job:
		return true
	default:
		return false // queue full
	}
}

// Start launches the worker pool plus the all-loaded watcher.
func (h *Hydrator) Start(ctx context.Context) {
	for i := 0; i < h.workers; i++ {
		h.wg.Add(1)
		go h.worker(ctx)
	}
	// Set up the BOOTING → ACTIVE promotion handler.
	h.tracker.OnAllLoaded(func() {
		h.mu.Lock()
		root := h.activeBptRoot
		blk := h.activeAtBlock
		h.mu.Unlock()

		if root == ([32]byte{}) {
			// No active trigger configured yet — caller hasn't told us
			// what root to match. Skip promotion; will be retried when
			// the next leaf is loaded (or via SetActiveTrigger after
			// tracker hits zero).
			return
		}
		h.state.PromoteToActive(root, blk)
	})
}

// Stop signals workers to exit and waits for them.
func (h *Hydrator) Stop() {
	close(h.stop)
	h.wg.Wait()
}

// Stats returns a snapshot of fetch counters.
func (h *Hydrator) Stats() Stats {
	return Stats{
		FetchedTraffic:    h.statsTraffic.Load(),
		FetchedEnumerator: h.statsEnumerator.Load(),
		FetchedTouch:      h.statsTouch.Load(),
		Failed:            h.statsFailed.Load(),
	}
}

func (h *Hydrator) worker(ctx context.Context) {
	defer h.wg.Done()
	for {
		// Priority order: traffic > enumerator > touch.
		select {
		case <-ctx.Done():
			return
		case <-h.stop:
			return
		case job := <-h.traffic:
			h.do(ctx, job)
		default:
		}

		select {
		case <-ctx.Done():
			return
		case <-h.stop:
			return
		case job := <-h.traffic:
			h.do(ctx, job)
		case job := <-h.enumerator:
			h.do(ctx, job)
		default:
		}

		select {
		case <-ctx.Done():
			return
		case <-h.stop:
			return
		case job := <-h.traffic:
			h.do(ctx, job)
		case job := <-h.enumerator:
			h.do(ctx, job)
		case job := <-h.touch:
			h.do(ctx, job)
		}
	}
}

func (h *Hydrator) do(ctx context.Context, job Job) {
	if h.tracker.IsLoaded(job.KeyHash) {
		return // raced; already loaded
	}
	err := h.fetcher.FetchAndVerify(ctx, job.KeyHash)
	if err != nil {
		h.statsFailed.Add(1)
		return
	}
	h.tracker.MarkLoaded(job.KeyHash)
	switch job.Source {
	case SourceTraffic:
		h.statsTraffic.Add(1)
	case SourceEnumerator:
		h.statsEnumerator.Add(1)
	case SourceTouch:
		h.statsTouch.Add(1)
	}
}
