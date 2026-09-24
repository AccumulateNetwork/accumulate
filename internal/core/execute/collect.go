// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"fmt"
	"sort"
	"strings"
)

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

	// Streams is, per stream the block carried, how many of its arrivals the
	// block held and why it held none of the others, in the order the block
	// sorts streams. A joining node's gap is an entry it did not hold, and
	// the reason says whether its peers did (#4432).
	Streams []CollectedStream
}

// A CollectedStream is what a collected block brought on one stream.
type CollectedStream struct {
	ID      StreamID
	Held    int
	NotHeld map[NotHeldReason]int
}

// A NotHeldReason is why collecting did not hold an arrival.
type NotHeldReason string

const (
	// NotHeldDelivered: at or below the Delivered this node's store says.
	NotHeldDelivered NotHeldReason = "delivered"
	// NotHeldHorizon: past the sanity horizon above Delivered.
	NotHeldHorizon NotHeldReason = "horizon"
	// NotHeldDuplicate: its number is already held; first sighting wins.
	NotHeldDuplicate NotHeldReason = "duplicate"
	// NotHeldRefused: its own executor refuses it, as a block would.
	NotHeldRefused NotHeldReason = "refused"
	// NotHeldUnattested: not proven here and not signed by a validator of
	// its source (#4243).
	NotHeldUnattested NotHeldReason = "unattested"
	// NotHeldProofBudget: its source's proof was turned away for want of
	// budget, and its entries with it (#4282).
	NotHeldProofBudget NotHeldReason = "proofBudget"
)

// Add counts other's arrivals into s.
func (s *CollectedStream) Add(other CollectedStream) {
	s.Held += other.Held
	for r, n := range other.NotHeld {
		if s.NotHeld == nil {
			s.NotHeld = map[NotHeldReason]int{}
		}
		s.NotHeld[r] += n
	}
}

// String is the stream, what it held, and each reason it did not hold the
// rest, reasons in name order: what the join's staging line prints.
func (s CollectedStream) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s held=%d", StreamName(s.ID), s.Held)
	reasons := make([]string, 0, len(s.NotHeld))
	for r := range s.NotHeld {
		reasons = append(reasons, string(r))
	}
	sort.Strings(reasons)
	for _, r := range reasons {
		fmt.Fprintf(&b, " %s=%d", r, s.NotHeld[NotHeldReason(r)])
	}
	return b.String()
}

// StreamName is a stream as source->ledger, without the scheme: what the
// join's lines and its gap gauge call it.
func StreamName(id StreamID) string {
	name := func(u fmt.Stringer, ok bool) string {
		if !ok {
			return "?"
		}
		return strings.TrimPrefix(u.String(), "acc://")
	}
	return name(id.Source, id.Source != nil) + "->" + name(id.Ledger, id.Ledger != nil)
}
