// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	v3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	mocks "gitlab.com/accumulatenetwork/accumulate/test/mocks/pkg/api/v3"
)

func querierTestInstance(t *testing.T) *Instance {
	t.Helper()
	inst := new(Instance)
	inst.rootDir = t.TempDir()
	inst.logger = slog.Default()
	inst.services = ioc.Registry{}
	require.NoError(t, ioc.Register[keyvalue.Beginner](inst.services, "BVN1", memory.New(nil)))
	return inst
}

// querierNodeState is what Querier.start hands the querier for the partition.
func querierNodeState(inst *Instance) (nodestate.Serving, error) {
	q := &Querier{Partition: "BVN1"}
	consensus, err := querierWantsConsensus.Get(inst.services, q)
	if err != nil {
		return nil, err
	}
	return q.nodeState(inst, consensus)
}

// TestQuerierNodeState_ConsensusWithoutNodeState -- #4335. A partition that
// has a consensus service has a join state to report; a querier that cannot
// find it must not start answering everything.
func TestQuerierNodeState_ConsensusWithoutNodeState(t *testing.T) {
	inst := querierTestInstance(t)
	require.NoError(t, ioc.Register[v3.ConsensusService](inst.services, "BVN1", mocks.NewConsensusService(t)))

	serving, err := querierNodeState(inst)
	require.Error(t, err)
	require.Nil(t, serving)
	require.Contains(t, err.Error(), "BVN1")
	require.Contains(t, err.Error(), "node state")
}

// A querier with no consensus service is a standalone one: it serves what its
// store holds, and says so by being given nodestate.Always, never nil.
func TestQuerierNodeState_NoConsensusServesEverything(t *testing.T) {
	serving, err := querierNodeState(querierTestInstance(t))
	require.NoError(t, err)
	require.Equal(t, nodestate.Always{}, serving)
}

func TestQuerierNodeState_ConsensusWithNodeState(t *testing.T) {
	inst := querierTestInstance(t)
	require.NoError(t, ioc.Register[v3.ConsensusService](inst.services, "BVN1", mocks.NewConsensusService(t)))
	machine := nodestate.New(protocol.PartitionUrl("BVN1"))
	require.NoError(t, ioc.Register[nodestate.Serving](inst.services, "BVN1", machine))

	serving, err := querierNodeState(inst)
	require.NoError(t, err)
	require.Same(t, machine, serving)
}
