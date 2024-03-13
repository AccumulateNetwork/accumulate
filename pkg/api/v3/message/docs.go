// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package message defines message types and implements binary message transport
// for Accumulate API v3.
//
// # Message Routing
//
// [RoutedTransport] can be configured to route messages to the appropriate
// network. However, some queries cannot be routed without a routing table. The
// following is an example of how the message client can be initialized with a
// routing table:
//
//	router := new(routing.MessageRouter)
//	transport := &message.RoutedTransport{
//	    Network: "MainNet",
//	    Dialer:  node.DialNetwork(),
//	    Router:  router,
//	}
//	client := &message.Client{Transport: transport}
//
//	ns, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
//	if err != nil { panic(err) }
//
//	router.Router, err = routing.NewStaticRouter(ns.Routing, nil, nil)
//	if err != nil { panic(err) }
package message
