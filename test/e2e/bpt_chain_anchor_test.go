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
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

// #4272: the bpt chain records every block's BPT root and was never anchored,
// so a root could be proven no further than the node asserting it.

// The bpt chain is now anchored, so every BPT root the partition has ever
// produced is provable into a root-chain anchor (#4272).
func TestBptChainIsAnchored(t *testing.T) {
	sim, _, _ := twoCallSim(t)
	sim.StepN(20)

	View(t, sim.Database("BVN1"), func(batch *database.Batch) {
		c := batch.Account(PartitionUrl("BVN1").JoinPath(Ledger)).BptChain()
		head, err := c.Inner().Head().Get()
		require.NoError(t, err)
		require.NotZero(t, head.Count, "the bpt chain must be written")

		idx, err := c.Index().Head().Get()
		require.NoError(t, err)
		require.NotZero(t, idx.Count, "the bpt chain must be anchored, so it has an index chain")
		t.Logf("bpt chain: %d entries, %d index entries", head.Count, idx.Count)
	})
}
