package message

import (
	"context"

	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

type Client struct {
	Dialer Dialer
	Router Router

	// DisableFanout disables request fanout and response aggregation for
	// requests such as transaction hash searches.
	DisableFanout bool
}

type Router interface {
	Route(Message) (multiaddr.Multiaddr, error)
}

var _ api.NodeService = (*Client)(nil)
var _ api.NetworkService = (*Client)(nil)
var _ api.MetricsService = (*Client)(nil)
var _ api.Querier = (*Client)(nil)
var _ api.Submitter = (*Client)(nil)
var _ api.Validator = (*Client)(nil)

func (c *Client) NodeStatus(ctx context.Context, opts NodeStatusOptions) (*api.NodeStatus, error) {
	return typedRequest[*NodeStatusResponse, *api.NodeStatus](c, ctx, &NodeStatusRequest{NodeStatusOptions: opts})
}

func (c *Client) NetworkStatus(ctx context.Context, opts NetworkStatusOptions) (*api.NetworkStatus, error) {
	return typedRequest[*NetworkStatusResponse, *api.NetworkStatus](c, ctx, &NetworkStatusRequest{NetworkStatusOptions: opts})
}

func (c *Client) Metrics(ctx context.Context, opts api.MetricsOptions) (*api.Metrics, error) {
	req := &MetricsRequest{MetricsOptions: opts}
	return typedRequest[*MetricsResponse, *api.Metrics](c, ctx, req)
}

func (c *Client) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	req := &QueryRequest{Scope: scope, Query: query}
	return typedRequest[*RecordResponse, api.Record](c, ctx, req)
}

func (c *Client) Submit(ctx context.Context, envelope *protocol.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
	req := &SubmitRequest{Envelope: envelope, SubmitOptions: opts}
	return typedRequest[*SubmitResponse, []*api.Submission](c, ctx, req)
}

func (c *Client) Validate(ctx context.Context, envelope *protocol.Envelope, opts api.ValidateOptions) ([]*api.Submission, error) {
	req := &ValidateRequest{Envelope: envelope, ValidateOptions: opts}
	return typedRequest[*ValidateResponse, []*api.Submission](c, ctx, req)
}

func typedRequest[M response[T], T any](c *Client, ctx context.Context, req Message) (T, error) {
	var typRes M
	var errRes *ErrorResponse
	err := c.routedRoundTrip(ctx, []Message{req}, func(res, _ Message) error {
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

func (c *Client) routedRoundTrip(ctx context.Context, requests []Message, callback func(res, req Message) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if c.DisableFanout {
		return c.roundTrip(ctx, requests, callback)
	}

	// Collect aggregate requests
	var parts []multiaddr.Multiaddr
	aggregate := map[Message]aggregator{}
	for i, n := 0, len(requests); i < n; i++ {
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

		// Copy the request for each partition
		agg := new(recordRangeAggregator)
		ar := &Addressed{Message: req, Address: parts[0]}
		aggregate[ar] = agg
		requests[i] = ar

		for _, part := range parts[1:] {
			ar := &Addressed{Message: req, Address: part}
			aggregate[ar] = agg
			requests = append(requests, ar)
		}
	}

	if len(aggregate) == 0 {
		return c.roundTrip(ctx, requests, callback)
	}

	// Send it
	err := c.roundTrip(ctx, requests, func(res, req Message) error {
		agg, ok := aggregate[req]
		if !ok {
			return callback(res, req)
		}
		return agg.Add(res)
	})
	if err != nil {
		return errors.UnknownError.Wrap(err)
	}

	// Handle aggregates
	for req, agg := range aggregate {
		err := callback(agg.Done(), req)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	return nil
}

func (c *Client) roundTrip(ctx context.Context, req []Message, callback func(res, req Message) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	streams := map[string]Stream{}
	for _, req := range req {
		addr, err := c.Router.Route(req)
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}

		_, err = c.dial(ctx, addr, streams, func(s Stream) error {
			err := s.Write(req)
			if err != nil {
				return errors.PeerMisbehaved.WithFormat("write request: %w", err)
			}

			res, err := s.Read()
			if err != nil {
				return errors.PeerMisbehaved.WithFormat("read request: %w", err)
			}

			err = callback(res, req)
			return errors.UnknownError.Wrap(err)
		})
		if err != nil {
			return errors.UnknownError.Wrap(err)
		}
	}
	return nil
}

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

		// Return the error if the dialer does not support error reporting
		if !isMulti {
			return nil, errors.UnknownError.Wrap(err)
		}

		// Return the error if it's a client error (e.g. misdial)
		var err2 *errors.Error
		if errors.As(err, &err2) && err2.Code.IsClientError() {
			return nil, errors.UnknownError.Wrap(err)
		}

		// Report the error and try again
		if streams != nil {
			delete(streams, addrStr)
		}
		if !multi.BadDial(ctx, addr, s, err) {
			return nil, errors.NoPeer.WithFormat("unable to submit request to %v after %d attempts", addrStr, i+1)
		}
	}
}

func (c *Client) getParts(ctx context.Context) ([]multiaddr.Multiaddr, error) {
	dirMa, err := multiaddr.NewComponent("acc", protocol.Directory)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
	}

	req := new(NetworkStatusRequest)
	req.Partition = protocol.Directory
	areq := &Addressed{Message: req, Address: dirMa}
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

	ma := make([]multiaddr.Multiaddr, len(res.Value.Network.Partitions))
	for i, part := range res.Value.Network.Partitions {
		ma[i], err = multiaddr.NewComponent("acc", part.ID)
		if err != nil {
			return nil, errors.BadRequest.WithFormat("build multiaddr: %w", err)
		}
	}

	return ma, nil
}

type aggregator interface {
	Add(Message) error
	Done() Message
}

type recordRangeAggregator struct {
	value []api.Record
	total uint64
}

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

func (agg *recordRangeAggregator) Done() Message {
	rr := new(api.RecordRange[api.Record])
	rr.Records = agg.value
	rr.Total = agg.total
	return &RecordResponse{Value: rr}
}
