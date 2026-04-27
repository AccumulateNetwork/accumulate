// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package pinned holds the per-network genesis snapshot hashes that
// the bootstrap launcher uses as its only out-of-band trust input
// (issue #3979, parent #3953).
//
// The values here are pinned at build time. A node bootstrapping
// against a network requires the binary's pinned hash to match the
// network's genesis snapshot; mismatch aborts startup unless the
// operator passes --trust-anchor explicitly.
//
// Until per-network hashes are populated by the release process, the
// table is empty — callers that don't pass --trust-anchor receive a
// zero hash and the back-walker's genesis-terminator check uses the
// loose "any SystemGenesis-typed entry" fallback (which is *not*
// secure; it's there so existing development flows work). When real
// hashes are populated, the back-walker fails closed on mismatch.
package pinned

import "encoding/hex"

// GenesisHash returns the pinned genesis snapshot hash for the named
// network, or [32]byte{} (zero) if the network is unknown. Callers
// MUST treat zero as "no pin" and apply development-only fallback
// rules accordingly.
func GenesisHash(network string) [32]byte {
	if h, ok := networkGenesis[network]; ok {
		return h
	}
	return [32]byte{}
}

// IsKnown reports whether the named network has a pinned hash.
func IsKnown(network string) bool {
	_, ok := networkGenesis[network]
	return ok
}

// networkGenesis is the pinned table. Populate at release time:
//
//	"mainnet": mustHex("..."),
//	"kermit":  mustHex("..."),
//
// Empty by default so dev/test flows aren't blocked. The release
// process should add entries as networks are launched / re-anchored.
var networkGenesis = map[string][32]byte{}

// mustHex parses a 64-char hex string into a [32]byte. Panics on
// malformed input — used in package-level init only, where panic at
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

// _ keeps mustHex referenced even when networkGenesis is empty so the
// linter doesn't complain. Remove when real entries are added.
var _ = mustHex
