// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// filling is a partition's fill with n sections still not loaded.
type filling int

func (f *filling) SectionsNotLoaded() int { return int(*f) }

// TestEveryPartitionPastItsFillWaitsForEveryConfiguredPartition — #4550,
// #4554: the gate a partition's history backfill runs behind is true only
// once every DAG-BFT partition the instance is configured to run has
// registered its state, may serve, and has loaded every section of its tree.
// A partition not yet registered is not vacuously past its fill; a partition
// ACTIVE and still filling holds the gate for the whole node (run
// 20261008T221608Z: the Directory's backfill ran through the BVN's fill).
func TestEveryPartitionPastItsFillWaitsForEveryConfiguredPartition(t *testing.T) {
	inst := &Instance{config: &Config{Services: []Service{&DAGBFTService{}, &DAGBFTService{}}}}
	require.False(t, inst.everyPartitionPastItsFill(), "no partition registered")

	inst.registerPartitionState("Directory", nodestate.Always{}, nil)
	require.False(t, inst.everyPartitionPastItsFill(), "one of two partitions registered")

	bvn := nodestate.New(protocol.PartitionUrl("BVN1"))
	left := filling(3)
	inst.registerPartitionState("BVN1", bvn, &left)
	require.False(t, inst.everyPartitionPastItsFill(), "the BVN is still joining")

	require.True(t, bvn.PromoteToActive([32]byte{1}, 10))
	require.False(t, inst.everyPartitionPastItsFill(), "the BVN is ACTIVE and still filling: the Directory's backfill would run through its fill")

	left = 0
	require.True(t, inst.everyPartitionPastItsFill(), "every partition ACTIVE and past its fill")
}
