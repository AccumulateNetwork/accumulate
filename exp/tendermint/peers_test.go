// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tendermint

import (
	"context"
	"fmt"
	"testing"

	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	testutil "gitlab.com/accumulatenetwork/accumulate/test/util"
)

func TestWalkPeersMainNet(t *testing.T) {
	t.Skip("Manual")

	addr := "http://apollo-mainnet.accumulate.defidevs.io:16592"
	c, err := http.New(addr, addr+"/ws")
	require.NoError(t, err)

	t.Run("All", func(t *testing.T) {
		WalkPeers(context.Background(), c, func(ctx context.Context, peer coretypes.Peer) (WalkClient, bool) {
			fmt.Println("Peer", peer.NodeInfo.ID(), peer.NodeInfo.Moniker, peer.RemoteIP)
			c, err := NewHTTPClientForPeer(peer, 0)
			require.NoError(t, err)
			return c, true
		})
	})

	t.Run("Stop early", func(t *testing.T) {
		var count int
		WalkPeers(context.Background(), c, func(ctx context.Context, peer coretypes.Peer) (WalkClient, bool) {
			count++
			if count > 10 {
				return nil, false
			}

			fmt.Println("Peer", peer.NodeInfo.ID(), peer.NodeInfo.Moniker, peer.RemoteIP)
			c, err := NewHTTPClientForPeer(peer, 0)
			require.NoError(t, err)
			return c, true
		})
	})
}

func TestWalkPeers(t *testing.T) {
	// Setup peers
	alice := coretypes.Peer{
		RemoteIP: "alice",
		NodeInfo: p2p.DefaultNodeInfo{
			DefaultNodeID: p2p.ID("alice"),
		},
	}
	bob := coretypes.Peer{
		RemoteIP: "bob",
		NodeInfo: p2p.DefaultNodeInfo{
			DefaultNodeID: p2p.ID("bob"),
		},
	}
	charlie := coretypes.Peer{
		RemoteIP: "charlie",
		NodeInfo: p2p.DefaultNodeInfo{
			DefaultNodeID: p2p.ID("charlie"),
		},
	}

	t.Run("Take all", func(t *testing.T) {
		// Setup mock clients
		clients := map[p2p.ID]WalkClient{
			"alice":   &staticNetInfo{Peers: []coretypes.Peer{bob}},
			"bob":     &staticNetInfo{Peers: []coretypes.Peer{charlie}},
			"charlie": &staticNetInfo{Peers: []coretypes.Peer{alice}},
		}

		// Walk
		var seen []p2p.ID
		ctx := testutil.ContextForTest(t)
		testutil.TrackGoroutines(ctx, t, func(ctx context.Context) {
			WalkPeers(ctx, clients["alice"], func(ctx context.Context, p coretypes.Peer) (WalkClient, bool) {
				seen = append(seen, p.NodeInfo.ID())
				c, ok := clients[p.NodeInfo.ID()]
				require.True(t, ok, "unknown peer")
				return c, true
			})
		})

		// Verify
		assert.Len(t, seen, 3)
		assert.Contains(t, seen, p2p.ID("alice"))
		assert.Contains(t, seen, p2p.ID("bob"))
		assert.Contains(t, seen, p2p.ID("charlie"))
	})

	t.Run("Take 1", func(t *testing.T) {
		// Setup mock clients
		clients := map[p2p.ID]WalkClient{
			"alice":   &staticNetInfo{Peers: []coretypes.Peer{bob}},
			"bob":     &staticNetInfo{Peers: []coretypes.Peer{charlie}},
			"charlie": &staticNetInfo{Peers: []coretypes.Peer{alice}},
		}

		// Walk
		var seen []p2p.ID
		ctx := testutil.ContextForTest(t)
		testutil.TrackGoroutines(ctx, t, func(ctx context.Context) {
			WalkPeers(ctx, clients["alice"], func(ctx context.Context, p coretypes.Peer) (WalkClient, bool) {
				seen = append(seen, p.NodeInfo.ID())
				c, ok := clients[p.NodeInfo.ID()]
				require.True(t, ok, "unknown peer")
				return c, false
			})
		})

		// Verify
		assert.Len(t, seen, 1)
		assert.Contains(t, seen, p2p.ID("bob"))
	})

	t.Run("Take 2", func(t *testing.T) {
		// Setup mock clients
		clients := map[p2p.ID]WalkClient{
			"alice":   &staticNetInfo{Peers: []coretypes.Peer{bob}},
			"bob":     &staticNetInfo{Peers: []coretypes.Peer{charlie}},
			"charlie": &staticNetInfo{Peers: []coretypes.Peer{alice}},
		}

		// Walk
		var seen []p2p.ID
		ctx := testutil.ContextForTest(t)
		testutil.TrackGoroutines(ctx, t, func(ctx context.Context) {
			var count int
			WalkPeers(ctx, clients["alice"], func(ctx context.Context, p coretypes.Peer) (WalkClient, bool) {
				count++
				seen = append(seen, p.NodeInfo.ID())
				c, ok := clients[p.NodeInfo.ID()]
				require.True(t, ok, "unknown peer")
				return c, count < 2
			})
		})

		// Verify
		assert.Len(t, seen, 2)
		assert.Contains(t, seen, p2p.ID("bob"))
		assert.Contains(t, seen, p2p.ID("charlie"))
	})

	t.Run("Full peering", func(t *testing.T) {
		// Setup mock clients
		clients := map[p2p.ID]WalkClient{
			"alice":   &staticNetInfo{Peers: []coretypes.Peer{bob, charlie}},
			"bob":     &staticNetInfo{Peers: []coretypes.Peer{alice, charlie}},
			"charlie": &staticNetInfo{Peers: []coretypes.Peer{alice, bob}},
		}

		// Walk
		var seen []p2p.ID
		ctx := testutil.ContextForTest(t)
		testutil.TrackGoroutines(ctx, t, func(ctx context.Context) {
			WalkPeers(ctx, clients["alice"], func(ctx context.Context, p coretypes.Peer) (WalkClient, bool) {
				seen = append(seen, p.NodeInfo.ID())
				c, ok := clients[p.NodeInfo.ID()]
				require.True(t, ok, "unknown peer")
				return c, true
			})
		})

		// Verify
		assert.Len(t, seen, 3)
		assert.Contains(t, seen, p2p.ID("alice"))
		assert.Contains(t, seen, p2p.ID("bob"))
		assert.Contains(t, seen, p2p.ID("charlie"))
	})
}

