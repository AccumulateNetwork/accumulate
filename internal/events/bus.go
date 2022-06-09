package events

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

type Bus struct {
	mu          *sync.Mutex
	subscribers []func(Event) error
	logger      logging.OptionalLogger
}

func NewBus(logger log.Logger) *Bus {
	b := new(Bus)
	b.mu = new(sync.Mutex)
	b.logger.L = logger
	return b
}

func (b *Bus) subscribe(sub func(Event) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers = append(b.subscribers, sub)
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

func SubscribeSync[T Event](b *Bus, sub func(T) error) {
	b.subscribe(func(e Event) (err error) {
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

func SubscribeAsync[T Event](b *Bus, sub func(T)) {
	b.subscribe(func(e Event) (err error) {
		et, ok := e.(T)
		if !ok {
			return nil
		}

		go func() {
			defer func() {
				r := recover()
				if r == nil {
					return
				}

				b.logger.Error("Subscriber panicked", "error", r, "stack", string(debug.Stack()))
			}()

			sub(et)
		}()
		return nil
	})
}
