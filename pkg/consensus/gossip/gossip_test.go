// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package gossip_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	bhost "github.com/libp2p/go-libp2p/p2p/host/blank"
	swarmt "github.com/libp2p/go-libp2p/p2p/net/swarm/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

// testNetwork holds test hosts and their pubsub instances.
type testNetwork struct {
	t     *testing.T
	ctx   context.Context
	hosts []host.Host
	ps    []*pubsub.PubSub
}

// newTestNetwork creates a network of connected test hosts.
func newTestNetwork(t *testing.T, ctx context.Context, n int) *testNetwork {
	t.Helper()

	tn := &testNetwork{
		t:     t,
		ctx:   ctx,
		hosts: make([]host.Host, n),
		ps:    make([]*pubsub.PubSub, n),
	}

	// Create hosts
	for i := 0; i < n; i++ {
		netw := swarmt.GenSwarm(t)
		h := bhost.NewBlankHost(netw)
		tn.hosts[i] = h
		t.Cleanup(func() { h.Close() })
	}

	// Connect all hosts to each other
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			tn.connect(i, j)
		}
	}

	// Create pubsub instances
	for i := 0; i < n; i++ {
		ps, err := pubsub.NewGossipSub(ctx, tn.hosts[i])
		require.NoError(t, err)
		tn.ps[i] = ps
	}

	return tn
}

// connect establishes a connection between two hosts.
func (tn *testNetwork) connect(i, j int) {
	pi := peer.AddrInfo{
		ID:    tn.hosts[j].ID(),
		Addrs: tn.hosts[j].Addrs(),
	}
	err := tn.hosts[i].Connect(tn.ctx, pi)
	require.NoError(tn.t, err)
}

func TestTopicManager_NewTopicManager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tn := newTestNetwork(t, ctx, 1)

	t.Run("success", func(t *testing.T) {
		tm, err := gossip.NewTopicManager(tn.ps[0], "test-partition")
		require.NoError(t, err)
		assert.NotNil(t, tm)
		assert.Equal(t, "test-partition", tm.Partition())
	})

	t.Run("nil pubsub", func(t *testing.T) {
		_, err := gossip.NewTopicManager(nil, "test")
		assert.Error(t, err)
	})

	t.Run("empty partition", func(t *testing.T) {
		_, err := gossip.NewTopicManager(tn.ps[0], "")
		assert.Error(t, err)
	})
}

func TestTopicManager_JoinAll(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tn := newTestNetwork(t, ctx, 1)

	tm, err := gossip.NewTopicManager(tn.ps[0], "test-partition")
	require.NoError(t, err)

	err = tm.JoinAll(ctx)
	require.NoError(t, err)

	// Verify topics are joined
	assert.NotNil(t, tm.GetTopic(gossip.TopicBatches))
	assert.NotNil(t, tm.GetTopic(gossip.TopicHeaders))
	assert.NotNil(t, tm.GetTopic(gossip.TopicVotes))
	assert.NotNil(t, tm.GetTopic(gossip.TopicCerts))

	// Verify subscriptions are created
	assert.NotNil(t, tm.GetSubscription(gossip.TopicBatches))
	assert.NotNil(t, tm.GetSubscription(gossip.TopicHeaders))
	assert.NotNil(t, tm.GetSubscription(gossip.TopicVotes))
	assert.NotNil(t, tm.GetSubscription(gossip.TopicCerts))

	// Verify topic names
	assert.Equal(t, "acc/test-partition/consensus/batches", tm.TopicName(gossip.TopicBatches))
	assert.Equal(t, "acc/test-partition/consensus/headers", tm.TopicName(gossip.TopicHeaders))

	err = tm.Close()
	require.NoError(t, err)
}

func TestGossipLayer_NewGossipLayer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tn := newTestNetwork(t, ctx, 1)

	t.Run("success", func(t *testing.T) {
		gl, err := gossip.NewGossipLayer(tn.hosts[0], tn.ps[0], "test")
		require.NoError(t, err)
		assert.NotNil(t, gl)
		assert.Equal(t, "test", gl.Partition())
		assert.Equal(t, tn.hosts[0], gl.Host())
	})

	t.Run("nil host", func(t *testing.T) {
		_, err := gossip.NewGossipLayer(nil, tn.ps[0], "test")
		assert.Error(t, err)
	})

	t.Run("nil pubsub", func(t *testing.T) {
		_, err := gossip.NewGossipLayer(tn.hosts[0], nil, "test")
		assert.Error(t, err)
	})

	t.Run("empty partition", func(t *testing.T) {
		_, err := gossip.NewGossipLayer(tn.hosts[0], tn.ps[0], "")
		assert.Error(t, err)
	})
}

func TestGossipLayer_Lifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tn := newTestNetwork(t, ctx, 1)

	gl, err := gossip.NewGossipLayer(tn.hosts[0], tn.ps[0], "test")
	require.NoError(t, err)

	// Start
	err = gl.Start(ctx)
	require.NoError(t, err)

	// Can't start twice
	err = gl.Start(ctx)
	assert.Error(t, err)

	// Close
	err = gl.Close()
	require.NoError(t, err)

	// Close is idempotent
	err = gl.Close()
	require.NoError(t, err)
}

