// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package tracker watches the local BPT root chase a moving target — the
// roots the Directory anchored for one partition — and says which anchored
// block, if any, the local root now equals. That block is Q of executor.md,
// "Sync", step 4: the block the node then hands off at.
//
// It does not promote the node. A match says the state is block Q's state; it
// does not say the node is executing from it, and a node is ACTIVE only while
// it is executing in agreement (step 6). The join promotes when the handoff
// succeeds (#4385: run 20260924T074702Z, acc-bvn1-val3 read ACTIVE from its
// first match, never handed off, and served stale anchors to the next
// joiner, #4413).
//
// The tracker is passive. Callers feed it the anchors they collect and ask it
// to check after every commit; there is no goroutine here.
//
// What it is handed matters. An anchor given to Observe must be one the
// Directory anchored — a StateTreeAnchor out of an anchor executed at the
// Directory — never a root a peer claims for itself and never a root the node
// computed for itself while BOOTING. **And the local root must be derived from
// state this node holds and has verified**: a leaf taken from a peer's word
// would make the local root the peer's root and the match meaningless, which
// is why enumeration writes nothing (package enumerate). The tracker is the
// matcher, not the verifier.
//
// A tracker belongs to one partition. Block numbers collide across partitions
// — with a one-second cadence the Directory and a BVN are at the same number
// at the same second — so a node serving two partitions runs two trackers and
// each ignores the other's anchors (#4205).
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

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// DefaultMatchThreshold is the number of consecutive matching Check calls
// required before the tracker reports a match. One: an anchored root is
// a fact about a block, so the local root equalling it says the state is that
// block's state, and there is nothing a second look adds.
const DefaultMatchThreshold = 1

// DefaultMaxObserved is how many anchors a tracker keeps. A join runs for as
// long as it takes to catch up, and an unbounded set of observations is an
// unbounded heap on a node that never converges. The oldest are dropped
// first: the local root cannot come to equal a root from thousands of blocks
// back, because the state the node is assembling is the current state.
const DefaultMaxObserved = 4096

// Tracker compares the local BPT root against the anchors observed for its
// partition, and reports a match once MatchThreshold consecutive Check calls
// have matched.
type Tracker struct {
	db        *database.Database
	partition *url.URL

	// MatchThreshold is the number of consecutive matches required
	// before a match is reported. Zero means use DefaultMatchThreshold.
	MatchThreshold int

	// MaxObserved bounds the observed set. Zero means DefaultMaxObserved.
	MaxObserved int

	mu sync.Mutex
	// observed maps anchor → the block of partition it was seen at. A map
	// rather than a single "latest" because the local root will briefly equal
	// an older block's anchor as it catches up, and because a match reports
	// the block the matching root was anchored for.
	observed map[[32]byte]uint64
	// order is the anchors in the order they were first observed, for
	// eviction. Anchors arrive in block order, so the front is the oldest.
	order [][32]byte
	// latestBlock is the highest block an anchor has been seen at.
	latestBlock uint64
	// consecutive is the current consecutive-match streak, reset on any
	// mismatch. A match is reported while it is at the threshold or above.
	consecutive int
}

// New constructs a Tracker over db's root for one partition's anchors.
func New(db *database.Database, partition *url.URL) (*Tracker, error) {
	if db == nil {
		return nil, fmt.Errorf("tracker.New: db required")
	}
	if partition == nil {
		return nil, fmt.Errorf("tracker.New: a tracker must name its partition")
	}
	return &Tracker{
		db:        db,
		partition: partition,
		observed:  make(map[[32]byte]uint64),
	}, nil
}

// Partition reports the partition whose anchors this tracker matches.
func (t *Tracker) Partition() *url.URL { return t.partition }

// Observe records an anchor the Directory executed for partition's block.
//
// An anchor for another partition is ignored: the caller feeds it every anchor
// the Directory executes, and a block number means nothing without the
// partition it belongs to. A zero anchor is ignored too — it is what an empty
// header looks like. Observing the same anchor twice keeps the earliest block
// it appeared at, because that block is the first at which the state behind
// the root was the partition's state.
func (t *Tracker) Observe(partition *url.URL, block uint64, anchor [32]byte) {
	if anchor == ([32]byte{}) || partition == nil || !partition.Equal(t.partition) {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if existing, ok := t.observed[anchor]; ok {
		if block < existing {
			t.observed[anchor] = block
		}
	} else {
		t.observed[anchor] = block
		t.order = append(t.order, anchor)
	}
	if block > t.latestBlock {
		t.latestBlock = block
	}

	max := t.MaxObserved
	if max <= 0 {
		max = DefaultMaxObserved
	}
	for len(t.order) > max {
		delete(t.observed, t.order[0])
		t.order = t.order[1:]
	}
}

// A Match is the anchored block the local root equals, and its root.
type Match struct {
	Block  uint64
	Anchor [32]byte
}

// Check reads the current local BPT root and updates the consecutive-match
// streak. It reports the match on every call on which the streak is at
// MatchThreshold or above, and nothing otherwise. It changes nothing but the
// streak: what a match means for the node is the join's to decide.
func (t *Tracker) Check(ctx context.Context) (Match, bool, error) {
	if err := ctx.Err(); err != nil {
		return Match{}, false, err
	}

	batch := t.db.Begin(false)
	defer batch.Discard()
	local, err := batch.GetBptRootHash()
	if err != nil {
		return Match{}, false, fmt.Errorf("read local BPT root: %w", err)
	}

	threshold := t.MatchThreshold
	if threshold <= 0 {
		threshold = DefaultMatchThreshold
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	block, ok := t.observed[local]
	if !ok {
		t.consecutive = 0
		return Match{}, false, nil
	}
	t.consecutive++
	if t.consecutive < threshold {
		return Match{}, false, nil
	}
	return Match{Block: block, Anchor: local}, true, nil
}

// ResetStreak starts the consecutive-match streak again. The join calls it
// when it demotes the node (#4385): a streak counted before the node stopped
// agreeing says nothing about the state it syncs to next.
func (t *Tracker) ResetStreak() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.consecutive = 0
}

// ConsecutiveMatches reports the current consecutive-match streak
// for diagnostics.
func (t *Tracker) ConsecutiveMatches() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.consecutive
}

// LatestObservedBlock reports the highest block of this partition any observed
// anchor has been seen at.
func (t *Tracker) LatestObservedBlock() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.latestBlock
}

// ObservedCount reports how many distinct anchors are currently held.
func (t *Tracker) ObservedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.observed)
}

// Observation is one (block, anchor) pair in the tracker's set. The partition
// is the tracker's.
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

// RestoreFrom merges observations into the tracker. Used at startup to
// rehydrate from persisted state; the observations are this tracker's
// partition's, which is what Snapshot returned. Existing entries take the
// earliest block per anchor (same rule as Observe).
func (t *Tracker) RestoreFrom(obs []Observation) {
	for _, o := range obs {
		t.Observe(t.partition, o.Block, o.Anchor)
	}
}
