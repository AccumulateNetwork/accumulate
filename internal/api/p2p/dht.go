// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"sync"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func (n *Node) startDHT(ctx context.Context, mode dht.ModeOpt, bootstrapPeers []multiaddr.Multiaddr) (*dht.IpfsDHT, error) {
	// Allocate a DHT
	d, err := dht.New(ctx, n.host, dht.Mode(mode))
	if err != nil {
		return nil, errors.UnknownError.WithFormat("create DHT: %w", err)
	}

	// Connect to the bootstrap peers
	wg := new(sync.WaitGroup)
	for _, addr := range bootstrapPeers {
		pi, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("parse address: %w", err)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			err := n.host.Connect(ctx, *pi)
			if err != nil {
				n.logger.Info("Unable to connect to bootstrap peer", "error", err)
			}
		}()
	}
	wg.Wait()

	// Bootstrap the DHT?
	err = d.Bootstrap(ctx)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("bootstrap DHT: %w", err)
	}

	return d, nil
}
