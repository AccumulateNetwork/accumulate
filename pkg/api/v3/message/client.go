// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Client is a binary message client for API v3.
type Client struct {
	Transport Transport
}

type AddressedClient struct {
	*Client
	Address multiaddr.Multiaddr
}

// A Router determines the address a message should be routed to.
type Router interface {
	Route(Message) (multiaddr.Multiaddr, error)
}

// Ensure Client satisfies the service definitions.
var _ api.NodeService = (*Client)(nil)
var _ api.ConsensusService = (*Client)(nil)
var _ api.NetworkService = (*Client)(nil)
var _ api.MetricsService = (*Client)(nil)
var _ api.Querier = (*Client)(nil)
var _ api.Submitter = (*Client)(nil)
var _ api.Validator = (*Client)(nil)
var _ api.Faucet = (*Client)(nil)

func (c AddressedClient) ForAddress(addr multiaddr.Multiaddr) AddressedClient {
	if c.Address != nil {
		addr = c.Address.Encapsulate(addr)
	}
	return AddressedClient{c.Client, addr}
}

func (c AddressedClient) ForPeer(peer peer.ID) AddressedClient {
	addr, err := multiaddr.NewComponent("p2p", peer.String())
	if err != nil {
		panic(err)
	}
	return c.ForAddress(addr)
}

func (c *Client) ForAddress(addr multiaddr.Multiaddr) AddressedClient {
	return AddressedClient{c, addr}
}

func (c *Client) ForPeer(peer peer.ID) AddressedClient {
	return c.ForAddress(nil).ForPeer(peer)
}

// NodeInfo implements [api.NodeService.NodeInfo].
func (c *Client) NodeInfo(ctx context.Context, opts api.NodeInfoOptions) (*api.NodeInfo, error) {
	return c.ForAddress(nil).NodeInfo(ctx, opts)
}

// FindService implements [api.NodeService.FindService].
func (c *Client) FindService(ctx context.Context, opts api.FindServiceOptions) ([]*api.FindServiceResult, error) {
	return c.ForAddress(nil).FindService(ctx, opts)
}

// ConsensusStatus implements [api.NodeService.ConsensusStatus].
func (c *Client) ConsensusStatus(ctx context.Context, opts api.ConsensusStatusOptions) (*api.ConsensusStatus, error) {
	return c.ForAddress(nil).ConsensusStatus(ctx, opts)
}

// NetworkStatus implements [api.NetworkService.NetworkStatus].
func (c *Client) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	return c.ForAddress(nil).NetworkStatus(ctx, opts)
}

// Metrics implements [api.MetricsService.Metrics].
func (c *Client) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	return c.ForAddress(nil).Metrics(ctx, opts)
}

// Query implements [api.Querier.Query].
func (c *Client) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	return c.ForAddress(nil).Query(ctx, scope, query)
}

// Submit implements [api.Submitter.Submit].
func (c *Client) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	return c.ForAddress(nil).Submit(ctx, envelope, opts)
}

// Validate implements [api.Validator.Validate].
func (c *Client) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	return c.ForAddress(nil).Validate(ctx, envelope, opts)
}

// Faucet implements [api.Faucet.Faucet].
func (c *Client) Faucet(ctx context.Context, account *url.URL, opts api.FaucetOptions) (*api.Submission, error) {
	return c.ForAddress(nil).Faucet(ctx, account, opts)
}

// NodeInfo implements [api.NodeService.NodeInfo].
func (c AddressedClient) NodeInfo(ctx context.Context, opts api.NodeInfoOptions) (*api.NodeInfo, error) {
	// Wrap the request as a NodeStatusRequest and expect a NodeStatusResponse,
	// which is unpacked into a NodeInfo
	return typedRequest[*NodeInfoResponse, *api.NodeInfo](c, ctx, &NodeInfoRequest{NodeInfoOptions: opts})
}

// FindService implements [api.NodeService.FindService].
func (c AddressedClient) FindService(ctx context.Context, opts api.FindServiceOptions) ([]*api.FindServiceResult, error) {
	// Wrap the request as a NodeStatusRequest and expect a NodeStatusResponse,
	// which is unpacked into a FindServiceResult
	return typedRequest[*FindServiceResponse, []*api.FindServiceResult](c, ctx, &FindServiceRequest{FindServiceOptions: opts})
}

// ConsensusStatus implements [api.NodeService.ConsensusStatus].
func (c AddressedClient) ConsensusStatus(ctx context.Context, opts api.ConsensusStatusOptions) (*api.ConsensusStatus, error) {
	// Wrap the request as a NodeStatusRequest and expect a NodeStatusResponse,
	// which is unpacked into a ConsensusStatus
	return typedRequest[*ConsensusStatusResponse, *api.ConsensusStatus](c, ctx, &ConsensusStatusRequest{ConsensusStatusOptions: opts})
}

