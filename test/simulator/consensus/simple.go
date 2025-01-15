// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package consensus

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

const debugSynchronous = false

// SimpleHub is a simple implementation of [Hub].
type SimpleHub struct {
	mu      *sync.Mutex
	context context.Context
	modules []Module
}

func NewSimpleHub(ctx context.Context) *SimpleHub {
	return &SimpleHub{mu: new(sync.Mutex), context: logging.With(ctx, "module", "consensus")}
}

func (s *SimpleHub) Register(module Module) {
	s.modules = append(s.modules, module)
}

func (s *SimpleHub) With(modules ...Module) Hub {
	// Use the same mutex
	r := *s
	r.modules = append(modules, s.modules...)
	return &r
}

func (s *SimpleHub) Send(messages ...Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Record the results of each module separately, then concatenate them. This
	// way, the result has the same order as if the modules were called
	// synchronously.
	results := make([][]Message, len(s.modules))

	// Use a mutex to avoid races when recording an error
	var errs []error
	errMu := new(sync.Mutex)

	wg := new(sync.WaitGroup)
	receive := func(i int) {
		defer wg.Done()
		m, err := s.modules[i].Receive(messages...)
		results[i] = m

		if err == nil {
			return
		}

		errMu.Lock()
		defer errMu.Unlock()
		errs = append(errs, err)
	}

	// Loop until there's an error or no more messages are produced
	for len(messages) > 0 && len(errs) == 0 {
		for _, m := range messages {
			slog.DebugContext(s.context, "Executing", "message-type", logging.TypeOf(m))
		}

		// Send the messages to each module
		for i := range s.modules {
			wg.Add(1)
			if debugSynchronous {
				receive(i)
			} else {
				go receive(i)
			}
		}
		wg.Wait()

		// Collect produced messages. Reuse `messages` to minimize allocation.
		messages = messages[:0]
		for _, m := range results {
			messages = append(messages, m...)
		}
	}

	return errors.Join(errs...)
}

type Capture[V Message] []V

func (r *Capture[V]) Receive(messages ...Message) ([]Message, error) {
	for _, msg := range messages {
		if msg, ok := msg.(V); ok {
			*r = append(*r, msg)
		}
	}
	return nil, nil
}
