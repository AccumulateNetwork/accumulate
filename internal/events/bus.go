package events

import (
	"runtime/debug"
	"sync"

	"github.com/tendermint/tendermint/libs/log"
	"gitlab.com/accumulatenetwork/accumulate/internal/logging"
)

type Bus struct {
	mu          *sync.Mutex
	subscribers []func(Event)
	logger      logging.OptionalLogger
}

func NewBus(logger log.Logger) *Bus {
	b := new(Bus)
	b.mu = new(sync.Mutex)
	b.logger.L = logger
	return b
}

func (b *Bus) subscribe(sub func(Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers = append(b.subscribers, sub)
}

func (b *Bus) Publish(event Event) {
	b.mu.Lock()
	n := len(b.subscribers)
	subs := b.subscribers
	b.mu.Unlock()

	for _, sub := range subs[:n] {
		sub(event)
	}
}

func SubscribeSync[T Event](b *Bus, sub func(T)) {
	b.subscribe(func(e Event) {
		et, ok := e.(T)
		if !ok {
			return
		}

		defer func() {
			err := recover()
			if err == nil {
				return
			}

			b.logger.Error("Subscriber panicked", "error", err, "stack", string(debug.Stack()))
		}()

		sub(et)
	})
}

func SubscribeAsync[T Event](b *Bus, sub func(T)) {
	b.subscribe(func(e Event) {
		et, ok := e.(T)
		if !ok {
			return
		}

		go func() {
			defer func() {
				err := recover()
				if err == nil {
					return
				}

				b.logger.Error("Subscriber panicked", "error", err, "stack", string(debug.Stack()))
			}()

			sub(et)
		}()
	})
}
