// Copyright 2022 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package events

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
	sortutil "gitlab.com/accumulatenetwork/accumulate/internal/util/sort"
)

type Bus struct {
	mu          *sync.Mutex
	subscribers []func(Event) error
	logger      logging.OptionalLogger
}

type Unsubscribe func()

func NewBus(logger log.Logger) *Bus {
	b := new(Bus)
	b.mu = new(sync.Mutex)
	b.logger.L = logger
	return b
}

func (b *Bus) subscribe(sub func(Event) error) Unsubscribe {
	b.mu.Lock()
	defer b.mu.Unlock()
	i := len(b.subscribers)
	b.subscribers = append(b.subscribers, sub)

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		sortutil.RemoveAt(&b.subscribers, i)
	}
}

func (b *Bus) Publish(event Event) error {
	b.mu.Lock()
	n := len(b.subscribers)
	subs := b.subscribers
	b.mu.Unlock()

	var errs []error
	for _, sub := range subs[:n] {
		err := sub(event)
		if err != nil {
			errs = append(errs, err)
		}
	}

	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	}

	s := make([]string, len(errs))
	for i, err := range errs {
		s[i] = err.Error()
	}
	return errors.New(strings.Join(s, "; "))
}

func SubscribeSync[T Event](b *Bus, sub func(T) error) Unsubscribe {
	return b.subscribe(func(e Event) (err error) {
		et, ok := e.(T)
		if !ok {
			return nil
		}

		defer func() {
			r := recover()
			if r == nil {
				return
			}

			b.logger.Error("Subscriber panicked", "error", r, "stack", string(debug.Stack()))

			var ok bool
			if err, ok = r.(error); !ok {
				err = fmt.Errorf("panicked: %v", r)
			}
		}()

		return sub(et)
	})
}

func SubscribeAsync[T Event](b *Bus, sub func(T)) Unsubscribe {
	return b.subscribe(func(e Event) (err error) {
		et, ok := e.(T)
		if !ok {
			return nil
		}

		go func() {
			defer logging.Recover(b.logger, "Subscriber panicked")
			sub(et)
		}()
		return nil
	})
}
