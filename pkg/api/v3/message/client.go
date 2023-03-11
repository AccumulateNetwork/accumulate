// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Client is a binary message transport client for API v3.
type Client struct {
	// Network to query. Some queries are only available when a network is
	// specified.
	Network string

	// Dialer dials connections to a given address. The client indicates that
	// the stream can be closed by canceling the context passed to Dial.
	Dialer Dialer

	// Router determines the address a message should be routed to.
	Router Router
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

// NodeInfo implements [api.NodeService.NodeInfo].
func (c *Client) NodeInfo(ctx context.Context, opts NodeInfoOptions) (*api.NodeInfo, error) {
	// Wrap the request as a NodeStatusRequest and expect a NodeStatusResponse,
	// which is unpacked into a NodeInfo
	return typedRequest[*NodeInfoResponse, *api.NodeInfo](c, ctx, &NodeInfoRequest{NodeInfoOptions: opts})
}

// FindService implements [api.NodeService.FindService].
func (c *Client) FindService(ctx context.Context, opts FindServiceOptions) ([]*api.FindServiceResult, error) {
	// Wrap the request as a NodeStatusRequest and expect a NodeStatusResponse,
	// which is unpacked into a FindServiceResult
	return typedRequest[*FindServiceResponse, []*api.FindServiceResult](c, ctx, &FindServiceRequest{FindServiceOptions: opts})
}

// ConsensusStatus implements [api.NodeService.ConsensusStatus].
func (c *Client) ConsensusStatus(ctx context.Context, opts ConsensusStatusOptions) (*api.ConsensusStatus, error) {
	// Wrap the request as a NodeStatusRequest and expect a NodeStatusResponse,
	// which is unpacked into a ConsensusStatus
	return typedRequest[*ConsensusStatusResponse, *api.ConsensusStatus](c, ctx, &ConsensusStatusRequest{ConsensusStatusOptions: opts})
}

// NetworkStatus implements [api.NetworkService.NetworkStatus].
func (c *Client) NetworkStatus(ctx context.Context, opts NetworkStatusOptions) (*api.NetworkStatus, error) {
	// Wrap the request as a NetworkStatusRequest and expect a
	// NetworkStatusResponse, which is unpacked into a NetworkStatus
	return typedRequest[*NetworkStatusResponse, *api.NetworkStatus](c, ctx, &NetworkStatusRequest{NetworkStatusOptions: opts})
}

// Metrics implements [api.MetricsService.Metrics].
func (c *Client) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	// Wrap the request as a MetricsRequest and expect a MetricsResponse, which
	// is unpacked into Metrics.
	req := &MetricsRequest{MetricsOptions: opts}
	return typedRequest[*MetricsResponse, *api.Metrics](c, ctx, req)
}

// Query implements [api.Querier.Query].
func (c *Client) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	// Wrap the request as a QueryRequest and expect a QueryResponse, which is
	// unpacked into a Record
	req := &QueryRequest{Scope: scope, Query: query}
	return typedRequest[*RecordResponse, api.Record](c, ctx, req)
}

// Submit implements [api.Submitter.Submit].
func (c *Client) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	// Wrap the request as a SubmitRequest and expect a SubmitResponse, which is
	// unpacked into Submissions
	req := &SubmitRequest{Envelope: envelope, SubmitOptions: opts}
	return typedRequest[*SubmitResponse, []*api.Submission](c, ctx, req)
}

// Validate implements [api.Validator.Validate].
func (c *Client) Validate(ctx context.Context, envelope *messaging.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	// Wrap the request as a ValidateRequest and expect a ValidateResponse,
	// which is unpacked into Submissions
	req := &ValidateRequest{Envelope: envelope, ValidateOptions: opts}
	return typedRequest[*ValidateResponse, []*api.Submission](c, ctx, req)
}

// Faucet implements [api.Faucet.Faucet].
func (c *Client) Faucet(ctx context.Context, account *url.URL, opts api.FaucetOptions) (*api.Submission, error) {
	// Wrap the request as a FaucetRequest and expect a FaucetResponse,
	// which is unpacked into Submissions
	req := &FaucetRequest{Account: account, FaucetOptions: opts}
	return typedRequest[*FaucetResponse, *api.Submission](c, ctx, req)
}

