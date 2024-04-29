// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"
	"encoding/json"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type ResponseCallback func(res, req Message) error

// Transport is a binary message transport implementation.
type Transport interface {
	// RoundTrip opens one or more streams to nodes that can serve the requests
	// and processes a request-response round trip on a stream for each request.
	RoundTrip(ctx context.Context, requests []Message, callback ResponseCallback) error

	// OpenStream opens a stream to a node that can serve the request, processes
	// a request-response round trip on the stream, and returns the open stream.
	//
	// If a batch has been opened for the context, multiple calls to OpenStream
	// (with that context) will return the same Stream. Depending on the service
	// it may not be safe to reuse streams in this way.
	//
	// If a batch has _not_ been opened for the context, it is the
	// responsibility of the caller to close the stream or cancel the context.
	OpenStream(ctx context.Context, request Message, callback ResponseCallback) (Stream, error)
}

// RoutedTransport is a [Transport] implementation that routes messages to the
// appropriate service.
type RoutedTransport struct {
	// Network to query. Some queries are only available when a network is
	// specified.
	Network string

	// Dialer dials connections to a given address. The client indicates that
	// the stream can be closed by canceling the context passed to Dial.
	Dialer Dialer

	// Router determines the address a message should be routed to.
	Router Router

	// Attempts is the number of connection attempts to make.
	Attempts int
}

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
func (c *RoutedTransport) RoundTrip(ctx context.Context, requests []Message, callback ResponseCallback) error {
	// Setup the context and stream dictionary
	ctx, cancel, streams := contextWithBatchData(ctx)
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
				// Add peer ID
				p, ok := err.(interface{ Peer() peer.ID })
				err := errors.PeerMisbehaved.WithFormat("read request: %w", err)
				if ok {
					err.Data, _ = json.Marshal(struct{ Peer peer.ID }{Peer: p.Peer()})
				}
				return err
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

func (c *RoutedTransport) OpenStream(ctx context.Context, request Message, callback ResponseCallback) (Stream, error) {
	// Setup the stream dictionary
	streams := getBatchData(ctx)
	if streams == nil {
		streams = map[string]Stream{}
	}

	addr, err := c.routeRequest(request)
	if err != nil {
		return nil, errors.BadRequest.WithFormat("route request: %w", err)
	}

	// Dial the address
	s, err := c.dial(ctx, addr, streams, func(s Stream) error {
		// Send the request
		err := s.Write(request)
		if err != nil {
			return errors.PeerMisbehaved.WithFormat("write request: %w", err)
		}

		// Wait for the response
		res, err := s.Read()
		if err != nil {
			return errors.PeerMisbehaved.WithFormat("read request: %w", err)
		}

		// Call the callback
		err = callback(res, request)
		return errors.UnknownError.Wrap(err)
	})

	// Return the stream
	return s, errors.UnknownError.Wrap(err)
}

type batchDataStreamsKey struct{}

func getBatchData(ctx context.Context) map[string]Stream {
	bd := api.GetBatchData(ctx)
	if bd == nil {
		return nil
	}
	streams, _ := bd.Get(batchDataStreamsKey{}).(map[string]Stream)
	return streams
}

func contextWithBatchData(ctx context.Context) (context.Context, context.CancelFunc, map[string]Stream) {
	ctx, cancel, bd := api.ContextWithBatchData(ctx)
	streams, _ := bd.Get(batchDataStreamsKey{}).(map[string]Stream)
	if streams == nil {
		streams = map[string]Stream{}
		bd.Put(batchDataStreamsKey{}, streams)
	}
	return ctx, cancel, streams
}

func (c *RoutedTransport) routeRequest(req Message) (multiaddr.Multiaddr, error) {
	var err error

	// Does the message already have an address?
	var addr multiaddr.Multiaddr
	if msg, ok := req.(*Addressed); ok {
		addr, req = msg.Address, msg.Message
		sa, _, _, err := extractAddr(addr)
		if err != nil {
			return nil, errors.InternalError.WithFormat("parse routed address: %w", err)
		}
		if sa != nil {
			goto routed // Skip routing
		}
	}

	// Can we route it?
	if c.Router == nil {
		return nil, errors.BadRequest.With("cannot route message: router not setup")
	}

	// Ask the router to route the request
	if a, err := c.Router.Route(req); err != nil {
		return nil, errors.UnknownError.Wrap(err)
	} else if addr == nil {
		addr = a
	} else {
		addr = addr.Encapsulate(a)
	}

routed:
	// Check if the address specifies a network or p2p node or is /acc-svc/node
	sa, network, peer, err := extractAddr(addr)
	if err != nil {
		return nil, errors.InternalError.WithFormat("parse routed address: %w", err)
	}
	if peer != "" || network != "" || sa != nil && sa.Type == api.ServiceTypeNode {
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

func extractAddr(addr multiaddr.Multiaddr) (sa *api.ServiceAddress, network, peer string, err error) {
	multiaddr.ForEach(addr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_P2P:
			peer = c.String()
		case api.P_ACC:
			network = c.Value()
		case api.P_ACC_SVC:
			sa = new(api.ServiceAddress)
			err = sa.UnmarshalBinary(c.RawValue())
			return err != nil
		}
		return true
	})
	if err != nil {
		return nil, "", "", errors.UnknownError.Wrap(err)
	}

	return sa, network, peer, nil
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
func (c *RoutedTransport) dial(ctx context.Context, addr multiaddr.Multiaddr, streams map[string]Stream, send func(Stream) error) (Stream, error) {
	addrStr := addr.String()
	multi, isMulti := c.Dialer.(MultiDialer)

	n := c.Attempts
	if n == 0 {
		n = 3
	}

	var i int
	for i = 0; i < n; i++ {
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

		var err2 *errors.Error
		switch {
		case errors.As(err, &err2) && err2.Code.IsClientError():
			// Return the error if it's a client error (e.g. misdial)
			return nil, errors.UnknownError.Wrap(err)

		case errors.EncodingError.ErrorAs(err, &err2):
			// If the error is an encoding issue, log it and return "internal error"
			if isMulti {
				multi.BadDial(ctx, addr, s, err)
			}
			return nil, errors.InternalError.WithFormat("internal error: %w", err)
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
			break
		}
	}

	return nil, errors.NoPeer.WithFormat("unable to submit request to %v after %d attempts", addrStr, i+1)
}
