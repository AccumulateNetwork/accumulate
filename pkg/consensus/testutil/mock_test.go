// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package testutil

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
)

func TestMockHost(t *testing.T) {
	peerID := peer.ID("test-peer-1")
	host := NewMockHost(peerID)

	t.Run("ID", func(t *testing.T) {
		assert.Equal(t, peerID, host.ID())
	})

	t.Run("SetAndGetStreamHandler", func(t *testing.T) {
		protoID := protocol.ID("/test/1.0")

		host.SetStreamHandler(protoID, func(s network.Stream) {
			// Handler receives stream
		})

		handler, ok := host.GetHandler(protoID)
		require.True(t, ok)
		require.NotNil(t, handler)
	})

	t.Run("ConnectAndNewStream", func(t *testing.T) {
		otherPeer := peer.ID("test-peer-2")
		err := host.Connect(context.Background(), peer.AddrInfo{ID: otherPeer})
		require.NoError(t, err)

		stream, err := host.NewStream(context.Background(), otherPeer, "/test/1.0")
		require.NoError(t, err)
		require.NotNil(t, stream)
	})

	t.Run("Close", func(t *testing.T) {
		err := host.Close()
		require.NoError(t, err)
	})
}

func TestMockStream(t *testing.T) {
	stream := &MockStream{
		readBuf:    bytes.NewBuffer(nil),
		writeBuf:   bytes.NewBuffer(nil),
		protocolID: "/test/1.0",
	}

	t.Run("WriteAndRead", func(t *testing.T) {
		// Write data to the stream's read buffer (simulating incoming data)
		stream.WriteData([]byte("hello world"))

		// Read from the stream
		buf := make([]byte, 11)
		n, err := stream.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, 11, n)
		assert.Equal(t, "hello world", string(buf))
	})

	t.Run("WriteToStream", func(t *testing.T) {
		// Write to the stream
		_, err := stream.Write([]byte("outgoing"))
		require.NoError(t, err)

		// Read what was written
		data := stream.ReadWrittenData()
		assert.Equal(t, "outgoing", string(data))
	})

	t.Run("Close", func(t *testing.T) {
		err := stream.Close()
		require.NoError(t, err)
		assert.True(t, stream.IsClosed())
	})
}

func TestMockP2PNetwork(t *testing.T) {
	network := NewMockP2PNetwork()

	host1 := NewMockHost(peer.ID("peer-1"))
	host2 := NewMockHost(peer.ID("peer-2"))
	host3 := NewMockHost(peer.ID("peer-3"))

	network.AddHost(host1)
	network.AddHost(host2)
	network.AddHost(host3)

	t.Run("ConnectAll", func(t *testing.T) {
		err := network.ConnectAll()
		require.NoError(t, err)

		assert.True(t, network.IsConnected(host1.ID(), host2.ID()))
		assert.True(t, network.IsConnected(host2.ID(), host3.ID()))
		assert.True(t, network.IsConnected(host1.ID(), host3.ID()))
	})

	t.Run("Partition", func(t *testing.T) {
		// Partition host3
		network.Partition([]peer.ID{host3.ID()})

		// host1 and host2 should be in same partition
		assert.Equal(t, network.GetPartition(host1.ID()), network.GetPartition(host2.ID()))

		// host3 should be in different partition
		assert.NotEqual(t, network.GetPartition(host1.ID()), network.GetPartition(host3.ID()))
	})

	t.Run("Heal", func(t *testing.T) {
		network.Heal()

		// All should be in same partition
		assert.Equal(t, 0, network.GetPartition(host1.ID()))
		assert.Equal(t, 0, network.GetPartition(host2.ID()))
		assert.Equal(t, 0, network.GetPartition(host3.ID()))
	})

	t.Run("PacketLoss", func(t *testing.T) {
		network.SetPacketLoss(0.5)
		network.ClearMessageLog()

		// Send many messages to test packet loss
		for i := 0; i < 100; i++ {
			_ = network.SendMessage(host1.ID(), host2.ID(), "/test", []byte("msg"))
		}

		// With 50% packet loss, some should be dropped
		dropped := network.DroppedMessageCount()
		assert.Greater(t, dropped, 0)
	})
}

