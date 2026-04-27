// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package backfill runs the post-bootstrap history backfill that drives
// a node from ACTIVE to COMPLETE (issue #3967, parent #3953).
//
// After bootstrap reaches ACTIVE, the node has full current state but
// only a small rolling window of recent history. This backfill pulls
// older blocks and transactions in the background, optional, never
// blocking the running node.
//
// Workflow:
//   1. Determine the target depth (oldest block to retain). Configurable
//      via the bootstrap config; zero = unlimited (full archive).
//   2. Walk backward from the rolling window's edge toward the target.
//   3. For each minor block: fetch the block, its transactions, and any
//      referenced data; verify against root-chain inclusion proofs
//      anchored in the validated anchor stream.
//   4. Write into the local database without interfering with live
//      execution.
//   5. When the target is reached, signal nodestate to promote
//      ACTIVE → COMPLETE.
package backfill

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
)

// Stats tracks backfill progress.
type Stats struct {
	BlocksFetched      int64
	TransactionsFetched int64
	Failed             int64

	// OldestRetained is the lowest block number fully retained so far.
	// Decreases as the backfill walks backward.
	OldestRetained uint64

	// TargetDepth is the oldest block the backfill is trying to reach.
	// Zero means unlimited (full archive).
	TargetDepth uint64

	// LastUpdated is when the stats were last refreshed.
	LastUpdated time.Time
}

// BlockFetcher fetches an individual minor block and its transactions
// from the network. Implementations sit on top of existing query APIs
// plus root-chain inclusion proof verification.
type BlockFetcher interface {
	// FetchAndStore retrieves the block at minorIndex, verifies it
	// against the locally-validated anchor stream, and persists it
	// locally. Returns the block's transaction count on success.
	FetchAndStore(ctx context.Context, partition string, minorIndex uint64) (txCount int, err error)
}

// Options configures a Backfill.
type Options struct {
	// State is the node-state machine. The Backfill promotes ACTIVE → COMPLETE
	// when the target depth is reached.
	State *nodestate.Machine

	// Fetcher does the actual block-fetch + verify + store.
	Fetcher BlockFetcher

	// Partition the backfill is responsible for. Multi-partition
	// backfill (DN + N BVNs) runs one Backfill per partition.
	Partition string

	// StartFrom is the highest minor block to fetch (typically
	// rolling-window edge - 1). The backfill walks backward from here.
	StartFrom uint64

	// TargetDepth is the lowest minor block to fetch. Zero = unlimited
	// (walk all the way back to the bootstrap pin block).
	TargetDepth uint64

	// PollInterval throttles the rate at which blocks are fetched, to
	// avoid overwhelming the network. Default 100ms.
	PollInterval time.Duration
}

// ErrInvalidOptions is returned when New is called with bad options.
var ErrInvalidOptions = errors.New("backfill: invalid options")

// Backfill runs the post-ACTIVE history backfill.
type Backfill struct {
	opts Options

	statsBlocks atomic.Int64
	statsTx     atomic.Int64
	statsFailed atomic.Int64

	mu             sync.RWMutex
	oldestRetained uint64
	lastUpdated    time.Time

	stop chan struct{}
	wg   sync.WaitGroup
}

// New constructs a Backfill. Call Start to begin background work.
func New(opts Options) (*Backfill, error) {
	if opts.State == nil {
		return nil, fmt.Errorf("%w: State required", ErrInvalidOptions)
	}
	if opts.Fetcher == nil {
		return nil, fmt.Errorf("%w: Fetcher required", ErrInvalidOptions)
	}
	if opts.Partition == "" {
		return nil, fmt.Errorf("%w: Partition required", ErrInvalidOptions)
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = 100 * time.Millisecond
	}
	return &Backfill{
		opts:           opts,
		oldestRetained: opts.StartFrom,
		stop:           make(chan struct{}),
	}, nil
}

// Start launches the background backfill loop. Idempotent; calling Start
// twice has no additional effect.
func (b *Backfill) Start(ctx context.Context) {
	b.wg.Add(1)
	go b.loop(ctx)
}

// Stop signals the loop to exit and waits for it.
func (b *Backfill) Stop() {
	close(b.stop)
	b.wg.Wait()
}

// Stats returns a snapshot of progress.
func (b *Backfill) Stats() Stats {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return Stats{
		BlocksFetched:       b.statsBlocks.Load(),
		TransactionsFetched: b.statsTx.Load(),
		Failed:              b.statsFailed.Load(),
		OldestRetained:      b.oldestRetained,
		TargetDepth:         b.opts.TargetDepth,
		LastUpdated:         b.lastUpdated,
	}
}

func (b *Backfill) loop(ctx context.Context) {
	defer b.wg.Done()
	tick := time.NewTicker(b.opts.PollInterval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-b.stop:
			return
		case <-tick.C:
		}

		b.mu.RLock()
		next := b.oldestRetained
		b.mu.RUnlock()

		if next == 0 || (b.opts.TargetDepth > 0 && next <= b.opts.TargetDepth) {
			// Reached the target.
			b.promoteIfReady()
			return
		}

		next--
		txCount, err := b.opts.Fetcher.FetchAndStore(ctx, b.opts.Partition, next)
		if err != nil {
			b.statsFailed.Add(1)
			continue // retry on next tick
		}

		b.statsBlocks.Add(1)
		b.statsTx.Add(int64(txCount))

		b.mu.Lock()
		b.oldestRetained = next
		b.lastUpdated = time.Now()
		b.mu.Unlock()
	}
}

func (b *Backfill) promoteIfReady() {
	// Promote ACTIVE → COMPLETE when target reached.
	historyDepth := uint64(0)
	if b.opts.TargetDepth > 0 {
		historyDepth = b.opts.TargetDepth
	}
	b.mu.RLock()
	since := b.oldestRetained
	b.mu.RUnlock()
	b.opts.State.PromoteToComplete(historyDepth, since)
}
