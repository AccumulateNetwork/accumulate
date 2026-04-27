// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package backwalk constructs the proof of derivation from genesis as a
// graph traversal across main chains (issue #3960, parent #3953).
//
// The traversal starts at a current account or keybook (pulled from a
// peer at the bootstrap pin block H) and walks each main chain backward.
// Every entry is verified by one of two rules:
//
//   - User-signed entries: signatures live on the *signer's* signature
//     chain (lateral navigation); resolve the keypage at the entry's
//     block time via #3957; verify the signature.
//   - Synthetic entries (cross-partition forwards, anchor results,
//     etc., carrying InternalSignature): trace the Cause to the
//     producing transaction and recurse; additionally verify the
//     synthetic was included in a validator-quorum-signed anchor. The
//     validator set at that block time is itself resolved via #3957
//     (the operators / partition keybook).
//
// Recursion bottoms out at the genesis snapshot: each chain's earliest
// entry must reference an account or keybook present in the genesis
// snapshot whose hash matches the binary's pinned value.
//
// Memoization keyed by (account, blockTime) handles legitimate cyclic
// dependencies between keybooks (mutual signing relationships).
package backwalk

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Walker constructs a proof of derivation by walking main chains.
type Walker struct {
	mu              sync.Mutex
	pinnedGenesis   [32]byte
	memo            map[memoKey]*VerifiedEntry
	terminations    map[*url.URL]struct{}
	stack           map[memoKey]struct{} // cycle detection (current DFS path)
	maxRecursion    int
	currentDepth    int
}

type memoKey struct {
	url  string
	time int64 // unix nanos
}

// VerifiedEntry is one validated main-chain entry in the proof.
type VerifiedEntry struct {
	Account     *url.URL
	BlockTime   time.Time
	TxHash      [32]byte
	SignerUrl   *url.URL
	Synthetic   bool // true if authenticated by validator-quorum + Cause
	GenesisTerm bool // true if this entry's chain bottoms out at the genesis snapshot
}

// Options configures a Walker.
type Options struct {
	// PinnedGenesisHash is the hash of the genesis snapshot the binary
	// was built against — the only out-of-band trust input.
	PinnedGenesisHash [32]byte

	// MaxRecursion bounds the depth of (account, blockTime) recursion
	// to defend against pathological cycles. Zero uses a sane default
	// (see DefaultMaxRecursion).
	MaxRecursion int
}

// DefaultMaxRecursion is the default depth bound for keypage-at-time
// recursion. Empirically the operator key book on mainnet has 1
// main-chain entry, so depth ~1 is enough for the typical case; this
// default leaves headroom for future complexity.
const DefaultMaxRecursion = 64

// New constructs a Walker.
func New(opts Options) *Walker {
	maxR := opts.MaxRecursion
	if maxR == 0 {
		maxR = DefaultMaxRecursion
	}
	return &Walker{
		pinnedGenesis: opts.PinnedGenesisHash,
		memo:          make(map[memoKey]*VerifiedEntry),
		terminations:  make(map[*url.URL]struct{}),
		stack:         make(map[memoKey]struct{}),
		maxRecursion:  maxR,
	}
}

// ErrCycleDetected is returned when keypage-at-time recursion enters a
// cycle that can't be broken by memoization.
var ErrCycleDetected = errors.New("backwalk: recursion cycle detected")

// ErrMaxRecursion is returned when the recursion depth bound is hit.
var ErrMaxRecursion = errors.New("backwalk: maximum recursion depth exceeded")

// ErrNotImplemented is returned for code paths not yet implemented.
// The walker's interface is stable; the underlying chain-walking and
// signature-verification logic is implemented incrementally.
var ErrNotImplemented = errors.New("backwalk: code path not yet implemented")

// Walk runs the back-walk for accountUrl as of blockTime, recording the
// validated chain in the Walker's internal proof state. Returns the
// terminal verified entry (the genesis-snapshot hit) or an error.
func (w *Walker) Walk(batch *database.Batch, accountUrl *url.URL, blockTime time.Time) (*VerifiedEntry, error) {
	if accountUrl == nil {
		return nil, fmt.Errorf("nil accountUrl")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	return w.walkLocked(batch, accountUrl, blockTime)
}

func (w *Walker) walkLocked(batch *database.Batch, accountUrl *url.URL, blockTime time.Time) (*VerifiedEntry, error) {
	mk := memoKey{url: accountUrl.String(), time: blockTime.UnixNano()}

	if cached, ok := w.memo[mk]; ok {
		return cached, nil
	}
	if _, on := w.stack[mk]; on {
		// We're already walking this (account, blockTime) — cycle.
		// The caller can break the cycle by returning a tentative
		// verification record; for now surface the cycle.
		return nil, ErrCycleDetected
	}
	if w.currentDepth >= w.maxRecursion {
		return nil, ErrMaxRecursion
	}

	w.stack[mk] = struct{}{}
	w.currentDepth++
	defer func() {
		delete(w.stack, mk)
		w.currentDepth--
	}()

	// Real implementation walks accountUrl's main chain backwards from
	// the latest entry whose block time <= blockTime, applies the
	// matching verification rule per entry, terminates at genesis, and
	// memoizes the result.
	//
	// This scaffolding records the call and returns a NotImplemented
	// terminal so the surrounding plumbing (persistence, advertisement)
	// can be wired and tested. The chain-walk + signature-verify logic
	// is the next slice on this branch.
	return nil, fmt.Errorf("walk %s @ %s: %w", accountUrl, blockTime.Format(time.RFC3339), ErrNotImplemented)
}

// MemoSize returns the number of cached (account, blockTime) → entry
// memoizations. Used by tests and the persistence layer (#3965).
func (w *Walker) MemoSize() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.memo)
}

// PinnedGenesisHash returns the hash the walker is anchoring to.
func (w *Walker) PinnedGenesisHash() [32]byte {
	return w.pinnedGenesis
}

// Memoize manually records a verified entry. Exposed for tests and for
// loading persisted memoizations on restart (#3965). The entry's
// Account and BlockTime are used as the key.
func (w *Walker) Memoize(entry *VerifiedEntry) {
	if entry == nil || entry.Account == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	mk := memoKey{url: entry.Account.String(), time: entry.BlockTime.UnixNano()}
	w.memo[mk] = entry
}
