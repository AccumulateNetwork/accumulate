// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package tendermint

import (
	"context"
	"fmt"
	"testing"

	"github.com/cometbft/cometbft/rpc/client"
	"github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/stretchr/testify/require"
)

func TestWalkPeersMainNet(t *testing.T) {
	t.Skip("Manual")

	addr := "http://apollo-mainnet.accumulate.defidevs.io:16592"
	c, err := http.New(addr, addr+"/ws")
	require.NoError(t, err)

	t.Run("All", func(t *testing.T) {
		WalkPeers(context.Background(), c, func(ctx context.Context, peer coretypes.Peer) (client.NetworkClient, bool) {
			fmt.Println("Peer", peer.NodeInfo.ID(), peer.NodeInfo.Moniker, peer.RemoteIP)
			c, err := NewHTTPClient(ctx, peer, 0)
			require.NoError(t, err)
			return c, true
		})
	})

	t.Run("Stop early", func(t *testing.T) {
		var count int
		WalkPeers(context.Background(), c, func(ctx context.Context, peer coretypes.Peer) (client.NetworkClient, bool) {
			count++
			if count > 10 {
				return nil, false
			}

			fmt.Println("Peer", peer.NodeInfo.ID(), peer.NodeInfo.Moniker, peer.RemoteIP)
			c, err := NewHTTPClient(ctx, peer, 0)
			require.NoError(t, err)
			return c, true
		})
	})
}
