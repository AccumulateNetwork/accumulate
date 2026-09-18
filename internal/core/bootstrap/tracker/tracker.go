// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package tracker watches the local BPT root chase a moving target — the
// roots the Directory anchored — and flips the node's state machine BOOTING →
// ACTIVE at the first block whose anchored root the local root equals. That
// block is Q of executor.md, "Sync", step 4: the block the node then executes
// from.
//
// The tracker is passive. Callers feed it the anchors they collect and ask it
// to check after every commit; there is no goroutine here.
//
// What it is handed matters. An anchor given to Observe must be one the
// Directory anchored — a StateTreeAnchor out of an anchor executed at the
// Directory — never a root a peer claims for itself and never a root the node
// computed for itself while BOOTING. The tracker is the matcher, not the
// verifier.
//
// Ported from bootstrap-v3 (issue #4293). Changed on this line: the default
// match threshold is 1, because one match against an anchored root is
// decisive; bootstrap-v3 needed ten in a row because it matched against a
// peer's claim about itself, where a single match could be a coincidence.
package tracker

import (
	"context"
	"fmt"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// DefaultMatchThreshold is the number of consecutive matching Check calls
// required before the tracker promotes the machine. One: an anchored root is
// a fact about a block, so the local root equalling it says the state is that
// block's state, and there is nothing a second look adds.
const DefaultMatchThreshold = 1

// Tracker compares the local BPT root against observed anchors and
// flips a nodestate.Machine to StateActive after MatchThreshold
// consecutive Check calls match.
type Tracker struct {
	db      *database.Database
	machine *nodestate.Machine

	// MatchThreshold is the number of consecutive matches required
	// before promoting. Zero means use DefaultMatchThreshold.
	MatchThreshold int

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
	// consecutive is the current consecutive-match streak. Reset
	// on any mismatch. Promotion fires when consecutive ≥
	// MatchThreshold (or DefaultMatchThreshold when zero).
	consecutive int
	// streakAnchor and streakBlock record the anchor/block that
	// most-recently extended the streak. PromoteToActive uses these
	// so SinceBlock reflects the up-to-date observation, not the
	// anchor that started the streak.
	streakAnchor [32]byte
	streakBlock  uint64
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

// Check reads the current local BPT root and updates the
// consecutive-match streak. On reaching MatchThreshold it promotes
// the state machine. Returns (true, nil) on the promoting call;
// (false, nil) if not yet or if the machine was already past
// BOOTING.
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

	threshold := t.MatchThreshold
	if threshold <= 0 {
		threshold = DefaultMatchThreshold
	}

	t.mu.Lock()
	block, ok := t.observed[local]
	if !ok {
		// Mismatch — reset the streak.
		t.consecutive = 0
		t.streakAnchor = [32]byte{}
		t.streakBlock = 0
		t.mu.Unlock()
		return false, nil
	}
	t.consecutive++
	t.streakAnchor = local
	t.streakBlock = block
	streak := t.consecutive
	t.mu.Unlock()

	if streak < threshold {
		return false, nil
	}

	// Streak reached the threshold. PromoteToActive returns false
	// if a concurrent caller already transitioned us past BOOTING.
	// We treat that as not-our-promotion rather than an error —
	// both reach the same destination state.
	if !t.machine.PromoteToActive(local, block) {
		return false, nil
	}
	return true, nil
}

// ConsecutiveMatches reports the current consecutive-match streak
// for diagnostics. Useful for logs that want to show "5/10 toward
// promotion."
func (t *Tracker) ConsecutiveMatches() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.consecutive
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

// Observation is one (block, anchor) pair in the tracker's set.
type Observation struct {
	Block  uint64
	Anchor [32]byte
}

// Snapshot returns a copy of the tracker's observed-anchor set, for
// persistence. Order is unspecified. Safe to call concurrently with
// Observe/Check.
func (t *Tracker) Snapshot() []Observation {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Observation, 0, len(t.observed))
	for anchor, block := range t.observed {
		out = append(out, Observation{Block: block, Anchor: anchor})
	}
	return out
}

// RestoreFrom merges observations into the tracker. Used at startup
// to rehydrate from persisted state. Existing entries take the
// earliest block per anchor (same rule as Observe).
func (t *Tracker) RestoreFrom(obs []Observation) {
	for _, o := range obs {
		t.Observe(o.Block, o.Anchor)
	}
}