// NetworkStatus implements [api.NetworkService.NetworkStatus].
func (c AddressedClient) NetworkStatus(ctx context.Context, opts api.NetworkStatusOptions) (*api.NetworkStatus, error) {
	// Wrap the request as a NetworkStatusRequest and expect a
	// NetworkStatusResponse, which is unpacked into a NetworkStatus
	return typedRequest[*NetworkStatusResponse, *api.NetworkStatus](c, ctx, &NetworkStatusRequest{NetworkStatusOptions: opts})
}

// Metrics implements [api.MetricsService.Metrics].
func (c AddressedClient) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	// Wrap the request as a MetricsRequest and expect a MetricsResponse, which
	// is unpacked into Metrics.
	req := &MetricsRequest{MetricsOptions: opts}
	return typedRequest[*MetricsResponse, *api.Metrics](c, ctx, req)
}

// Query implements [api.Querier.Query].
func (c AddressedClient) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	// Wrap the request as a QueryRequest and expect a QueryResponse, which is
	// unpacked into a Record
	req := &QueryRequest{Scope: scope, Query: query}
	return typedRequest[*RecordResponse, api.Record](c, ctx, req)
}

// Submit implements [api.Submitter.Submit].
func (c AddressedClient) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	// Wrap the request as a SubmitRequest and expect a SubmitResponse, which is
	// unpacked into Submissions
	req := &SubmitRequest{Envelope: envelope, SubmitOptions: opts}
	return typedRequest[*SubmitResponse, []*api.Submission](c, ctx, req)
}

// Validate implements [api.Validator.Validate].
func (c AddressedClient) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	// Wrap the request as a ValidateRequest and expect a ValidateResponse,
	// which is unpacked into Submissions
	req := &ValidateRequest{Envelope: envelope, ValidateOptions: opts}
	return typedRequest[*ValidateResponse, []*api.Submission](c, ctx, req)
}

// Faucet implements [api.Faucet.Faucet].
func (c AddressedClient) Faucet(ctx context.Context, account *url.URL, opts api.FaucetOptions) (*api.Submission, error) {
	// Wrap the request as a FaucetRequest and expect a FaucetResponse,
	// which is unpacked into Submissions
	req := &FaucetRequest{Account: account, FaucetOptions: opts}
	return typedRequest[*FaucetResponse, *api.Submission](c, ctx, req)
}

// typedRequest executes a round-trip call, sending the request and expecting a
// response of the given type.
func typedRequest[M response[T], T any](c AddressedClient, ctx context.Context, req Message) (T, error) {
	if c.Address != nil {
		req = &Addressed{Message: req, Address: c.Address}
	}

	var typRes M
	var errRes *ErrorResponse
	err := c.Transport.RoundTrip(ctx, []Message{req}, func(res, _ Message) error {
		switch res := res.(type) {
		case *ErrorResponse:
			errRes = res
			return nil
		case M:
			typRes = res
			return nil
		default:
			return errors.Conflict.WithFormat("invalid response type %T", res)
		}
	})
	var z T
	if err != nil {
		return z, err
	}
	if errRes != nil {
		return z, errRes.Error
	}
	return typRes.rval(), nil
}

// response exists for typedRequest.
type response[T any] interface {
	Message
	rval() T
}

func (r *NodeInfoResponse) rval() *api.NodeInfo               { return r.Value }             //nolint:unused
func (r *FindServiceResponse) rval() []*api.FindServiceResult { return unNilArray(r.Value) } //nolint:unused
func (r *ConsensusStatusResponse) rval() *api.ConsensusStatus { return r.Value }             //nolint:unused
func (r *NetworkStatusResponse) rval() *api.NetworkStatus     { return r.Value }             //nolint:unused
func (r *MetricsResponse) rval() *api.Metrics                 { return r.Value }             //nolint:unused
func (r *RecordResponse) rval() api.Record                    { return r.Value }             //nolint:unused
func (r *SubmitResponse) rval() []*api.Submission             { return unNilArray(r.Value) } //nolint:unused
func (r *ValidateResponse) rval() []*api.Submission           { return unNilArray(r.Value) } //nolint:unused
func (r *FaucetResponse) rval() *api.Submission               { return r.Value }             //nolint:unused
func (r *EventMessage) rval() []api.Event                     { return unNilArray(r.Value) } //nolint:unused

func (r *PrivateSequenceResponse) rval() *api.MessageRecord[messaging.Message] { //nolint:unused
	return r.Value
}

func unNilArray[T any, L ~[]T](l L) L { //nolint:unused
	// Make the array not nil - returning an empty array instead of nil/null is
	// better for most languages that aren't Go
	if l != nil {
		return l
	}
	return L{}
}
