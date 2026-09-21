// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

// A CollectedBlock is what taking a committed block into staging without
// executing it yields (executor spec, "Sync", step 1): how many entries the
// block added to staging.
//
// It used to carry the accounts the block's envelopes named, for the state
// pull to fetch. It does not any more. A block's envelopes name principals,
// signers and anchor pools: never the system accounts every block changes, and
// sometimes a name that cannot be routed, so a tree built from them chases a
// root it can never reach (#4306). The set of accounts to pull comes from the
// block ledger, which records every chain a block changed and which the state
// root commits to -- read from a peer by the join, not produced here
// (executor spec, "Sync", step 3).
type CollectedBlock struct {
	// Held is how many entries this block added to staging.
	Held int

	// Reach is, per stream, the highest sequence number this block carried.
	// It is what the gap check is made against when the join decides whether
	// this block can be executed (#4362).
	Reach []StreamReach
}

// A StreamReach is the highest sequence number one collected block carried on
// one stream.
//
// It is the block's own reach and not the node's. The gap check for block
// B + 1 asks whether the run from the pulled Delivered is contiguous through
// what THAT BLOCK carries; asked instead about everything the node has
// collected, it would call a number that block B + 3 will carry a gap at
// B + 1, and a join that waits for a gap to close that nothing will close
// never executes anything (executor spec, "Sync", §4).
type StreamReach struct {
	ID StreamID

	// High is the highest number the block carried on the stream.
	High uint64
}

// A StreamGap is a stream whose run from Delivered is not contiguous through
// a block's reach: an entry that arrived before this node started listening.
//
// Executing a block over one of these is the #4290 divergence — the peers
// held the missing entry from a block before the node's state, so they
// deliver a longer run from the same block than the node can, and a Merkle
// root over the history of block roots never matches again.
type StreamGap struct {
	ID StreamID

	// Delivered is what the PULLED state says the stream delivered: block
	// output, hashed with the rest, and the peers' word on it — never what
	// staging remembers, which is zero on a node that has executed nothing.
	Delivered uint64

	// Through is how far the run was walked: the block's reach on the stream
	// or the highest number this node holds, whichever is greater.
	Through uint64

	// Missing is the runs of numbers in (Delivered, Through] that the node
	// holds nothing at, oldest first.
	Missing [][2]uint64
}
