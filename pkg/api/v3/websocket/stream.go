package websocket

import (
	"context"
	"encoding/json"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/errors"
)

type Request struct {
	Id     uint            `json:"id"`
	Stream uint            `json:"stream"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type Response struct {
	RequestId uint            `json:"requestId"`
	Stream    uint            `json:"stream"`
	Result    json.RawMessage `json:"result"`
	Error     *errors.Error   `json:"error"`
}

type serverStream = stream[*Request]
type clientStream = stream[*Response]

type stream[T any] struct {
	id      uint
	close   *sync.Once
	cancel  context.CancelFunc
	recv    chan<- T
	cleanup func()
}

func (s *stream[T]) Close() {
	s.close.Do(func() {
		s.cancel()
		close(s.recv)
		if s.cleanup != nil {
			s.cleanup()
		}
	})
}
