// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package apiutil

import (
	"context"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p/dial"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"golang.org/x/exp/slog"
)

type RouterOptions struct {
	Context context.Context
	Node    *p2p.Node
	Network string
	Events  *events.Bus
	Logger  log.Logger
}

func InitRouter(opts RouterOptions) (routing.Router, error) {
	ctx, node := opts.Context, opts.Node

	// Address of the network service for the directory partition
	dirNetSvc := api.ServiceTypeNetwork.AddressFor(protocol.Directory)

	// Verify the network service is running
	tr, ok := node.Tracker().(*dial.PersistentTracker)
	if !ok {
		// If we're not using a persistent tracker, wait for the service
		slog.InfoCtx(ctx, "Waiting for a live network service")
		svcAddr, err := dirNetSvc.MultiaddrFor(opts.Network)
		if err != nil {
			return nil, err
		}

		err = node.WaitForService(ctx, svcAddr)
		if err != nil {
			return nil, err
		}

	} else {
		// Check if we know of a suitable peer
		var found bool
		for _, peer := range tr.DB().Peers.Load() {
			if peer.Network(opts.Network).Service(dirNetSvc).Last.Success != nil {
				found = true
			}
		}

		// If not then scan the network (synchronously)
		if !found {
			slog.InfoCtx(ctx, "Scanning for peers")
			tr.ScanPeers(5 * time.Minute)
		}
	}

	slog.InfoCtx(ctx, "Fetching routing information")
	client := &message.Client{
		Transport: &message.RoutedTransport{
			Network: opts.Network,
			Dialer:  node.DialNetwork(),
			Router:  new(routing.MessageRouter),
		},
	}

	ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	if err != nil {
		return nil, err
	}

	return routing.NewRouter(routing.RouterOptions{
		Initial: ns.Routing,
		Events:  opts.Events,
		Logger:  opts.Logger,
	}), nil
}