func TestGossipLayer_BroadcastAndReceiveBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tn := newTestNetwork(t, ctx, 2)

	// Create gossip layers
	gl1, err := gossip.NewGossipLayer(tn.hosts[0], tn.ps[0], "test")
	require.NoError(t, err)

	gl2, err := gossip.NewGossipLayer(tn.hosts[1], tn.ps[1], "test")
	require.NoError(t, err)

	// Start both
	err = gl1.Start(ctx)
	require.NoError(t, err)
	defer gl1.Close()

	err = gl2.Start(ctx)
	require.NoError(t, err)
	defer gl2.Close()

	// Wait for mesh to form
	time.Sleep(500 * time.Millisecond)

	// Create and broadcast a batch
	batch := types.NewBatch([][]byte{
		[]byte("transaction 1"),
		[]byte("transaction 2"),
	})

	err = gl1.BroadcastBatch(ctx, batch)
	require.NoError(t, err)

	// Receive on gl2
	select {
	case received := <-gl2.SubscribeBatches():
		assert.Equal(t, batch.Digest(), received.Digest())
		assert.Equal(t, len(batch.Transactions), len(received.Transactions))
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for batch")
	}
}

func TestGossipLayer_BroadcastAndReceiveHeader(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tn := newTestNetwork(t, ctx, 2)

	gl1, err := gossip.NewGossipLayer(tn.hosts[0], tn.ps[0], "test")
	require.NoError(t, err)

	gl2, err := gossip.NewGossipLayer(tn.hosts[1], tn.ps[1], "test")
	require.NoError(t, err)

	err = gl1.Start(ctx)
	require.NoError(t, err)
	defer gl1.Close()

	err = gl2.Start(ctx)
	require.NoError(t, err)
	defer gl2.Close()

	time.Sleep(500 * time.Millisecond)

	// Create a header
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	header := types.NewHeader(pub, 1, 0, nil, nil)
	err = header.Sign(priv)
	require.NoError(t, err)

	err = gl1.BroadcastHeader(ctx, header)
	require.NoError(t, err)

	select {
	case received := <-gl2.SubscribeHeaders():
		assert.Equal(t, header.Digest(), received.Digest())
		assert.Equal(t, header.Round, received.Round)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for header")
	}
}

func TestGossipLayer_BroadcastAndReceiveVote(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tn := newTestNetwork(t, ctx, 2)

	gl1, err := gossip.NewGossipLayer(tn.hosts[0], tn.ps[0], "test")
	require.NoError(t, err)

	gl2, err := gossip.NewGossipLayer(tn.hosts[1], tn.ps[1], "test")
	require.NoError(t, err)

	err = gl1.Start(ctx)
	require.NoError(t, err)
	defer gl1.Close()

	err = gl2.Start(ctx)
	require.NoError(t, err)
	defer gl2.Close()

	time.Sleep(500 * time.Millisecond)

	// Create a vote
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	var headerDigest types.HeaderDigest
	rand.Read(headerDigest[:])

	vote := types.NewVote(headerDigest, 5, 1, pub)
	err = vote.Sign(priv)
	require.NoError(t, err)

	err = gl1.BroadcastVote(ctx, vote)
	require.NoError(t, err)

	select {
	case received := <-gl2.SubscribeVotes():
		assert.Equal(t, vote.HeaderDigest, received.HeaderDigest)
		assert.Equal(t, vote.Round, received.Round)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for vote")
	}
}

func TestGossipLayer_BroadcastAndReceiveCertificate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tn := newTestNetwork(t, ctx, 2)

	gl1, err := gossip.NewGossipLayer(tn.hosts[0], tn.ps[0], "test")
	require.NoError(t, err)

	gl2, err := gossip.NewGossipLayer(tn.hosts[1], tn.ps[1], "test")
	require.NoError(t, err)

	err = gl1.Start(ctx)
	require.NoError(t, err)
	defer gl1.Close()

	err = gl2.Start(ctx)
	require.NoError(t, err)
	defer gl2.Close()

	time.Sleep(500 * time.Millisecond)

	// Create a certificate
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	header := types.NewHeader(pub, 1, 0, nil, nil)
	err = header.Sign(priv)
	require.NoError(t, err)

	cert := types.NewCertificate(header, [][]byte{header.Signature}, []uint16{0})

	err = gl1.BroadcastCertificate(ctx, cert)
	require.NoError(t, err)

	select {
	case received := <-gl2.SubscribeCertificates():
		assert.Equal(t, cert.Digest(), received.Digest())
		assert.Equal(t, cert.Round(), received.Round())
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for certificate")
	}
}

func TestGossipLayer_MultipleNodes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a network of 4 nodes
	tn := newTestNetwork(t, ctx, 4)

	// Create gossip layers for all nodes
	gls := make([]*gossip.GossipLayer, 4)
	for i := 0; i < 4; i++ {
		gl, err := gossip.NewGossipLayer(tn.hosts[i], tn.ps[i], "test")
		require.NoError(t, err)

		err = gl.Start(ctx)
		require.NoError(t, err)
		defer gl.Close()

		gls[i] = gl
	}

	// Wait for mesh to form
	time.Sleep(time.Second)

	// Broadcast from node 0
	batch := types.NewBatch([][]byte{[]byte("multi-node test")})
	err := gls[0].BroadcastBatch(ctx, batch)
	require.NoError(t, err)

	// All other nodes should receive it
	for i := 1; i < 4; i++ {
		select {
		case received := <-gls[i].SubscribeBatches():
			assert.Equal(t, batch.Digest(), received.Digest())
		case <-time.After(5 * time.Second):
			t.Fatalf("node %d: timeout waiting for batch", i)
		}
	}
}
