// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/indexing"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/lastcomplete"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// The halt wiring end to end (#4512): the closure a node installs
// (lastcomplete.Source) queries the Directory's anchor pool through the API
// the node's client reaches, decodes the ledger, and finds the anchor in the
// node's own store; the service puts N in the halt line's fields and on the
// gauge; resuming clears the gauges; the Directory partition installs none.
func TestTheHaltNamesTheLastCompleteBlockThroughTheRealSource(t *testing.T) {
	sim := harness.NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 4),
		simulator.Genesis(harness.GenesisTime),
	)
	sim.StepN(60)
	bvn0 := sim.S.Partition("BVN0")
	for _, i := range []int{2, 3} {
		bvn0.StopNode(i)
	}
	sim.StepN(30)

	svc, _ := newLivenessService(t)
	svc.config.Partition = &protocol.PartitionInfo{ID: "BVN0"}
	db := bvn0.NodeDatabase(0)
	svc.config.LastComplete = lastcomplete.Source(sim.S.Services(), db, "BVN0")
	require.NotNil(t, svc.config.LastComplete)

	f := fields(svc.lastComplete())
	require.NotEqual(t, "unknown", f["lastCompleteBlock"], "%v", f["lastCompleteWhy"])
	n := f["lastCompleteBlock"].(uint64)
	require.NotZero(t, n)

	// The root the halt line states is the one the node recorded for N.
	require.NoError(t, db.View(func(b *database.Batch) error {
		root, blk, err := indexing.BPTRootAt(config.NetworkUrl{URL: protocol.PartitionUrl("BVN0")}, b, n)
		require.NoError(t, err)
		require.Equal(t, n, blk)
		require.Equal(t, root, rootOf(t, f["lastCompleteRoot"].(string)))
		return nil
	}))
	require.EqualValues(t, n, testutil.ToFloat64(mLastComplete.WithLabelValues("BVN0")))

	// The partition resumes: both gauges are cleared.
	mBelowQuorum.WithLabelValues("BVN0").Set(1)
	svc.stallSince = time.Now().Add(-time.Minute)
	svc.noteBlockProduced(n+1, 1)
	require.Zero(t, testutil.ToFloat64(mBelowQuorum.WithLabelValues("BVN0")))
	require.Zero(t, testutil.CollectAndCount(mLastComplete), "the last complete block outlived the stall")

	// The Directory installs no source, so its halt says nothing of N.
	require.Nil(t, lastcomplete.Source(sim.S.Services(), sim.S.Partition(protocol.Directory).NodeDatabase(0), protocol.Directory))
}

func rootOf(t *testing.T, s string) (root [32]byte) {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	require.Len(t, b, len(root))
	copy(root[:], b)
	return root
}
