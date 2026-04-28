// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package pinned holds the per-network validator-set hash that the
// v2 bootstrap launcher uses as its only out-of-band trust input.
//
// Each entry pairs a network name with a hash of the validator set
// at a specific recent height. A node bootstrapping against the
// network requires the binary's pinned hash to match what the
// network actually had at that height; mismatch aborts startup
// unless the operator passes an explicit override.
//
// Until per-network hashes are populated by the release process, the
// table is empty — callers that look up an unknown network receive
// a zero hash. The accumulated run handoff treats zero as "no pin
// available" and falls back to a documented warn-and-proceed mode
// for development networks; production deployments must either ship
// a populated table or refuse to start without an override.
//
// v1 used a pinned genesis-snapshot hash here; the v2 trust model
// (validator quorum on a recent block, no genesis terminator) makes
// the genesis hash irrelevant. The package name and surface remain
// the same so callers porting from v1 don't need restructuring.
package pinned

import "encoding/hex"

// Pin records the validator-set state the binary trusts for a
// network: the hash of the operators key book at PinnedHeight.
//
// PinnedHeight is the height at which the validator-set hash was
// captured. The bootstrap header walk's first verification happens
// at this height; the walker carries the validator set forward via
// keybookat as operators-keybook updates appear in subsequent
// blocks.
type Pin struct {
	// ValidatorSetHash is the hash of the canonical-form validator
	// set at PinnedHeight. The header walker rejects any header
	// whose pre-walk validator set doesn't hash to this.
	ValidatorSetHash [32]byte

	// PinnedHeight is the partition's minor block height at which
	// ValidatorSetHash applies.
	PinnedHeight uint64
}

// IsZero reports whether the pin is the zero value (no hash
// recorded).
func (p Pin) IsZero() bool {
	return p.ValidatorSetHash == ([32]byte{}) && p.PinnedHeight == 0
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
// entries as networks reach a stable validator set or as
// re-anchoring is performed.
//
// Example population (commented; populate at release time):
//
//	"mainnet": {
//	    ValidatorSetHash: mustHex("..."),
//	    PinnedHeight:     12345678,
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
