// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"
	"errors"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	"golang.org/x/exp/slog"
)

// SimpleHub is a simple implementation of [Hub].
type SimpleHub struct {
	context context.Context
	modules []Module
}

func NewSimpleHub(ctx context.Context) *SimpleHub {
	return &SimpleHub{context: logging.With(ctx, "module", "consensus")}
}

func (s *SimpleHub) Register(module Module) {
	s.modules = append(s.modules, module)
}

func (s *SimpleHub) Send(messages ...Message) error {
	var errs []error
	var next []Message

	// Loop until there's an error or no more messages are produced
	for len(messages) > 0 && len(errs) == 0 {
		for _, m := range messages {
			slog.DebugContext(s.context, "Executing", "message-type", logging.TypeOf(m))
		}

		// Send the message to each module and collect any produced messages
		for _, module := range s.modules {
			m, err := module.Receive(messages...)
			if err != nil {
				errs = append(errs, err)
			}
			next = append(next, m...)
		}

		// Process the produced messages. Reuse `messages` and `next` to
		// minimize allocation.
		messages = append(messages[:0], next...)
		next = next[:0]

	}
	return errors.Join(errs...)
}
