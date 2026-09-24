// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package simulator_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	nodeconfig "gitlab.com/accumulatenetwork/accumulate/internal/node/config"
	accumulated "gitlab.com/accumulatenetwork/accumulate/internal/node/daemon"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// simWithAFollower is a one-BVN network of three validators and a fourth node,
// BVN0's node 3, whose key is in no committee.
func simWithAFollower(t *testing.T) (*Sim, *simulator.Partition) {
	net := simulator.NewSimpleNetwork(t.Name(), 1, 3)
	net.Bvns[0].Nodes = append(net.Bvns[0].Nodes, &accumulated.NodeInit{
		DnnType:    nodeconfig.Follower,
		BvnnType:   nodeconfig.Follower,
		PrivValKey: acctesting.GenerateKey(t.Name(), "follower"),
		DnNodeKey:  acctesting.GenerateKey(t.Name(), "follower", "dn"),
		BvnNodeKey: acctesting.GenerateKey(t.Name(), "follower", "bvn"),
	})
	sim := NewSim(t,
		simulator.WithNetwork(net),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	p := sim.S.Partition("BVN0")
	require.Equal(t, 4, p.NodeCount())
	return sim, p
}

func ledgerIndex(t *testing.T, db *database.Database) uint64 {
	t.Helper()
	var ledger *SystemLedger
	View(t, db, func(batch *database.Batch) {
		require.NoError(t, batch.Account(PartitionUrl("BVN0").JoinPath(Ledger)).Main().GetAs(&ledger))
	})
	return ledger.Index
}

// A restarted simulator node refuses a read addressed to it with NotReady, by
// the production querier's gate over the machine of the join RestartNode
// built, and a read that names no node is still answered (#4363).
func TestARestartedNodeRefusesReadsByItsJoinsMachine(t *testing.T) {
	sim, p := simWithAFollower(t)
	sim.StepN(5)

	const follower = 3
	ctx := context.Background()
	query := func(i int) error {
		_, err := sim.S.Services().ForPeer(p.NodePeerID(i)).
			ForAddress(api.ServiceTypeQuery.AddressFor("BVN0").Multiaddr()).
			Query(ctx, PartitionUrl("BVN0").JoinPath(Ledger), &api.DefaultQuery{})
		return err
	}
	require.NoError(t, query(follower), "precondition: a node that has not restarted answers")
	require.Nil(t, p.NodeJoinState(follower), "precondition: a node that has not restarted has no join")

	p.RestartNode(follower)
	state := p.NodeJoinState(follower)
	require.NotNil(t, state, "RestartNode built no join state")
	require.Equal(t, nodestate.StateBooting, state.Machine().State())

	err := query(follower)
	require.Error(t, err, "a joining node answered a read addressed to it")
	require.Equal(t, errors.NotReady, errors.Code(err), "a joining node must refuse a read as NotReady, got %v", err)
	require.NoError(t, query(0), "a validator stopped answering")

	// A read that names no node is answered by one that can: every one of
	// these would land on the joining node about a quarter of the time.
	for i := 0; i < 40; i++ {
		_, err := sim.S.Services().Query(ctx, PartitionUrl("BVN0").JoinPath(Ledger), &api.DefaultQuery{})
		require.NoError(t, err, "a routed read reached the joining node")
	}
}

// A stopped simulator node is handed no block and answers nothing, and the
// partition runs on without it (#4363).
func TestAStoppedNodeIsHandedNothingAndAnswersNothing(t *testing.T) {
	sim, p := simWithAFollower(t)
	sim.StepN(5)

	const follower = 3
	p.StopNode(follower)
	stoppedAt := sim.S.BlockIndex("BVN0")
	stoppedLedger := ledgerIndex(t, p.NodeDatabase(follower))

	// Every block from here carries something, so the ledger moves.
	sim.StepN(10)
	require.Greater(t, sim.S.BlockIndex("BVN0"), stoppedAt, "the partition stopped with the node")
	require.Greater(t, ledgerIndex(t, p.NodeDatabase(0)), stoppedLedger, "the validators executed nothing")
	require.Equal(t, stoppedLedger, ledgerIndex(t, p.NodeDatabase(follower)),
		"the stopped node went on executing blocks")

	_, err := sim.S.Services().ForPeer(p.NodePeerID(follower)).
		ForAddress(api.ServiceTypeQuery.AddressFor("BVN0").Multiaddr()).
		Query(context.Background(), PartitionUrl("BVN0").JoinPath(Ledger), &api.DefaultQuery{})
	require.Error(t, err, "the stopped node answered a read")
	require.Equal(t, errors.NoPeer, errors.Code(err), "a stopped node must be absent, got %v", err)
}