// typedRequest executes a round-trip call, sending the request and expecting a
// response of the given type.
func typedRequest[M response[T], T any](c *Client, ctx context.Context, req Message) (T, error) {
	var typRes M
	var errRes *ErrorResponse
	err := c.RoundTrip(ctx, []Message{req}, func(res, _ Message) error {
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

func (r *NodeInfoResponse) rval() *api.NodeInfo                 { return r.Value } //nolint:unused
func (r *FindServiceResponse) rval() []*api.FindServiceResult   { return r.Value } //nolint:unused
func (r *ConsensusStatusResponse) rval() *api.ConsensusStatus   { return r.Value } //nolint:unused
func (r *NetworkStatusResponse) rval() *api.NetworkStatus       { return r.Value } //nolint:unused
func (r *MetricsResponse) rval() *api.Metrics                   { return r.Value } //nolint:unused
func (r *RecordResponse) rval() api.Record                      { return r.Value } //nolint:unused
func (r *SubmitResponse) rval() []*api.Submission               { return r.Value } //nolint:unused
func (r *ValidateResponse) rval() []*api.Submission             { return r.Value } //nolint:unused
func (r *FaucetResponse) rval() *api.Submission                 { return r.Value } //nolint:unused
func (r *EventMessage) rval() []api.Event                       { return r.Value } //nolint:unused
func (r *PrivateSequenceResponse) rval() *api.TransactionRecord { return r.Value } //nolint:unused

// RoundTrip routes each requests and executes a round-trip call. If there are
// no transport errors, RoundTrip will only dial each address once. If multiple
// requests route to the same address, the first request will dial a stream and
// subsequent requests to that address will reuse the existing stream.
//
// Certain types of requests, such as transaction hash searches, are fanned out
// to every partition. If requests contains such a request, RoundTrip queries
// the network for a list of partitions, submits the request to every partition,
// and aggregates the responses into a single response.
//
// RoundTrip is a low-level interface not intended for general use.
//
// roundTrip routes each request, dials a stream, and executes a round-trip
// call, sending the request and waiting for the response. roundTrip calls the
// callback for each request-response pair. The callback may be called more than
// once if there are transport errors.
//
// roundTrip maintains a stream dictionary, which is passed to dial. Since dial
// will reuse an existing stream when appropriate, if there are no transport
// errors, roundTrip will only dial each address once. If multiple requests
// route to the same address, the first request will dial a stream and
// subsequent requests to that address will reuse the existing stream.
func (c *Client) RoundTrip(ctx context.Context, requests []Message, callback func(res, req Message) error) error {
	// Setup the context and stream dictionary
	streams := map[string]Stream{}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Process each request
	for _, req := range requests {
		// Route it
		addr, err := c.routeRequest(req)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		// Dial the address
		_, err = c.dial(ctx, addr, streams, func(s Stream) error {
			// Send the request
			err := s.Write(req)
			if err != nil {
				return errors.PeerMisbehaved.WithFormat("write request: %w", err)
			}

			// Wait for the response
			res, err := s.Read()
			if err != nil {
				return errors.PeerMisbehaved.WithFormat("read request: %w", err)
			}

			// Call the callback
			err = callback(res, req)
			return errors.UnknownError.Wrap(err)
		})
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	return nil
}

func (c *Client) routeRequest(req Message) (multiaddr.Multiaddr, error) {
	var err error

	// Does the message already have an address?
	addr := AddressOf(req)
	if addr != nil {
		goto routed // Skip routing
	}

	// Can we route it?
	if c.Router == nil {
		return nil, errors.BadRequest.With("cannot route message: router is missing")
	}

	// Ask the router to route the request
	addr, err = c.Router.Route(req)
	if err != nil {
		return nil, errors.UnknownError.Wrap(err)
	}

routed:
	// Check if the address specifies a network or p2p node or is /acc-svc/node
	var complete bool
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_P2P, api.P_ACC:
			complete = true
		case api.P_ACC_SVC:
			sa := new(api.ServiceAddress)
			err = sa.UnmarshalBinary(c.RawValue())
			if err == nil && sa.Type == api.ServiceTypeNode {
				complete = true
			}
		}
		return !complete
	})
	if err != nil {
		return nil, errors.InternalError.WithFormat("parse routed address: %w", err)
	}
	if complete {
		return addr, nil
	}

	// Did the caller specify a network?
	if c.Network == "" {
		return nil, errors.BadRequest.With("cannot route message: network is unspecified")
	}

	// Add the network
	net, err := multiaddr.NewComponent(api.N_ACC, c.Network)
	if err != nil {
		return nil, errors.UnknownError.WithFormat("build network address: %w", err)
	}

	// Encapsulate the address with /acc/{network}
	return net.Encapsulate(addr), nil
}

// dial dials a given address and calls send with the stream, returning the
// stream. If the call provides a stream dictionary dial will use a stream from
// the dictionary if there is one, add the dialed stream if there is not, and
// remove the stream from the dictionary if a transport error occurs.
//
// If the client's dialer implements [MultiDialer], dial may make multiple
// attempts. This may result in calling send multiple times. When a transport
// error occurs, dial will report the error to [MultiDialer.BadDial]. If that
// returns true, dial will try again.
func (c *Client) dial(ctx context.Context, addr multiaddr.Multiaddr, streams map[string]Stream, send func(Stream) error) (Stream, error) {
	addrStr := addr.String()
	multi, isMulti := c.Dialer.(MultiDialer)

	for i := 0; ; i++ {
		// Check for an existing stream
		var err error
		s, ok := streams[addrStr]
		if !ok {
			// Dial a new stream
			s, err = c.Dialer.Dial(ctx, addr)
			if err != nil {
				return nil, errors.UnknownError.WithFormat("dial %v: %w", addrStr, err)
			}
			if streams != nil {
				streams[addrStr] = s
			}
		}

		err = send(s)
		if err == nil {
			return s, nil // Success
		}

		// Return the error if it's a client error (e.g. misdial)
		var err2 *errors.Error
		if errors.As(err, &err2) && err2.Code.IsClientError() {
			return nil, errors.UnknownError.Wrap(err)
		}

		// Remove the stream from the dictionary
		if streams != nil {
			delete(streams, addrStr)
		}

		// Return the error if the dialer does not support error reporting
		if !isMulti {
			return nil, errors.UnknownError.Wrap(err)
		}

		// Report the error and try again
		if !multi.BadDial(ctx, addr, s, err) {
			return nil, errors.NoPeer.WithFormat("unable to submit request to %v after %d attempts", addrStr, i+1)
		}
	}
}

// GetNetInfo queries the directory network for the network status. GetNetInfo
// is intended to be used to initialize the message router. GetNetInfo returns
// an error if the network is not specified.
func (c *Client) GetNetInfo(ctx context.Context) (*api.NetworkStatus, error) {
	if c.Network == "" {
		return nil, errors.BadRequest.With("network is unspecified")
	}

	// Route the message to /acc/directory
	dirMa, err := api.ServiceTypeNetwork.AddressFor(protocol.Directory).MultiaddrFor(c.Network)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
	}

	// Construct an Addressed NetworkStatusRequest
	req := new(NetworkStatusRequest)
	req.Partition = protocol.Directory
	areq := &Addressed{Message: req, Address: dirMa}

	// Send it and wait for a NetworkStatusResponse
	var res *NetworkStatusResponse
	err = c.RoundTrip(ctx, []Message{areq}, func(r, _ Message) error {
		var ok bool
		res, ok = r.(*NetworkStatusResponse)
		if !ok {
			return errors.WrongType.WithFormat("expected %T, got %T", res, r)
		}
		return nil
	})
	if err != nil {
		return nil, errors.UnknownError.WithFormat("get network status: %w", err)
	}

	return res.Value, nil
}
