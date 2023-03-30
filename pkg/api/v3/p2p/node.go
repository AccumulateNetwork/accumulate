// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ClientNode is a peer to peer node. ClientNode acts as a client, routing messages to its
// peers, and as a server that can register services to the network.
type ClientNode struct {
	*Node
	client *message.Client
}

// NewClient creates a new node. NewClient waits for the Network service of the Directory
// network to be available and queries it to determine the routing table. Thus
// NewClient must not be used by the core nodes themselves.
func NewClient(opts Options) (*ClientNode, error) {
	if opts.Network == "" {
		return nil, errors.BadRequest.With("missing network")
	}

	node, err := New(opts)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize node: %w", err)
	}

	return NewClientWith(node)
}

// NewClientWith creates a new client for the given node. NewClientWith waits
// for the Network service of the Directory network to be available and queries
// it to determine the routing table. Thus NewClientWith must not be used by the
// core nodes themselves.
func NewClientWith(node *Node) (*ClientNode, error) {
	// Wait for the directory service
	dnAddr, err := api.ServiceTypeNetwork.AddressFor(protocol.Directory).MultiaddrFor(node.peermgr.network)
	if err != nil {
		return nil, err
	}
	err = node.WaitForService(context.Background(), dnAddr)
	if err != nil {
		return nil, err
	}

	// Query the network status
	mr := new(routing.MessageRouter)
	client := &message.Client{Transport: &message.RoutedTransport{
		Network: node.peermgr.network,
		Dialer:  node.DialNetwork(),
		Router:  mr,
	}}
	ns, err := client.NetworkStatus(context.Background(), api.NetworkStatusOptions{})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("query network status: %w", err)
	}

	// Create a router
	mr.Router, err = routing.NewStaticRouter(ns.Routing, nil)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("initialize router: %w", err)
	}

	return &ClientNode{node, client}, nil
}

var _ api.NodeService = (*ClientNode)(nil)
var _ api.ConsensusService = (*ClientNode)(nil)
var _ api.NetworkService = (*ClientNode)(nil)
var _ api.MetricsService = (*ClientNode)(nil)
var _ api.Querier = (*ClientNode)(nil)
var _ api.Submitter = (*ClientNode)(nil)
var _ api.Validator = (*ClientNode)(nil)
var _ api.Faucet = (*ClientNode)(nil)

// NodeInfo implements [api.NodeService.NodeInfo].
func (n *ClientNode) NodeInfo(ctx context.Context, opts api.NodeInfoOptions) (*api.NodeInfo, error) {
	return n.client.NodeInfo(ctx, opts)
}

// FindService implements [api.NodeService.FindService].
func (n *ClientNode) FindService(ctx context.Context, opts api.FindServiceOptions) ([]*api.FindServiceResult, error) {
	return n.client.FindService(ctx, opts)
}

// ConsensusStatus implements [api.ConsensusService.ConsensusStatus].
func (n *ClientNode) ConsensusStatus(ctx context.Context, opts api.ConsensusStatusOptions) (*api.ConsensusStatus, error) {
	return n.client.ConsensusStatus(ctx, opts)
}

// NetworkStatus implements [api.NetworkService.NetworkStatus].
func (n *ClientNode) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	return n.client.NetworkStatus(ctx, opts)
}

// Metrics implements [api.MetricsService.Metrics].
func (n *ClientNode) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	return n.client.Metrics(ctx, opts)
}

// Query implements [api.Querier.Query].
func (n *ClientNode) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	return n.client.Query(ctx, scope, query)
}

// Submit implements [api.Submitter.Submit].
func (n *ClientNode) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	return n.client.Submit(ctx, envelope, opts)
}

// Validate implements [api.Validator.Validate].
func (n *ClientNode) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	return n.client.Validate(ctx, envelope, opts)
}

// Faucet implements [api.Faucet.Faucet].
func (n *ClientNode) Faucet(ctx context.Context, account *url.URL, opts api.FaucetOptions) (*api.Submission, error) {
	return n.client.Faucet(ctx, account, opts)
}

// Subscribe implements [api.EventService.Subscribe].
func (n *ClientNode) Subscribe(ctx context.Context, opts api.SubscribeOptions) (<-chan api.Event, error) {
	return n.client.Subscribe(ctx, opts)
}
