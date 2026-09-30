// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package spine

import (
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// LoadGenesisGlobals extracts the trust-anchor state — the validator set and
// network globals — from a genesis snapshot, the walk's only out-of-band
// trust input.
func LoadGenesisGlobals(file ioutil2.SectionReader, partition config.NetworkUrl) (*network.GlobalValues, error) {
	db := database.OpenInMemory(nil)
	err := snapshot.FullRestore(db, file, nil, partition)
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
