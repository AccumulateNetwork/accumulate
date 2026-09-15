// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package fastsync

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
)

// DI keeps its own LoadGenesisGlobals in snapshot.go, which restores through
// snapshot.FullRestore rather than database.Restore. Only GenesisAnchorHash is
// new here (#4275).

// GenesisAnchorHash is the canonical hash of a trust anchor: SHA-256 over the
// marshalled network definition followed by the marshalled globals. It is what
// a network operator publishes out of band and what a verifier recomputes from
// whatever snapshot it was handed, so that the two can be compared without
// either party trusting the other's copy.
//
// It commits to the validator set and the network parameters — the two things
// the induction walk starts from — and to nothing else in the snapshot.
func GenesisAnchorHash(g *network.GlobalValues) ([32]byte, error) {
	if g == nil || g.Network == nil || g.Globals == nil {
		return [32]byte{}, errors.BadRequest.With("missing network definition or globals")
	}

	def, err := g.Network.MarshalBinary()
	if err != nil {
		return [32]byte{}, errors.UnknownError.WithFormat("marshal network definition: %w", err)
	}
	glob, err := g.Globals.MarshalBinary()
	if err != nil {
		return [32]byte{}, errors.UnknownError.WithFormat("marshal network globals: %w", err)
	}

	h := sha256.New()
	h.Write(def)
	h.Write(glob)
	var v [32]byte
	copy(v[:], h.Sum(nil))
	return v, nil
}
