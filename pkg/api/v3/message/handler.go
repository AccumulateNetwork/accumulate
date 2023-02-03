// Copyright 2023 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"
	"io"
	"runtime/debug"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Handler handles message streams.
type Handler struct {
	logger  logging.OptionalLogger
	methods serviceMethodMap
}

// NewHandler constructs a new Handler with the given list of services.
// NewHandler only returns an error if more than one service attempts to
// register a method for the same request type.
func NewHandler(logger log.Logger, services ...Service) (*Handler, error) {
	h := new(Handler)
	h.logger.Set(logger)
	h.methods = serviceMethodMap{}
	for _, service := range services {
		for typ, method := range service.methods() {
			if _, ok := h.methods[typ]; ok {
				return nil, errors.Conflict.WithFormat("double registered method %v", typ)
			}
			h.methods[typ] = method
		}
	}

	return h, nil
}

// Handle handles a message stream. Handle is safe to call from a goroutine.
func (h *Handler) Handle(s Stream) {
	// Panic protection
	defer func() {
		if r := recover(); r != nil {
			h.logger.Error("Panicked while handling stream", "error", r, "stack", debug.Stack())
		}
	}()

	// Gotta have that context 👌
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
			h.logger.Error("Unable to decode request from peer", "error", err)
			return
		}

		// Strip off addressing
		for {
			if r, ok := req.(*Addressed); ok {
				req = r.Message
			} else {
				break
			}
		}

		// Find the method
		m, ok := h.methods[req.Type()]
		if !ok {
			err = s.Write(&ErrorResponse{Error: errors.NotAllowed.WithFormat("%v not supported", req.Type())})
			if err != nil {
				h.logger.Error("Unable to send error response to peer", "error", err)
				return
			}
		}

		// And call it
		m(&call[Message]{
			context: ctx,
			logger:  h.logger,
			stream:  s,
			params:  req,
		})
	}
}
