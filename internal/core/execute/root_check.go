// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import "sync"

// RootCheck is what this node last learned when it compared its own root with
// the root its partition signed, and whether that lets it sign.
//
// A node signs and dispatches block N's anchor and synthetic transactions
// only if the newest root it could check matched (executor spec, "Sync",
// §6, "Two things do not wait for ACTIVE"; lag one: block N's own root cannot
// be checked before the node signs it, because the network signs it after).
// A node whose last checked root did not match signs nothing, dispatches
// nothing, and repairs. A wrongly executing node then stalls rather than
// forms a wrong quorum with others like it.
//
// It is one value per node and partition, shared by everything that signs
// for the partition under this node's key: the conductor (anchors), the
// executor (the leader's synthetic dispatch) and the sequencer (the anchors
// it answers for). Whoever compares roots reports here; that is the join's
// root watch, which does not report yet (DIFFERENCES E19).
//
// A node that has checked nothing signs: a node that never joined has taken
// nothing from a peer, and a partition whose validators are all booting must
// still anchor, or no root is ever signed for any of them to check.
//
// The zero value and a nil RootCheck are "nothing checked".
type RootCheck struct {
	mu      sync.Mutex
	block   uint64
	checked bool
	matched bool
}

// Matched records that the root this node had after block matched the root
// its partition signed for it.
func (r *RootCheck) Matched(block uint64) { r.report(block, true) }

// Mismatched records that the root this node had after block differed from
// the root its partition signed for it.
func (r *RootCheck) Mismatched(block uint64) { r.report(block, false) }

// report keeps the newest check. A report for a block older than the one held
// says nothing about the node's state now, and is ignored; a second report
// for the same block (a repair checked again) replaces the first.
func (r *RootCheck) report(block uint64, matched bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.checked && block < r.block {
		return
	}
	r.block, r.checked, r.matched = block, true, matched
}

// Last is the newest check: the block, whether its root matched, and whether
// there has been one.
func (r *RootCheck) Last() (block uint64, matched, checked bool) {
	if r == nil {
		return 0, false, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.block, r.matched, r.checked
}

// MaySign reports whether this node may sign and dispatch for its partition:
// true unless the newest root it checked did not match.
func (r *RootCheck) MaySign() bool {
	_, matched, checked := r.Last()
	return !checked || matched
}
