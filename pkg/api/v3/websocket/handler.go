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

type methodFunc func(ctx context.Context, logger log.Logger, params json.RawMessage, send chan<- *Response, recv <-chan *Request) error

type Service interface {
	methods() map[string]methodFunc
}

func NewHandler(logger log.Logger, services ...Service) (*Handler, error) {
	methods := map[string]methodFunc{}
	for _, service := range services {
		for name, method := range service.methods() {
			if _, ok := methods[name]; ok {
				return nil, errors.Format(errors.StatusConflict, "double registered method %q", name)
			}
			methods[name] = method
		}
	}

	h := new(Handler)
	h.logger.Set(logger)
	h.methods = methods
	h.upgrader = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
	return h, nil
}

type Handler struct {
	logger   logging.OptionalLogger
	upgrader *websocket.Upgrader
	methods  map[string]methodFunc
}

func (h *Handler) FallbackTo(fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			h.ServeHTTP(w, r)
		} else {
			fallback.ServeHTTP(w, r)
		}
	})
}

type stream struct {
	id     uint
	once   *sync.Once
	cancel context.CancelFunc
	recv   chan<- *Request
}

type Request struct {
	Id     uint            `json:"id"`
	Stream uint            `json:"stream"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type Response struct {
	RequestId uint          `json:"requestId"`
	Stream    uint          `json:"stream"`
	Result    interface{}   `json:"result"`
	Error     *errors.Error `json:"error"`
}

func (s *stream) Close() {
	s.once.Do(func() {
		s.cancel()
		close(s.recv)
	})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("Websocket upgrade failed", "error", err)
		return
	}

	var nextStream uint
	streams := map[uint]*stream{}

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
				h.logger.Info("Write loop panicked", "error", r, "stack", debug.Stack())
			}
		}()

		// Ensure the connection is closed
		defer conn.Close()

		for resp := range allSend {
			err := conn.WriteJSON(resp)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					h.logger.Info("Write message failed", "error", err)
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
				h.logger.Info("Read message failed", "error", err)
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
				delete(streams, s.id)
				s.Close()
				continue
			}

			s.recv <- req
			continue
		}

		// Open a new stream
		method, ok := h.methods[strings.ToLower(req.Method)]
		if !ok {
			allSend <- &Response{RequestId: req.Id, Error: errors.NotFound("%s is not a method", req.Method).(*errors.Error)}
			continue
		}

		nextStream++
		s := new(stream)
		s.id = nextStream
		s.once = new(sync.Once)
		streams[s.id] = s

		mctx, mcancel := context.WithCancel(r.Context())
		s.cancel = mcancel
		defer mcancel()

		recv := make(chan *Request)
		send := make(chan *Response)
		s.recv = recv

		// Forward messages
		go func() {
			for {
				select {
				case <-mctx.Done():
					return
				case msg := <-send:
					msg.Stream = s.id
					allSend <- msg
				}
			}
		}()

		// Call the method
		err = method(mctx, h.logger, req.Params, send, recv)
		if err != nil {
			delete(streams, s.id)
			s.Close()
			allSend <- &Response{RequestId: req.Id, Error: errors.Wrap(errors.StatusUnknownError, err).(*errors.Error)}
			continue
		}

		allSend <- &Response{RequestId: req.Id, Stream: s.id, Result: "success"}
	}
}
