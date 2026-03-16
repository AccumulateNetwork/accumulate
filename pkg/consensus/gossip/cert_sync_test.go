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
	bhost "github.com/libp2p/go-libp2p/p2p/host/blank"
	swarmt "github.com/libp2p/go-libp2p/p2p/net/swarm/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/gossip"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/types"
)

func TestCertSyncRequest_MarshalUnmarshal(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	digests := make([]types.CertificateDigest, 3)
	for i := range digests {
		rand.Read(digests[i][:])
	}

	req := &gossip.CertSyncRequest{
		Digests:   digests,
		Requester: pub,
		RequestID: 12345,
	}

	data, err := req.Marshal()
	require.NoError(t, err)

	req2, err := gossip.UnmarshalCertSyncRequest(data)
	require.NoError(t, err)

	assert.Equal(t, req.RequestID, req2.RequestID)
	assert.Equal(t, req.Requester, req2.Requester)
	require.Equal(t, len(req.Digests), len(req2.Digests))
	for i := range req.Digests {
		assert.Equal(t, req.Digests[i], req2.Digests[i])
	}
}

func TestCertSyncResponse_MarshalUnmarshal(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	header := types.NewHeader(pub, 1, 0, nil, nil)
	err = header.Sign(priv)
	require.NoError(t, err)

	cert := types.NewCertificate(*header, [][]byte{header.Signature}, []uint16{0})

	missing := make([]types.CertificateDigest, 2)
	for i := range missing {
		rand.Read(missing[i][:])
	}

	resp := &gossip.CertSyncResponse{
		Certificates: []*types.Certificate{cert},
		RequestID:    67890,
		Missing:      missing,
	}

	data, err := resp.Marshal()
	require.NoError(t, err)

	resp2, err := gossip.UnmarshalCertSyncResponse(data)
	require.NoError(t, err)

	assert.Equal(t, resp.RequestID, resp2.RequestID)
	require.Equal(t, len(resp.Certificates), len(resp2.Certificates))
	assert.Equal(t, resp.Certificates[0].Digest(), resp2.Certificates[0].Digest())
	require.Equal(t, len(resp.Missing), len(resp2.Missing))
	for i := range resp.Missing {
		assert.Equal(t, resp.Missing[i], resp2.Missing[i])
	}
}

func TestCertSyncRequest_TooManyDigests(t *testing.T) {
	digests := make([]types.CertificateDigest, gossip.MaxSyncDigests+1)

	req := &gossip.CertSyncRequest{
		Digests:   digests,
		Requester: make([]byte, 32),
		RequestID: 1,
	}

	_, err := req.Marshal()
	assert.Error(t, err)
}

func TestCertSyncResponse_TooManyCertificates(t *testing.T) {
	certs := make([]*types.Certificate, gossip.MaxSyncCertificates+1)

	resp := &gossip.CertSyncResponse{
		Certificates: certs,
		RequestID:    1,
	}

	_, err := resp.Marshal()
	assert.Error(t, err)
}

func TestCertSyncRequest_InvalidData(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		_, err := gossip.UnmarshalCertSyncRequest([]byte{1, 2, 3})
		assert.Error(t, err)
	})

	t.Run("truncated digests", func(t *testing.T) {
		data := make([]byte, 20)
		// Set a large digest count
		data[12] = 0
		data[13] = 0
		data[14] = 0
		data[15] = 100

		_, err := gossip.UnmarshalCertSyncRequest(data)
		assert.Error(t, err)
	})
}

func TestCertSyncResponse_InvalidData(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		_, err := gossip.UnmarshalCertSyncResponse([]byte{1, 2, 3})
		assert.Error(t, err)
	})
}

func TestGossipLayer_BroadcastAndReceiveSyncRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create network of 2 hosts
	netw1 := swarmt.GenSwarm(t)
	h1 := bhost.NewBlankHost(netw1)
	t.Cleanup(func() { h1.Close() })

	netw2 := swarmt.GenSwarm(t)
	h2 := bhost.NewBlankHost(netw2)
	t.Cleanup(func() { h2.Close() })

	// Connect them
	h1.Peerstore().AddAddrs(h2.ID(), h2.Addrs(), time.Hour)
	err := h1.Connect(ctx, h2.Peerstore().PeerInfo(h2.ID()))
	require.NoError(t, err)

	// Create pubsub
	ps1, err := pubsub.NewGossipSub(ctx, h1)
	require.NoError(t, err)

	ps2, err := pubsub.NewGossipSub(ctx, h2)
	require.NoError(t, err)

	// Create gossip layers
	gl1, err := gossip.NewGossipLayer(h1, ps1, "test")
	require.NoError(t, err)

	gl2, err := gossip.NewGossipLayer(h2, ps2, "test")
	require.NoError(t, err)

	err = gl1.Start(ctx)
	require.NoError(t, err)
	defer gl1.Close()

	err = gl2.Start(ctx)
	require.NoError(t, err)
	defer gl2.Close()

	// Wait for mesh
	time.Sleep(500 * time.Millisecond)

	// Create and send sync request
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	var digest types.CertificateDigest
	rand.Read(digest[:])

	req := &gossip.CertSyncRequest{
		Digests:   []types.CertificateDigest{digest},
		Requester: pub,
		RequestID: 999,
	}

	err = gl1.BroadcastSyncRequest(ctx, req)
	require.NoError(t, err)

	// Receive on gl2
	select {
	case received := <-gl2.SubscribeSyncRequests():
		assert.Equal(t, req.RequestID, received.RequestID)
		assert.Equal(t, len(req.Digests), len(received.Digests))
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for sync request")
	}
}

func TestGossipLayer_BroadcastAndReceiveSyncResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create network
	netw1 := swarmt.GenSwarm(t)
	h1 := bhost.NewBlankHost(netw1)
	t.Cleanup(func() { h1.Close() })

	netw2 := swarmt.GenSwarm(t)
	h2 := bhost.NewBlankHost(netw2)
	t.Cleanup(func() { h2.Close() })

	h1.Peerstore().AddAddrs(h2.ID(), h2.Addrs(), time.Hour)
	err := h1.Connect(ctx, h2.Peerstore().PeerInfo(h2.ID()))
	require.NoError(t, err)

	ps1, err := pubsub.NewGossipSub(ctx, h1)
	require.NoError(t, err)

	ps2, err := pubsub.NewGossipSub(ctx, h2)
	require.NoError(t, err)

	gl1, err := gossip.NewGossipLayer(h1, ps1, "test")
	require.NoError(t, err)

	gl2, err := gossip.NewGossipLayer(h2, ps2, "test")
	require.NoError(t, err)

	err = gl1.Start(ctx)
	require.NoError(t, err)
	defer gl1.Close()

	err = gl2.Start(ctx)
	require.NoError(t, err)
	defer gl2.Close()

	time.Sleep(500 * time.Millisecond)

	// Create certificate
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	header := types.NewHeader(pub, 1, 0, nil, nil)
	err = header.Sign(priv)
	require.NoError(t, err)

	cert := types.NewCertificate(*header, [][]byte{header.Signature}, []uint16{0})

	resp := &gossip.CertSyncResponse{
		Certificates: []*types.Certificate{cert},
		RequestID:    888,
	}

	err = gl1.BroadcastSyncResponse(ctx, resp)
	require.NoError(t, err)

	select {
	case received := <-gl2.SubscribeSyncResponses():
		assert.Equal(t, resp.RequestID, received.RequestID)
		require.Len(t, received.Certificates, 1)
		assert.Equal(t, cert.Digest(), received.Certificates[0].Digest())
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for sync response")
	}
}
