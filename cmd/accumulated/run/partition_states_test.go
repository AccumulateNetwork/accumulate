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

// TestEveryPartitionActiveWaitsForEveryConfiguredPartition — #4550: the
// gate a partition's history backfill runs behind is true only once every
// DAG-BFT partition the instance is configured to run has registered its
// state and may serve. A partition not yet registered is not vacuously
// ACTIVE.
func TestEveryPartitionActiveWaitsForEveryConfiguredPartition(t *testing.T) {
	inst := &Instance{config: &Config{Services: []Service{&DAGBFTService{}, &DAGBFTService{}}}}
	require.False(t, inst.everyPartitionActive(), "no partition registered")

	inst.registerPartitionState("Directory", nodestate.Always{})
	require.False(t, inst.everyPartitionActive(), "one of two partitions registered")

	bvn := nodestate.New(protocol.PartitionUrl("BVN1"))
	inst.registerPartitionState("BVN1", bvn)
	require.False(t, inst.everyPartitionActive(), "the BVN is still joining")

	require.True(t, bvn.PromoteToActive([32]byte{1}, 10))
	require.True(t, inst.everyPartitionActive(), "every partition ACTIVE")
}
