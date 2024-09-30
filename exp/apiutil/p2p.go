// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package apiutil

import (
	"context"
	"log/slog"
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
)

type RouterOptions struct {
	Context context.Context
	Node    *p2p.Node
	Network string
	Events  *events.Bus
	Logger  log.Logger
	Dialer  message.Dialer
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
			slog.ErrorContext(opts.Context, "Panicked while initializing router", "error", r, "stack", debug.Stack())
		}
	}()

	// Address of the network service for the directory partition
	dirNetSvc := api.ServiceTypeNetwork.AddressFor(protocol.Directory)

	// Verify the network service is running
	tr, ok := opts.Node.Tracker().(*dial.PersistentTracker)
	if !ok {
		// If we're not using a persistent tracker, wait for the service
		slog.InfoContext(opts.Context, "Waiting for a live network service")
		svcAddr, err := dirNetSvc.MultiaddrFor(opts.Network)
		if err != nil {
			slog.ErrorContext(opts.Context, "Failed to initialize router (1)", "error", err)
			_ = opts.Events.Publish(events.WillChangeGlobals{New: &network.GlobalValues{}})
			return
		}

		err = opts.Node.WaitForService(opts.Context, svcAddr)
		if err != nil {
			slog.ErrorContext(opts.Context, "Failed to initialize router (2)", "error", err)
			_ = opts.Events.Publish(events.WillChangeGlobals{New: &network.GlobalValues{}})
			return
		}

	} else {
		// Check if we know of a suitable peer
		var found bool
		for _, peer := range tr.DB().Peers.Load() {
			s := peer.Network(opts.Network).Service(dirNetSvc)
			if s.Last.Success == nil {
				continue
			}
			if tr.SuccessIsTooOld(s.Last) {
				continue
			}
			found = true
		}

		// If not then scan the network (synchronously)
		if !found {
			slog.InfoContext(opts.Context, "Scanning for peers")
			time.Sleep(time.Minute) // Give the DHT time
			tr.ScanPeers(5 * time.Minute)
		}
	}

	if opts.Dialer == nil {
		opts.Dialer = opts.Node.DialNetwork()
	}

	slog.InfoContext(opts.Context, "Fetching routing information")
	client := &message.Client{
		Transport: &message.RoutedTransport{
			Network: opts.Network,
			Dialer:  opts.Dialer,
			Router:  new(routing.MessageRouter),
		},
	}

	ns, err := client.NetworkStatus(opts.Context, api.NetworkStatusOptions{})
	if err != nil {
		slog.ErrorContext(opts.Context, "Failed to initialize router (3)", "error", err)
		_ = opts.Events.Publish(events.WillChangeGlobals{New: &network.GlobalValues{}})
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
		slog.ErrorContext(opts.Context, "Failed to initialize router (4)", "error", err)
		return
	}
}