func TestMockPubSub(t *testing.T) {
	registry := NewMockPubSubRegistry()

	ps1 := NewMockPubSubWithRegistry(peer.ID("peer-1"), registry)
	ps2 := NewMockPubSubWithRegistry(peer.ID("peer-2"), registry)

	t.Run("JoinTopic", func(t *testing.T) {
		topic1, err := ps1.Join("test-topic")
		require.NoError(t, err)
		require.NotNil(t, topic1)

		topic2, err := ps2.Join("test-topic")
		require.NoError(t, err)
		require.NotNil(t, topic2)
	})

	t.Run("Subscribe", func(t *testing.T) {
		topic1, _ := ps1.Join("test-topic")
		sub, err := topic1.Subscribe()
		require.NoError(t, err)
		require.NotNil(t, sub)
	})

	t.Run("PublishAndReceive", func(t *testing.T) {
		topic1, _ := ps1.Join("cross-test")
		topic2, _ := ps2.Join("cross-test")

		sub2, err := topic2.Subscribe()
		require.NoError(t, err)

		// Give subscription time to setup
		time.Sleep(10 * time.Millisecond)

		// Publish from ps1
		err = topic1.Publish(context.Background(), []byte("hello"))
		require.NoError(t, err)

		// Receive on ps2
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		msg, err := sub2.Next(ctx)
		require.NoError(t, err)
		assert.Equal(t, []byte("hello"), msg.Data)
	})
}

func TestTestCluster(t *testing.T) {
	cluster := NewTestCluster(t, 4)
	defer cluster.Stop()

	t.Run("NodeCount", func(t *testing.T) {
		assert.Equal(t, 4, cluster.NumNodes())
	})

	t.Run("AllConnected", func(t *testing.T) {
		for i := 0; i < 4; i++ {
			for j := i + 1; j < 4; j++ {
				nodeI := cluster.GetNode(i)
				nodeJ := cluster.GetNode(j)
				assert.True(t, cluster.Network.IsConnected(nodeI.PeerID, nodeJ.PeerID))
			}
		}
	})

	t.Run("Committee", func(t *testing.T) {
		assert.NotNil(t, cluster.Committee)
		assert.Equal(t, 4, cluster.Committee.Len())
	})

	t.Run("StopNode", func(t *testing.T) {
		cluster.StopNode(0)
		assert.True(t, cluster.GetNode(0).IsStopped())
		assert.Equal(t, 3, cluster.ActiveNodeCount())
	})

	t.Run("PartitionNode", func(t *testing.T) {
		cluster.PartitionNode(1)
		assert.NotEqual(t, 0, cluster.Network.GetPartition(cluster.GetNode(1).PeerID))
	})

	t.Run("HealPartition", func(t *testing.T) {
		cluster.HealPartition()
		assert.Equal(t, 0, cluster.Network.GetPartition(cluster.GetNode(1).PeerID))
	})
}

func TestMockExecutor(t *testing.T) {
	executor := NewMockExecutor()

	t.Run("LastBlock", func(t *testing.T) {
		params, hash, err := executor.LastBlock()
		require.NoError(t, err)
		assert.Equal(t, uint64(0), params.Index)
		assert.Equal(t, [32]byte{}, hash)
	})

	t.Run("Begin", func(t *testing.T) {
		params := MockBlockParams(1)
		block, err := executor.Begin(params)
		require.NoError(t, err)
		require.NotNil(t, block)

		assert.Equal(t, uint64(1), block.Params().Index)
	})

	t.Run("BlockCount", func(t *testing.T) {
		assert.Equal(t, 1, executor.BlockCount())
	})

	t.Run("OnBeginHook", func(t *testing.T) {
		called := false
		executor.OnBegin = func(params execute.BlockParams) error {
			called = true
			return nil
		}

		_, err := executor.Begin(MockBlockParams(2))
		require.NoError(t, err)
		assert.True(t, called)
	})
}

func TestGenerateTestCommittee(t *testing.T) {
	committee, keys := GenerateTestCommittee(4)

	assert.Equal(t, 4, committee.Len())
	assert.Equal(t, 4, len(keys))

	// Verify all keys are different
	for i := 0; i < 4; i++ {
		for j := i + 1; j < 4; j++ {
			assert.NotEqual(t, keys[i], keys[j])
		}
	}

	// Verify committee is valid
	err := committee.Validate()
	require.NoError(t, err)
}

func TestGenerateTestCommitteeWithStakes(t *testing.T) {
	stakes := []uint64{100, 200, 300, 400}
	committee, keys := GenerateTestCommitteeWithStakes(stakes)

	assert.Equal(t, 4, committee.Len())
	assert.Equal(t, 4, len(keys))

	// Verify stakes
	totalStake := uint64(0)
	for _, v := range committee.Validators {
		totalStake += v.Stake
	}
	assert.Equal(t, uint64(1000), totalStake)
}
