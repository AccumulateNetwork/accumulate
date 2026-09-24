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
}
