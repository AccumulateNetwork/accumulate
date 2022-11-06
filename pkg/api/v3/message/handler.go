package message

import (
	"context"
	"io"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type Handler struct {
	logger  logging.OptionalLogger
	methods serviceMethodMap
}

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

func (h *Handler) Handle(s Stream) {
	defer func() {
		if r := recover(); r != nil {
			h.logger.Error("Panicked while handling stream", "error", r)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		req, err := s.Read()
		switch {
		case err == nil:
			// Ok

		case errors.Is(err, io.EOF),
			errors.Is(err, context.Canceled):
			// Done
			return

		default:
			h.logger.Error("Unable to decode request from peer", "error", err)
			return
		}

		for {
			if r, ok := req.(*Addressed); ok {
				req = r.Message
			} else {
				break
			}
		}

		m, ok := h.methods[req.Type()]
		if !ok {
			err = s.Write(&ErrorResponse{Error: errors.NotAllowed.WithFormat("%v not supported", req.Type())})
			if err != nil {
				h.logger.Error("Unable to send error response to peer", "error", err)
				return
			}
		}

		m(&call[Message]{
			context: ctx,
			logger:  h.logger,
			stream:  s,
			params:  req,
		})
	}
}
