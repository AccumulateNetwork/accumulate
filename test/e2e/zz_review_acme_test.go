// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestReview_AcmeIsAHarnessArtifact settles whether acc://ACME differing after a
// simulator restore is a restore defect or a test-harness artifact.
//
// It restores WITHOUT InitialAcmeSupply(nil), and checks acc://ACME.Issued
// BEFORE either network executes another block. If the restore were at fault,
// the restored value would be wrong in some arbitrary way. If the harness is at
// fault, the restored value is exactly the snapshot's value plus one more
// initial supply - and no other account differs.
func TestReview_AcmeIsAHarnessArtifact(t *testing.T) {
	acctesting.EnableDebugFeatures()

	network := simulator.SimpleNetwork(t.Name(), 1, 1)
	genesis := newGenesisSim(t, network)
	genesis.StepN(20)
	snapshots := collectPartitions(t, genesis)

	issued := func(db database.Viewer) *big.Int {
		var v big.Int
		View(t, db, func(batch *database.Batch) {
			var acme *TokenIssuer
			require.NoError(t, batch.Account(AcmeUrl()).Main().GetAs(&acme))
			v.Set(&acme.Issued)
		})
		return &v
	}

	// Restore the way every other simulator test does: no InitialAcmeSupply
	// override, so the harness adds the supply a second time.
	naive := NewSim(t, network, simulator.SnapshotMap(snapshots))

	supply := big.NewInt(1e6 * AcmePrecision)
	want := new(big.Int).Add(issued(genesis.Database(Directory)), supply)
	require.Equal(t, want.String(), issued(naive.Database(Directory)).String(),
		"the difference is exactly one more initial supply, added by the harness after init")

	differ := accountsThatDiffer(t, genesis.Database(Directory), naive.Database(Directory))
	require.Equal(t, []string{AcmeUrl().String()}, differ,
		"acc://ACME is the only account that differs, and it differs before any block is executed")

	// And with the override, nothing differs at all.
	fixed := restoreSim(t, network, snapshots)
	require.Equal(t, issued(genesis.Database(Directory)).String(), issued(fixed.Database(Directory)).String())
	require.Empty(t, accountsThatDiffer(t, genesis.Database(Directory), fixed.Database(Directory)))
}
