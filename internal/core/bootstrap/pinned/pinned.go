// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package pinned holds the per-network DN genesis StateTreeAnchor
// that the v2 bootstrap launcher uses as its only out-of-band trust
// input.
//
// The launcher walks DN-validator-signed DAs backward through a
// chosen BVN's anchor pool. The walk terminates at DN major-block 1.
// The pin is the DN's StateTreeAnchor at that genesis boundary —
// invariant across BVN destinations, so one pin per network covers
// every BVN bootstrap.
//
// Until per-network values are populated by the release process, the
// table is empty — callers that look up an unknown network receive
// a zero hash. Operators on dev networks can pass
// --genesis-state-tree-anchor to override.
//
// History: v1 pinned a genesis-snapshot hash. The first v2 draft
// pinned a validator-set hash at a recent height. Both were wrong-
// shaped: the actual proof artifact is the DN's BPT root at major-
// block 1, since that's what the back-walk naturally terminates
// against (extracted from each DA's PartitionAnchor).
package pinned

import "encoding/hex"

// Pin holds the per-network bootstrap anchor. One field today: the
// DN's StateTreeAnchor at major-block 1.
//
// Kept as a struct so future fields (e.g., a major-block-1 BlockTime
// for client-side sanity checks, or a pin-validity-window) can
// extend without breaking the call surface.
type Pin struct {
	// DNGenesisStateTreeAnchor is the DN's BPT root at DN major-
	// block 1. The bootstrap back-walk's terminator value must equal
	// this. Fail closed otherwise.
	DNGenesisStateTreeAnchor [32]byte
}

// IsZero reports whether the pin is the zero value (no anchor
// recorded).
func (p Pin) IsZero() bool {
	return p.DNGenesisStateTreeAnchor == ([32]byte{})
}

// Get returns the pin for the named network, or the zero Pin if the
// network is unknown. Callers MUST treat the zero Pin as "no pin"
// and apply development-only fallback rules accordingly.
func Get(network string) Pin {
	if p, ok := networkPins[network]; ok {
		return p
	}
	return Pin{}
}

// IsKnown reports whether the named network has a populated pin.
func IsKnown(network string) bool {
	_, ok := networkPins[network]
	return ok
}

// networkPins is the build-time-populated table. Empty by default
// so dev/test flows aren't blocked. The release process should add
// entries as networks reach the point where their genesis
// StateTreeAnchor is known good.
//
// Example population (commented; populate at release time):
//
//	"mainnet": {
//	    DNGenesisStateTreeAnchor: mustHex("..."),
//	},
var networkPins = map[string]Pin{}

// mustHex parses a 64-char hex string into a [32]byte. Panics on
// malformed input — used at package init only, where panic at
// startup is the right behavior.
func mustHex(s string) [32]byte {
	if len(s) != 64 {
		panic("pinned: hash must be 64 hex chars: " + s)
	}
	var out [32]byte
	if _, err := hex.Decode(out[:], []byte(s)); err != nil {
		panic("pinned: " + err.Error())
	}
	return out
}

// _ keeps mustHex referenced even when networkPins is empty so the
// linter doesn't complain. Remove when real entries are added.
var _ = mustHex
