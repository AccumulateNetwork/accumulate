package websocket

import (
	"context"
	"encoding/json"
	"runtime/debug"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

type EventService struct{ api.EventService }

func (s EventService) methods() map[string]methodFunc {
	return map[string]methodFunc{
		"subscribe": s.Subscribe,
	}
}

func (s EventService) Subscribe(ctx context.Context, logger log.Logger, _ json.RawMessage, send chan<- *Response, recv <-chan *Request) error {
	events, err := s.EventService.Subscribe(ctx)
	if err != nil {
		return err
	}

	// Discard all requests
	go func() {
		for range recv {
		}
	}()

	// Send events
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if logger != nil {
					logger.Error("Subscribe write loop panicked", "error", r, "stack", debug.Stack())
				}
			}
		}()
		for event := range events {
			b, err := json.Marshal(event)
			if err == nil {
				send <- &Response{Result: b}
			} else if logger != nil {
				logger.Error("Marshalling response failed", "error", err, "stack", debug.Stack())
			}
		}
	}()

	return nil
}
