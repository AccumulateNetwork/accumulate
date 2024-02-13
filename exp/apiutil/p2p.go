// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package apiutil

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p/dial"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
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

// InitRouter initializes a router. If an event bus is provided, InitRouter will
// use that. Otherwise, InitRouter will use the node to query the network to
// determine the initial routing table.
func InitRouter(opts RouterOptions) (routing.Router, error) {
	// Use the event bus if provided
	if opts.Events != nil {
		return routing.NewRouter(routing.RouterOptions{
			Events: opts.Events,
			Logger: opts.Logger,
		}), nil
	}

	// Create a new event bus and fetch the routing table asynchronously
	opts.Events = events.NewBus(opts.Logger)
	router := routing.NewRouter(routing.RouterOptions{
		Events: opts.Events,
		Logger: opts.Logger,
	})
	go initRouter(opts)

	return router, nil
}

func initRouter(opts RouterOptions) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorCtx(opts.Context, "Panicked while initializing router", "error", r, "stack", debug.Stack())
		}
	}()

	// Address of the network service for the directory partition
	dirNetSvc := api.ServiceTypeNetwork.AddressFor(protocol.Directory)

	// Verify the network service is running
	tr, ok := opts.Node.Tracker().(*dial.PersistentTracker)
	if !ok {
		// If we're not using a persistent tracker, wait for the service
		slog.InfoCtx(opts.Context, "Waiting for a live network service")
		svcAddr, err := dirNetSvc.MultiaddrFor(opts.Network)
		if err != nil {
			slog.ErrorCtx(opts.Context, "Failed to initialize router", "error", err)
			return
		}

		err = opts.Node.WaitForService(opts.Context, svcAddr)
		if err != nil {
			slog.ErrorCtx(opts.Context, "Failed to initialize router", "error", err)
			return
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
			slog.InfoCtx(opts.Context, "Scanning for peers")
			tr.ScanPeers(5 * time.Minute)
		}
	}

	slog.InfoCtx(opts.Context, "Fetching routing information")
	client := &message.Client{
		Transport: &message.RoutedTransport{
			Network: opts.Network,
			Dialer:  opts.Node.DialNetwork(),
			Router:  new(routing.MessageRouter),
		},
	}

	ns, err := client.NetworkStatus(opts.Context, api.NetworkStatusOptions{})
	if err != nil {
		slog.ErrorCtx(opts.Context, "Failed to initialize router", "error", err)
		return
	}

	err = opts.Events.Publish(events.WillChangeGlobals{
		New: &network.GlobalValues{
			Oracle:          ns.Oracle,
			Globals:         ns.Globals,
			Network:         ns.Network,
			Routing:         ns.Routing,
			ExecutorVersion: ns.ExecutorVersion,
		},
	})
	if err != nil {
		slog.ErrorCtx(opts.Context, "Failed to initialize router", "error", err)
		return
	}
}
