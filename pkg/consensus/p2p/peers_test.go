// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/testutil"
)

func TestNewPeerManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := testutil.NewMockHost(peer.ID("test-host"))
	pm := NewPeerManager(ctx, host)

	require.NotNil(t, pm)
}

func TestPeerManager_Peers_Empty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := testutil.NewMockHost(peer.ID("test-host"))
	pm := NewPeerManager(ctx, host)

	peers := pm.Peers()
	assert.Len(t, peers, 0)
	assert.Equal(t, 0, pm.NumPeers())
}

func TestPeerManager_Bootstrap_Empty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := testutil.NewMockHost(peer.ID("test-host"))
	pm := NewPeerManager(ctx, host)

	// Bootstrap with empty list should succeed
	err := pm.Bootstrap(ctx, []peer.AddrInfo{})
	require.NoError(t, err)
}

func TestPeerManager_Bootstrap_SkipSelf(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := testutil.NewMockHost(peer.ID("test-host"))
	pm := NewPeerManager(ctx, host)

	// Bootstrap with self should be skipped
	err := pm.Bootstrap(ctx, []peer.AddrInfo{
		{ID: peer.ID("test-host")},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, pm.NumPeers())
}

func TestPeerManager_PartitionPeers_Empty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := testutil.NewMockHost(peer.ID("test-host"))
	pm := NewPeerManager(ctx, host)

	// No partition peers initially
	peers := pm.PartitionPeers("bvn0")
	assert.Nil(t, peers)
}

func TestPeerManager_RegisterPartitionPeer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := testutil.NewMockHost(peer.ID("test-host"))
	pm := NewPeerManager(ctx, host).(*peerManager)

	peer1 := peer.ID("peer-1")
	peer2 := peer.ID("peer-2")

	// Register peers
	pm.RegisterPartitionPeer("bvn0", peer1)
	pm.RegisterPartitionPeer("bvn0", peer2)
	pm.RegisterPartitionPeer("bvn1", peer1)

	// Note: PartitionPeers checks if peers are connected, so they won't show up
	// unless we connect them. Let's verify the internal map instead.
	pm.mu.RLock()
	assert.Len(t, pm.partitionPeers["bvn0"], 2)
	assert.Len(t, pm.partitionPeers["bvn1"], 1)
	pm.mu.RUnlock()
}

func TestPeerManager_UnregisterPartitionPeer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := testutil.NewMockHost(peer.ID("test-host"))
	pm := NewPeerManager(ctx, host).(*peerManager)

	peer1 := peer.ID("peer-1")
	peer2 := peer.ID("peer-2")

	// Register peers
	pm.RegisterPartitionPeer("bvn0", peer1)
	pm.RegisterPartitionPeer("bvn0", peer2)

	// Unregister one
	pm.UnregisterPartitionPeer("bvn0", peer1)

	pm.mu.RLock()
	assert.Len(t, pm.partitionPeers["bvn0"], 1)
	pm.mu.RUnlock()

	// Unregister from non-existent partition (should not panic)
	pm.UnregisterPartitionPeer("nonexistent", peer1)
}

func TestPeerManager_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := testutil.NewMockHost(peer.ID("test-host"))
	pm := NewPeerManager(ctx, host)

	err := pm.Close()
	require.NoError(t, err)
}

// Note: GetPeerInfo and GetAllPeerInfo require a real Peerstore which MockHost doesn't provide.
// These methods are tested via integration tests instead.

func TestPeerInfo_Structure(t *testing.T) {
	info := PeerInfo{
		ID:        peer.ID("test-peer"),
		Addrs:     []string{"/ip4/127.0.0.1/tcp/9000"},
		Partition: "bvn0",
		Connected: true,
		Latency:   100 * time.Millisecond,
	}

	assert.Equal(t, peer.ID("test-peer"), info.ID)
	assert.Len(t, info.Addrs, 1)
	assert.Equal(t, "bvn0", info.Partition)
	assert.True(t, info.Connected)
	assert.Equal(t, 100*time.Millisecond, info.Latency)
}

func TestPeerManager_ConcurrentAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := testutil.NewMockHost(peer.ID("test-host"))
	pm := NewPeerManager(ctx, host).(*peerManager)

	// Concurrent register/unregister operations
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		peerID := peer.ID("peer-" + string(rune('0'+i)))

		go func(pid peer.ID) {
			defer wg.Done()
			pm.RegisterPartitionPeer("bvn0", pid)
		}(peerID)

		go func(pid peer.ID) {
			defer wg.Done()
			pm.UnregisterPartitionPeer("bvn0", pid)
		}(peerID)
	}
	wg.Wait()
}
