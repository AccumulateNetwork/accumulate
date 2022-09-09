package websocket

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

type Service interface {
	methods() map[string]methodFunc
}

type methodFunc func(ctx context.Context, logger log.Logger, params json.RawMessage, send chan<- *Response, recv <-chan *Request) error

func NewServer(logger log.Logger, services ...Service) (*Server, error) {
	methods := map[string]methodFunc{}
	for _, service := range services {
		for name, method := range service.methods() {
			if _, ok := methods[name]; ok {
				return nil, errors.Format(errors.StatusConflict, "double registered method %q", name)
			}
			methods[name] = method
		}
	}

	s := new(Server)
	s.logger.Set(logger)
	s.methods = methods
	s.upgrader = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
	return s, nil
}

type Server struct {
	logger   logging.OptionalLogger
	upgrader *websocket.Upgrader
	methods  map[string]methodFunc
}

func (s *Server) FallbackTo(fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			s.ServeHTTP(w, r)
		} else {
			fallback.ServeHTTP(w, r)
		}
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("Websocket upgrade failed", "error", err)
		return
	}

	var nextStream uint
	streams := map[uint]*serverStream{}

	// Close all the streams if the connection is closed
	defer func() {
		for _, s := range streams {
			s.Close()
		}
	}()

	// Ensure the connection is closed
	defer conn.Close()

	// Write loop
	allSend := make(chan *Response, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Info("Write loop panicked", "error", r, "stack", debug.Stack())
			}
		}()

		// Ensure the connection is closed
		defer conn.Close()

		for resp := range allSend {
			err := conn.WriteJSON(resp)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					s.logger.Info("Write message failed", "error", err)
				}
				return
			}
		}
	}()

	// Read loop
	for {
		req := new(Request)
		err := conn.ReadJSON(req)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.logger.Info("Read message failed", "error", err)
			}
			return
		}

		// Is the message for a stream?
		if req.Stream > 0 {
			s, ok := streams[req.Stream]
			if !ok {
				allSend <- &Response{RequestId: req.Id, Stream: req.Stream, Error: errors.NotFound("no stream with ID %d", req.Stream).(*errors.Error)}
				continue
			}

			// Close?
			if strings.EqualFold(req.Method, "close") {
				s.Close()
				continue
			}

			s.recv <- req
			continue
		}

		// Open a new stream
		method, ok := s.methods[strings.ToLower(req.Method)]
		if !ok {
			allSend <- &Response{RequestId: req.Id, Error: errors.NotFound("%s is not a method", req.Method).(*errors.Error)}
			continue
		}

		nextStream++
		ss := new(serverStream)
		ss.id = nextStream
		ss.close = new(sync.Once)
		streams[ss.id] = ss

		mctx, mcancel := context.WithCancel(r.Context())
		ss.cancel = mcancel
		defer mcancel()

		recv := make(chan *Request)
		send := make(chan *Response)
		ss.recv = recv

		// Forward messages
		go func() {
			for {
				select {
				case <-mctx.Done():
					return
				case msg := <-send:
					msg.Stream = ss.id
					allSend <- msg
				}
			}
		}()

		// Call the method
		err = method(mctx, s.logger, req.Params, send, recv)
		if err != nil {
			delete(streams, ss.id)
			ss.Close()
			allSend <- &Response{RequestId: req.Id, Error: errors.Wrap(errors.StatusUnknownError, err).(*errors.Error)}
			continue
		}

		allSend <- &Response{RequestId: req.Id, Stream: ss.id, Result: []byte(`"success"`)}
	}
}
