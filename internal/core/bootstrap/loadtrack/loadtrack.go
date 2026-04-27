// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package loadtrack tracks which accounts have data loaded vs. not loaded
// during the BOOTING phase of bootstrap (issue #3962, parent #3953).
//
// The BPT structure is filled completely in Phase 1 (every (key_hash,
// value_hash) leaf inserted via #3969 enumeration). What remains "sparse"
// is the account *data* behind each leaf — collected progressively until
// every account is loaded (BOOTING → ACTIVE).
//
// The Tracker exposes:
//   - IsLoaded(keyHash): is the account behind this leaf available locally?
//   - MarkLoaded(keyHash): record that an account has been hydrated.
//   - UnloadedCount(): how many leaves still lack their account data.
//   - OnAllLoaded(callback): fire once when the last unloaded account is
//     loaded — the BOOTING → ACTIVE transition.
//
// The set of all known leaves comes from the BPT itself. The tracker
// exposes per-key load state but does not own the BPT; it queries it via
// the supplied LeafSource.
package loadtrack

import (
	"sync"
)

// LeafSource lets the Tracker enumerate the set of all (key_hash) it
// expects to eventually have data for. In practice this is backed by the
// local BPT once Phase 1 (#3969) has filled it.
type LeafSource interface {
	// AllKeyHashes calls fn for every leaf currently in the BPT structure.
	// Called once at Init() and again whenever the BPT structure mutates
	// (live updates from anchors during BOOTING).
	AllKeyHashes(fn func(keyHash [32]byte) error) error
}

// Tracker tracks load state for accounts behind BPT leaves.
type Tracker struct {
	mu        sync.RWMutex
	src       LeafSource
	known     map[[32]byte]bool // keyHash -> loaded?
	unloaded  int
	allLoaded []func()
	allFired  bool
}

// New constructs a Tracker; you must call Init before use to populate
// the known leaves from src.
func New(src LeafSource) *Tracker {
	return &Tracker{
		src:   src,
		known: make(map[[32]byte]bool),
	}
}

// Init populates the tracker with every key hash in the LeafSource. Each
// is initially marked unloaded.
func (t *Tracker) Init() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.known = make(map[[32]byte]bool)
	t.unloaded = 0
	t.allFired = false

	return t.src.AllKeyHashes(func(kh [32]byte) error {
		if _, exists := t.known[kh]; !exists {
			t.known[kh] = false
			t.unloaded++
		}
		return nil
	})
}

// IsLoaded reports whether the account behind keyHash is loaded locally.
func (t *Tracker) IsLoaded(keyHash [32]byte) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	loaded, known := t.known[keyHash]
	return known && loaded
}

// IsKnown reports whether keyHash is a known leaf (regardless of load
// state). Returns false if the leaf was added to the BPT after the last
// Init / SyncFromSource call.
func (t *Tracker) IsKnown(keyHash [32]byte) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, known := t.known[keyHash]
	return known
}

// MarkLoaded records that the account behind keyHash has been hydrated.
// Idempotent: marking an already-loaded entry is a no-op.
func (t *Tracker) MarkLoaded(keyHash [32]byte) {
	t.mu.Lock()
	loaded, known := t.known[keyHash]
	if !known {
		// Late insertion: the leaf wasn't known at Init time. Add it as
		// already-loaded.
		t.known[keyHash] = true
		t.mu.Unlock()
		return
	}
	if loaded {
		t.mu.Unlock()
		return
	}
	t.known[keyHash] = true
	t.unloaded--

	var toFire []func()
	if t.unloaded == 0 && !t.allFired {
		t.allFired = true
		toFire = t.allLoaded
		t.allLoaded = nil
	}
	t.mu.Unlock()

	for _, fn := range toFire {
		fn()
	}
}

// AddLeaf records a newly-known leaf (e.g., from a live BPT update during
// BOOTING). Initially unloaded. Idempotent for already-known leaves.
func (t *Tracker) AddLeaf(keyHash [32]byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, known := t.known[keyHash]; known {
		return
	}
	t.known[keyHash] = false
	t.unloaded++
	if t.allFired {
		// All-loaded already fired; a new leaf re-opens the question.
		// Implementations may choose to handle this by signaling a
		// regression; for now we just track it. ACTIVE is reached again
		// only when this leaf is loaded.
		t.allFired = false
	}
}

// UnloadedCount returns the number of known leaves whose account data is
// not yet loaded.
func (t *Tracker) UnloadedCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.unloaded
}

// KnownCount returns the total number of known leaves.
func (t *Tracker) KnownCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.known)
}

// OnAllLoaded registers a callback fired once when UnloadedCount transitions
// to zero. If already at zero when registered, the callback fires immediately.
// Multiple callbacks are supported and fire in registration order.
func (t *Tracker) OnAllLoaded(fn func()) {
	t.mu.Lock()
	if t.unloaded == 0 && !t.allFired {
		t.allFired = true
		t.mu.Unlock()
		fn()
		return
	}
	if t.allFired {
		t.mu.Unlock()
		fn()
		return
	}
	t.allLoaded = append(t.allLoaded, fn)
	t.mu.Unlock()
}

// IterUnloaded calls fn for each known leaf whose account isn't loaded.
// Stops early if fn returns false.
func (t *Tracker) IterUnloaded(fn func(keyHash [32]byte) bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for kh, loaded := range t.known {
		if loaded {
			continue
		}
		if !fn(kh) {
			return
		}
	}
}
