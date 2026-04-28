// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package tracker watches the local BPT root chase a moving target —
// the latest signed major-block anchor observed via gossip — and flips
// the bootstrap state machine BOOTING → ACTIVE the first time the two
// match.
//
// The tracker is deliberately passive: callers (the orchestrator,
// v3-5) feed it observed anchors and ask it to check after every
// commit. There is no background goroutine here; the orchestrator
// drives cadence.
//
// Trust model: anchors handed to Observe are presumed already
// signature-verified. The tracker is the matcher, not the verifier.
// Verification of major-block anchor signatures lives elsewhere — in
// this build queue, the orchestrator pulls signed anchors from the
// peer's anchor pool and trusts the peer's ACTIVE/COMPLETE claim per
// the v3 doc.
package tracker

import (
	"context"
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// Tracker compares the local BPT root against observed anchors and
// flips a nodestate.Machine to StateActive on first match.
type Tracker struct {
	db      *database.Database
	machine *nodestate.Machine

	mu sync.Mutex
	// observed maps anchor → block height it was seen at. A map
	// rather than a single "latest" because (a) the local root will
	// briefly equal an older block's anchor as it catches up, and (b)
	// we want to record the correct sinceBlock when we promote.
	observed map[[32]byte]uint64
	// latestBlock is the highest block we've seen an anchor for —
	// used as a tie-breaker if the local root somehow matches
	// multiple observed anchors in one Check (it shouldn't, but a
	// chain reorg in the source would).
	latestBlock uint64
}

// New constructs a Tracker bound to db and machine.
func New(db *database.Database, machine *nodestate.Machine) (*Tracker, error) {
	if db == nil {
		return nil, fmt.Errorf("tracker.New: db required")
	}
	if machine == nil {
		return nil, fmt.Errorf("tracker.New: machine required")
	}
	return &Tracker{
		db:       db,
		machine:  machine,
		observed: make(map[[32]byte]uint64),
	}, nil
}

// Observe records a verified BPT-root anchor seen at block. Calling
// Observe with a zero anchor is a no-op (avoids accidentally accepting
// an empty header). Observing the same anchor twice keeps the earliest
// block it appeared at — replays of the same anchor across blocks are
// harmless.
func (t *Tracker) Observe(block uint64, anchor [32]byte) {
	if anchor == ([32]byte{}) {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if existing, ok := t.observed[anchor]; ok {
		// Keep the earliest block where this root was first valid.
		if block < existing {
			t.observed[anchor] = block
		}
	} else {
		t.observed[anchor] = block
	}
	if block > t.latestBlock {
		t.latestBlock = block
	}
}

// Check reads the current local BPT root and, if it matches any
// observed anchor, promotes the state machine to StateActive. Returns
// (true, nil) on the promoting call; (false, nil) if no match yet or
// if the machine was already past BOOTING.
func (t *Tracker) Check(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if t.machine.State() != nodestate.StateBooting {
		return false, nil
	}

	batch := t.db.Begin(false)
	defer batch.Discard()
	local, err := batch.GetBptRootHash()
	if err != nil {
		return false, fmt.Errorf("read local BPT root: %w", err)
	}

	t.mu.Lock()
	block, ok := t.observed[local]
	t.mu.Unlock()
	if !ok {
		return false, nil
	}

	// PromoteToActive returns false if a concurrent caller already
	// transitioned us past BOOTING. We treat that as not-our-promotion
	// rather than an error — both reach the same destination state.
	if !t.machine.PromoteToActive(local, block) {
		return false, nil
	}
	return true, nil
}

// LatestObservedBlock reports the highest block any observed anchor
// has been seen at. Useful for logging / progress output.
func (t *Tracker) LatestObservedBlock() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.latestBlock
}

// ObservedCount reports how many distinct anchors are currently held.
// Pure observability helper.
func (t *Tracker) ObservedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.observed)
}