func BenchmarkWalkPeers(b *testing.B) {
	// Setup peers
	alice := coretypes.Peer{
		RemoteIP: "alice",
		NodeInfo: p2p.DefaultNodeInfo{
			DefaultNodeID: p2p.ID("alice"),
		},
	}
	bob := coretypes.Peer{
		RemoteIP: "bob",
		NodeInfo: p2p.DefaultNodeInfo{
			DefaultNodeID: p2p.ID("bob"),
		},
	}
	charlie := coretypes.Peer{
		RemoteIP: "charlie",
		NodeInfo: p2p.DefaultNodeInfo{
			DefaultNodeID: p2p.ID("charlie"),
		},
	}

	// Setup mock clients
	clients := map[p2p.ID]WalkClient{
		"alice":   &staticNetInfo{Peers: []coretypes.Peer{bob}},
		"bob":     &staticNetInfo{Peers: []coretypes.Peer{charlie}},
		"charlie": &staticNetInfo{Peers: []coretypes.Peer{alice}},
	}

	// Walk
	ctx := testutil.ContextForTest(b)
	for i := 0; i < b.N; i++ {
		WalkPeers(ctx, clients["alice"], func(ctx context.Context, p coretypes.Peer) (WalkClient, bool) {
			c, ok := clients[p.NodeInfo.ID()]
			require.True(b, ok, "unknown peer")
			return c, true
		})
	}
}

type staticNetInfo coretypes.ResultNetInfo

func (s *staticNetInfo) NetInfo(context.Context) (*coretypes.ResultNetInfo, error) {
	return (*coretypes.ResultNetInfo)(s), nil
}
