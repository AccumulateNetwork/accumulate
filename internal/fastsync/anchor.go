// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package fastsync

import (
	"crypto/sha256"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// LoadGenesisGlobals extracts the trust-anchor state — the validator set and
// network globals — from a genesis snapshot. This is the walk's only
// out-of-band trust input: [NewSpine] starts from what this returns, and
// everything the walk concludes rests on it being the right one.
//
// Obtaining the snapshot from the network being verified proves nothing on its
// own. What makes it a trust root is that its [GenesisAnchorHash] matches a
// hash pinned somewhere the network does not control.
func LoadGenesisGlobals(file ioutil.SectionReader, partition config.NetworkUrl) (*network.GlobalValues, error) {
	db := database.OpenInMemory(nil)
	defer db.Close()

	err := database.Restore(db, file, nil)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("restore genesis snapshot: %w", err)
	}

	g := new(network.GlobalValues)
	err = db.View(func(batch *database.Batch) error {
		return g.Load(partition.URL, func(account *url.URL, target interface{}) error {
			return batch.Account(account).Main().GetAs(target)
		})
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("load genesis globals: %w", err)
	}
	return g, nil
}

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
