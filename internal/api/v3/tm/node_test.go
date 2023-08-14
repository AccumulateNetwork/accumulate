// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tm_test

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/p2p"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	. "gitlab.com/accumulatenetwork/accumulate/internal/api/v3/tm"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestConsensusStatus(t *testing.T) {
	logger := acctesting.NewTestLogger(t)
	net := simulator.NewSimpleNetwork(t.Name(), 1, 1)
	sim, err := simulator.New(
		logger,
		simulator.MemoryDatabase,
		simulator.WithNetwork(net),
		simulator.Genesis(GenesisTime),
	)
	require.NoError(t, err)

	svc := NewConsensusService(ConsensusServiceParams{
		Logger:           logger,
		Local:            staticClient{},
		Database:         sim.Database(protocol.Directory),
		PartitionID:      protocol.Directory,
		PartitionType:    protocol.PartitionTypeDirectory,
		EventBus:         events.NewBus(logger),
		NodeKeyHash:      sha256.Sum256(net.Bvns[0].Nodes[0].DnNodeKey[32:]),
		ValidatorKeyHash: sha256.Sum256(net.Bvns[0].Nodes[0].PrivValKey[32:]),
	})

	s, err := svc.ConsensusStatus(context.Background(), api.ConsensusStatusOptions{})
	require.NoError(t, err)

	require.Len(t, s.Peers, 2)
	require.Equal(t, "82e8cf34c37159f5379f8dfc96c3f01ba15cda77", s.Peers[0].NodeID)
	require.Equal(t, 16591, int(s.Peers[0].Port))
	require.Equal(t, "bvn1-seed.testnet.accumulatenetwork.io", s.Peers[0].Host)
	require.Equal(t, "7883dec6a840fc9c115554fb73ce532e17ccddb0", s.Peers[1].NodeID)
	require.Equal(t, 16591, int(s.Peers[1].Port))
	require.Equal(t, "bvn2-seed.testnet.accumulatenetwork.io", s.Peers[1].Host)
}

type staticClient struct{}

func (staticClient) Status(context.Context) (*coretypes.ResultStatus, error) {
	return &coretypes.ResultStatus{
		SyncInfo: coretypes.SyncInfo{
			LatestBlockHeight: 123,
			LatestBlockHash:   make(bytes.HexBytes, 32),
			LatestBlockTime:   GenesisTime,
		},
	}, nil
}

func (staticClient) NetInfo(context.Context) (*coretypes.ResultNetInfo, error) {
	return &coretypes.ResultNetInfo{
		Peers: []coretypes.Peer{
			{
				NodeInfo: p2p.DefaultNodeInfo{
					DefaultNodeID: "82e8cf34c37159f5379f8dfc96c3f01ba15cda77",
					ListenAddr:    "bvn1-seed.testnet.accumulatenetwork.io:16591",
				},
				RemoteIP: "44.206.74.96",
			},
			{
				NodeInfo: p2p.DefaultNodeInfo{
					DefaultNodeID: "7883dec6a840fc9c115554fb73ce532e17ccddb0",
					ListenAddr:    "tcp://bvn2-seed.testnet.accumulatenetwork.io:16591",
				},
				RemoteIP: "44.208.174.1",
			},
		},
	}, nil
}
