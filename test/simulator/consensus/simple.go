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
	"slices"
	"sync"
	"sync/atomic"

	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

const debugSynchronous = false

// SimpleHub is a simple implementation of [Hub].
type SimpleHub struct {
	mu      *sync.Mutex
	context context.Context
	modules atomic.Pointer[[]Module]
}

func NewSimpleHub(ctx context.Context) *SimpleHub {
	s := &SimpleHub{mu: new(sync.Mutex), context: logging.With(ctx, "module", "consensus")}
	s.modules.Store(&[]Module{})
	return s
}

func (s *SimpleHub) Register(module Module) {
	for {
		m := s.modules.Load()
		n := append(*m, module)
		if s.modules.CompareAndSwap(m, &n) {
			break
		}
	}
}

func (s *SimpleHub) Unregister(module Module) {
	for {
		m := s.modules.Load()
		n := make([]Module, len(*m))
		copy(n, *m)
		n = slices.DeleteFunc(n, func(m Module) bool { return m == module })
		if s.modules.CompareAndSwap(m, &n) {
			break
		}
	}
}

func (s *SimpleHub) With(modules ...Module) Hub {
	m := *s.modules.Load()
	n := make([]Module, len(m)+len(modules))
	copy(n, m)
	copy(n[len(m):], modules)

	// Use the same mutex
	r := &SimpleHub{
		mu:      s.mu,
		context: s.context,
	}
	r.modules.Store(&n)
	return r
}

func (s *SimpleHub) Send(messages ...Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Record the results of each module separately, then concatenate them. This
	// way, the result has the same order as if the modules were called
	// synchronously.
	modules := *s.modules.Load()
	results := make([][]Message, len(modules))

	// Use a mutex to avoid races when recording an error
	var errs []error
	errMu := new(sync.Mutex)

	wg := new(sync.WaitGroup)
	receive := func(i int) {
		defer wg.Done()
		m, err := modules[i].Receive(messages...)
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
		for i := range modules {
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
