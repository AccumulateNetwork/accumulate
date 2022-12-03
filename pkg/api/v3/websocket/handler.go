package websocket

import (
	"context"
	"io"
	"net/http"
	"runtime/debug"

	"github.com/gorilla/websocket"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

func NewHandler(logger log.Logger, services ...message.Service) (*Handler, error) {
	inner, err := message.NewHandler(logger, services...)
	if err != nil {
		return nil, err
	}

	s := new(Handler)
	s.logger.Set(logger)
	s.inner = inner
	s.upgrader = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
	return s, nil
}

type Handler struct {
	inner    *message.Handler
	logger   logging.OptionalLogger
	upgrader *websocket.Upgrader
}

func (s *Handler) FallbackTo(fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			s.ServeHTTP(w, r)
		} else {
			fallback.ServeHTTP(w, r)
		}
	})
}

type streamCanceller struct {
	message.Stream
	cancel context.CancelFunc
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("Websocket upgrade failed", "error", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Ensure the connection is closed
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	h.handle(clientConn{conn}, ctx, cancel)
}

func (h *Handler) handle(s message.StreamOf[*Message], ctx context.Context, cancel context.CancelFunc) {
	streams := map[uint64]streamCanceller{}

	// Write loop
	outgoing := make(chan *Message, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				h.logger.Info("Write loop panicked", "error", r, "stack", debug.Stack())
			}
		}()
		defer cancel()
		for msg := range outgoing {
			err := s.Write(msg)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					h.logger.Info("Failed to write to connection", "error", err)
				}
				return
			}
		}
	}()

	// Read loop
	for {
		// Read a message
		req, err := s.Read()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				h.logger.Info("Failed to read from connection", "error", err)
			}
			return
		}

		// Is the message for an existing stream?
		s, ok := streams[req.ID]
		if ok {
			err = s.Write(req.Message)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					h.logger.Info("Failed to write to stream", "id", req.ID, "error", err)
				}
				s.cancel()
			}
			continue
		}

		// Open a new stream
		ctx, cancel := context.WithCancel(ctx)
		p, q := message.DuplexPipe(ctx)
		streams[req.ID] = streamCanceller{p, cancel}

		// Cleanup
		go func() {
			<-ctx.Done()
			delete(streams, req.ID)
			outgoing <- &Message{ID: req.ID, Status: StreamStatusClosed}
		}()

		// Forward outgoing messages
		go func() {
			defer func() {
				if r := recover(); r != nil {
					h.logger.Error("Panicked while handling stream", "error", r)
				}
			}()
			defer cancel()
			for {
				msg, err := p.Read()
				if err != nil {
					if !errors.Is(err, io.EOF) {
						h.logger.Info("Failed to read from stream", "id", req.ID, "error", err)
					}
					return
				}

				outgoing <- &Message{ID: req.ID, Message: msg}
			}
		}()

		// Process the stream
		go func() {
			defer func() {
				if r := recover(); r != nil {
					h.logger.Error("Panicked while handling stream", "error", r)
				}
			}()
			defer cancel()
			h.inner.Handle(q)
		}()

		err = p.Write(req.Message)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				h.logger.Info("Failed to write to stream", "id", req.ID, "error", err)
			}
			cancel()
		}
	}
}
