// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Handler handles message streams.
type Handler struct {
	methods serviceMethodMap
}

// NewHandler constructs a new Handler with the given list of services.
// NewHandler only returns an error if more than one service attempts to
// register a method for the same request type.
func NewHandler(services ...Service) (*Handler, error) {
	h := new(Handler)
	h.methods = serviceMethodMap{}
	err := h.Register(services...)
	return h, err
}

func (h *Handler) Register(services ...Service) error {
	for _, service := range services {
		for typ, method := range service.methods() {
			if _, ok := h.methods[typ]; ok {
				return errors.Conflict.WithFormat("double registered method %v", typ)
			}
			h.methods[typ] = method
		}
	}
	return nil
}

type request struct {
	message Message
	done    chan struct{}
}

// Handle handles a message stream. Handle is safe to call from a goroutine.
func (h *Handler) Handle(s Stream) {
	// Gotta have that context 👌
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// If the stream spends too long (more than 10 minutes) waiting for a
	// request, the handler shuts down. Since Stream does not support read
	// deadlines, this is achieved using a separate read loop that feeds
	// requests to the main loop via a channel, allowing the main loop to set a
	// timeout.
	//
	// However, Handle needs to read a request, process it, and write the
	// response. If the read loop were allowed to continue reading unimpeded,
	// under certain circumstances (a single-duplex message pipe), the read loop
	// could read back the response. To avoid this, the read loop passes a
	// completion channel along with the message so that the main loop can
	// signal when it is done processing.

	// Read requests in a goroutine
	rd := make(chan request)
	go func() {
		defer close(rd)

		for {
			// Read the next request
			req, err := s.Read()
			switch {
			case err == nil:
				// Ok

			case errors.Is(err, io.EOF),
				errors.Is(err, network.ErrReset),
				errors.Is(err, context.Canceled):
				// Done
				return

			default:
				slog.Error("Unable to decode request from peer", "error", err, "module", "api")
				return
			}

			// Send the request
			done := make(chan struct{})
			select {
			case rd <- request{req, done}:
			case <-ctx.Done():
				return
			}

			// Wait for the request to be processed
			select {
			case <-done:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case req, ok := <-rd:
			if !ok {
				return
			}
			h.processRequest(ctx, s, req)

		case <-time.After(10 * time.Minute):
			// If the connection is idle for more than 10 minutes, close it
			return
		}
	}
}

func (h *Handler) processRequest(ctx context.Context, s Stream, req request) {
	defer close(req.done)

	// Strip off addressing
	for {
		if r, ok := req.message.(*Addressed); ok {
			req.message = r.Message
		} else {
			break
		}
	}

	// Find the method
	m, ok := h.methods[req.message.Type()]
	if !ok {
		err := s.Write(&ErrorResponse{Error: errors.NotAllowed.WithFormat("%v not supported", req.message.Type())})
		if err != nil {
			slog.Error("Unable to send error response to peer", "error", err, "module", "api")
			return
		}
		return
	}

	// And call it
	m(&call[Message]{
		context: ctx,
		stream:  s,
		params:  req.message,
	})
}
