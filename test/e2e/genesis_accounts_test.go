// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// A network created after #3910 has no votes or evidence account: they held
// CometBFT's commit info and misbehaviour, which a DAG-BFT block does not
// have (executor spec, block begin step 7). Removing them changed the genesis
// state root, for fresh installs only.
func TestGenesisHasNoVotesOrEvidenceAccount(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 1),
		simulator.Genesis(GenesisTime),
	)

	for _, part := range []string{Directory, "BVN0"} {
		require.NoError(t, sim.Database(part).View(func(batch *database.Batch) error {
			for _, name := range []string{Votes, Evidence} {
				u := PartitionUrl(part).JoinPath(name)
				_, err := batch.Account(u).Main().Get()
				require.ErrorIs(t, err, errors.NotFound, "%v exists", u)
			}
			// Genesis still made the partition's other system accounts.
			_, err := batch.Account(PartitionUrl(part).JoinPath(Ledger)).Main().Get()
			require.NoError(t, err)
			return nil
		}))
	}
}
