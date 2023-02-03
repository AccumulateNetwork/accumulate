// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/p2p"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// This package exists so the peer to peer implementation can be internal, but
// we still provide something for external parties to use.

// Node is a peer to peer node. Node acts as a client, routing messages to its
// peers, and as a server that can register services to the network.
type Node struct {
	client *message.Client
	node   *p2p.Node
}

// Options are the options for a [Node].
type Options = p2p.Options

// New creates a new node. New waits for the Network service of the Directory
// network to be available and queries it to determine the routing table. Thus
// New must not be used by the core nodes themselves.
func New(opts Options) (*Node, error) {
	node, err := p2p.New(opts)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize node: %w", err)
	}

	// Wait for the directory service
	node.WaitForService(&api.ServiceAddress{Type: api.ServiceTypeNetwork, Partition: protocol.Directory})

	// Query the network status
	client := &message.Client{Dialer: node.Dialer()}
	ns, err := client.GetNetInfo(context.Background())
	if err != nil {
		return nil, errors.UnknownError.WithFormat("query network status: %w", err)
	}

	// Create a router
	router, err := routing.NewStaticRouter(ns.Routing, nil, nil)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize router: %w", err)
	}
	client.Router = routing.MessageRouter{Router: router}

	return &Node{client, node}, nil
}

// Close shuts down the node.
func (n *Node) Close() error { return n.node.Close() }

// Addresses returns the advertized addresses of the node.
func (n *Node) Addresses() []multiaddr.Multiaddr { return n.node.Addrs() }

// WaitForService blocks until the given service is available. WaitForService
// will return once the service is registered on the current node or until the
// node is informed of a peer with the given service. WaitForService will return
// immediately if the service is already registered or known.
func (n *Node) WaitForService(sa *api.ServiceAddress) {
	n.node.WaitForService(sa)
}

// RegisterService registers a service handler and registers the service with
// the network.
func (n *Node) RegisterService(sa *api.ServiceAddress, handler func(message.Stream)) bool {
	return n.node.RegisterService(sa, handler)
}

var _ api.NodeService = (*Node)(nil)
var _ api.NetworkService = (*Node)(nil)
var _ api.MetricsService = (*Node)(nil)
var _ api.Querier = (*Node)(nil)
var _ api.Submitter = (*Node)(nil)
var _ api.Validator = (*Node)(nil)
var _ api.Faucet = (*Node)(nil)

// NodeStatus implements [api.NodeService.NodeStatus].
func (n *Node) NodeStatus(ctx context.Context, opts api.NodeStatusOptions) (*api.NodeStatus, error) {
	return n.client.NodeStatus(ctx, opts)
}

// NetworkStatus implements [api.NetworkService.NetworkStatus].
func (n *Node) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	return n.client.NetworkStatus(ctx, opts)
}

// Metrics implements [api.MetricsService.Metrics].
func (n *Node) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	return n.client.Metrics(ctx, opts)
}

// Query implements [api.Querier.Query].
func (n *Node) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	return n.client.Query(ctx, scope, query)
}

// Submit implements [api.Submitter.Submit].
func (n *Node) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	return n.client.Submit(ctx, envelope, opts)
}

// Validate implements [api.Validator.Validate].
func (n *Node) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	return n.client.Validate(ctx, envelope, opts)
}

// Faucet implements [api.Faucet.Faucet].
func (n *Node) Faucet(ctx context.Context, account *url.URL, opts api.FaucetOptions) (*api.Submission, error) {
	return n.client.Faucet(ctx, account, opts)
}

// Subscribe implements [api.EventService.Subscribe].
func (n *Node) Subscribe(ctx context.Context, opts api.SubscribeOptions) (<-chan api.Event, error) {
	return n.client.Subscribe(ctx, opts)
}
