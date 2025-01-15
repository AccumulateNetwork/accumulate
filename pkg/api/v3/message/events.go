// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message

import (
	"context"
	"io"

	"github.com/libp2p/go-libp2p/core/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

type EventService struct {
	api.EventService
}

func (s EventService) methods() serviceMethodMap {
	typ, fn := makeServiceMethod(s.Subscribe)
	return serviceMethodMap{typ: fn}
}

func (s EventService) Subscribe(c *call[*SubscribeRequest]) {
	ch, err := s.EventService.Subscribe(c.context, c.params.SubscribeOptions)
	if err != nil {
		c.Write(&ErrorResponse{Error: errors.UnknownError.Wrap(err).(*errors.Error)})
		return
	}

	if !c.Write(&SubscribeResponse{}) {
		return
	}

	for event := range ch {
		if !c.Write(&EventMessage{Value: []api.Event{event}}) {
			return
		}
	}
}

func (c *Client) Subscribe(ctx context.Context, opts api.SubscribeOptions) (<-chan api.Event, error) {
	req := &SubscribeRequest{SubscribeOptions: opts}
	var errRes *ErrorResponse
	s, err := c.Transport.OpenStream(ctx, req, func(res, _ Message) error {
		switch res := res.(type) {
		case *ErrorResponse:
			errRes = res
			return nil
		case *SubscribeResponse:
			// Success
			return nil
		default:
			return errors.Conflict.WithFormat("invalid response type %T", res)
		}
	})
	if err != nil {
		return nil, err
	}
	if errRes != nil {
		return nil, errRes.Error
	}

	ch := make(chan api.Event, 10)
	go func() {
		defer close(ch)
		defer func() {
			r := recover()
			if r != nil {
				format := "panicked: %v"
				if _, ok := r.(error); ok {
					format = "panicked: %w"
				}
				ch <- &api.ErrorEvent{Err: errors.InternalError.WithFormat(format, r)}
			}
		}()

		for {
			msg, err := s.Read()
			switch {
			case err == nil:
				// Ok

			case errors.Is(err, io.EOF),
				errors.Is(err, network.ErrReset),
				errors.Is(err, context.Canceled):
				// Done
				return

			default:
				ch <- &api.ErrorEvent{Err: errors.UnknownError.Wrap(err).(*errors.Error)}
				return
			}

			switch msg := msg.(type) {
			case *ErrorResponse:
				ch <- &api.ErrorEvent{Err: errors.UnknownError.Wrap(msg.Error).(*errors.Error)}
			case *EventMessage:
				for _, v := range msg.Value {
					ch <- v
				}
			default:
				ch <- &api.ErrorEvent{Err: errors.Conflict.WithFormat("invalid response type %T", msg)}
				return
			}
		}
	}()
	return ch, nil
}
