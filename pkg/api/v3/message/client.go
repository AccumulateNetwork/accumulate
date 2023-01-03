// Copyright 2022 The Accumulate Authors
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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Client is a binary message transport client for API v3.
type Client struct {
	// Dialer dials connections to a given address. The client indicates that
	// the stream can be closed by canceling the context passed to Dial.
	Dialer Dialer

	// Router determines the address a message should be routed to.
	Router Router

	// DisableFanout disables request fanout and response aggregation for
	// requests such as transaction hash searches.
	DisableFanout bool
}

// A Router determines the address a message should be routed to.
type Router interface {
	Route(Message) (multiaddr.Multiaddr, error)
}

// Ensure Client satisfies the service definitions.
var _ api.NodeService = (*Client)(nil)
var _ api.NetworkService = (*Client)(nil)
var _ api.MetricsService = (*Client)(nil)
var _ api.Querier = (*Client)(nil)
var _ api.Submitter = (*Client)(nil)
var _ api.Validator = (*Client)(nil)

// NodeStatus implements [api.NodeService.NodeStatus].
func (c *Client) NodeStatus(ctx context.Context, opts NodeStatusOptions) (*api.NodeStatus, error) {
	// Wrap the request as a NodeStatusRequest and expect a NodeStatusResponse,
	// which is unpacked into a NodeStatus
	return typedRequest[*NodeStatusResponse, *api.NodeStatus](c, ctx, &NodeStatusRequest{NodeStatusOptions: opts})
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
func (c *Client) Submit(ctx context.Context, envelope *protocol.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	// Wrap the request as a SubmitRequest and expect a SubmitResponse, which is
	// unpacked into Submissions
	req := &SubmitRequest{Envelope: envelope, SubmitOptions: opts}
	return typedRequest[*SubmitResponse, []*api.Submission](c, ctx, req)
}

// Validate implements [api.Validator.Validate].
func (c *Client) Validate(ctx context.Context, envelope *protocol.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	// Wrap the request as a ValidateRequest and expect a ValidateResponse,
	// which is unpacked into Submissions
	req := &ValidateRequest{Envelope: envelope, ValidateOptions: opts}
	return typedRequest[*ValidateResponse, []*api.Submission](c, ctx, req)
}

// typedRequest executes a round-trip call, sending the request and expecting a
// response of the given type.
func typedRequest[M response[T], T any](c *Client, ctx context.Context, req Message) (T, error) {
	var typRes M
	var errRes *ErrorResponse
	err := c.roundTripWithFanout(ctx, []Message{req}, func(res, _ Message) error {
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

func (r *NodeStatusResponse) rval() *api.NodeStatus             { return r.Value } //nolint:unused
func (r *NetworkStatusResponse) rval() *api.NetworkStatus       { return r.Value } //nolint:unused
func (r *MetricsResponse) rval() *api.Metrics                   { return r.Value } //nolint:unused
func (r *RecordResponse) rval() api.Record                      { return r.Value } //nolint:unused
func (r *SubmitResponse) rval() []*api.Submission               { return r.Value } //nolint:unused
func (r *ValidateResponse) rval() []*api.Submission             { return r.Value } //nolint:unused
func (r *EventMessage) rval() []api.Event                       { return r.Value } //nolint:unused
func (r *PrivateSequenceResponse) rval() *api.TransactionRecord { return r.Value } //nolint:unused

// roundTripWithFanout routes each requests and executes a round-trip call. If
// there are no transport errors, roundTripWithFanout will only dial each
// address once. If multiple requests route to the same address, the first
// request will dial a stream and subsequent requests to that address will reuse
// the existing stream.
//
// Certain types of requests, such as transaction hash searches, are fanned out
// to every partition. If requests contains such a request, roundTripWithFanout
// queries the network for a list of partitions, submits the request to every
// partition, and aggregates the responses into a single response.
func (c *Client) roundTripWithFanout(ctx context.Context, requests []Message, callback func(res, req Message) error) error {
	if c.DisableFanout {
		return c.roundTrip(ctx, requests, callback)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Collect aggregate requests
	var parts []multiaddr.Multiaddr
	aggregate := map[Message]aggregator{}
	for i, n := 0, len(requests); i < n; i++ {
		// If the request is a transaction hash search query request, fan it out
		req, ok := requests[i].(*QueryRequest)
		if !ok {
			continue
		}
		if req.Scope == nil || !protocol.IsUnknown(req.Scope) {
			continue
		}
		if _, ok := req.Query.(*api.TransactionHashSearchQuery); !ok {
			continue
		}

		// Query the network for a list of partitions
		if parts == nil {
			var err error
			parts, err = c.getParts(ctx)
			if err != nil {
				return errors.UnknownError.Wrap(err)
			}
		}

		// Overwrite the existing entry in requests with an Addressed message
		// for the first partition
		agg := new(recordRangeAggregator)
		ar := &Addressed{Message: req, Address: parts[0]}
		aggregate[ar] = agg
		requests[i] = ar

		// Create a new Addressed message for every other partition
		for _, part := range parts[1:] {
			ar := &Addressed{Message: req, Address: part}
			aggregate[ar] = agg
			requests = append(requests, ar)
		}
	}

	// If there are no requests to fanout, just call roundTrip
	if len(aggregate) == 0 {
		return c.roundTrip(ctx, requests, callback)
	}

	// Send the requests
	err := c.roundTrip(ctx, requests, func(res, req Message) error {
		// If the request was a fanout request, aggregate the response
		agg, ok := aggregate[req]
		if !ok {
			return callback(res, req)
		}
		return agg.Add(res)
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Finalize aggregates and pass them to the callback
	for req, agg := range aggregate {
		err := callback(agg.Done(), req)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	return nil
}

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
func (c *Client) roundTrip(ctx context.Context, req []Message, callback func(res, req Message) error) error {
	// Setup the context and stream dictionary
	streams := map[string]Stream{}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Process each request
	for _, req := range req {
		// Route it
		addr, err := c.Router.Route(req)
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

// getParts queries the network for the list of partitions.
func (c *Client) getParts(ctx context.Context) ([]multiaddr.Multiaddr, error) {
	// Route the message to /acc/directory
	dirMa, err := multiaddr.NewComponent("acc", protocol.Directory)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
	}

	// Construct an Addressed NetworkStatusRequest
	req := new(NetworkStatusRequest)
	req.Partition = protocol.Directory
	areq := &Addressed{Message: req, Address: dirMa}

	// Send it and wait for a NetworkStatusResponse
	var res *NetworkStatusResponse
	err = c.roundTrip(ctx, []Message{areq}, func(r, _ Message) error {
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

	// Pre-calculate a list of multiaddresses for each partition of the form
	// /acc/{partition}
	ma := make([]multiaddr.Multiaddr, len(res.Value.Network.Partitions))
	for i, part := range res.Value.Network.Partitions {
		ma[i], err = multiaddr.NewComponent("acc", part.ID)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
		}
	}

	return ma, nil
}

// An aggregator aggregates responses.
type aggregator interface {
	// Add adds a response.
	Add(Message) error

	// Done returns the aggregated response.
	Done() Message
}

// A recordRangeRecord aggregates RecordResponses. Start is not preserved.
type recordRangeAggregator struct {
	value []api.Record
	total uint64
}

// Add checks if the message is a RecordResponse containing a RecordRange and
// adds its records to the aggregator.
func (agg *recordRangeAggregator) Add(m Message) error {
	res, ok := m.(*RecordResponse)
	if !ok {
		return errors.Conflict.WithFormat("invalid response type %T", res)
	}

	v, ok := res.Value.(*api.RecordRange[api.Record])
	if !ok {
		return errors.Conflict.WithFormat("invalid response value type %T", res.Value)
	}

	agg.value = append(agg.value, v.Records...)
	agg.total += v.Total
	return nil
}

// Done constructs a new RecordResponse containing a RecordRange containing all
// of the records.
func (agg *recordRangeAggregator) Done() Message {
	rr := new(api.RecordRange[api.Record])
	rr.Records = agg.value
	rr.Total = agg.total
	return &RecordResponse{Value: rr}
}
