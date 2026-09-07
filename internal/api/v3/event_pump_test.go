// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

// A subscriber that goes away without reading must release its pump: the
// goroutine, its slot in the subscriber count, and with it the per-block
// entry loading the count gates (#4240). Before, the pump parked forever on
// its second send to a one-slot channel nobody read.
func TestSubscribe_CancelledSubscriberReleasesThePump(t *testing.T) {
	s := NewEventService(EventServiceParams{Partition: "bvn1", EventBus: events.NewBus(nil)})
	loaded := func(i uint64) *api.BlockEvent {
		return &api.BlockEvent{Partition: "bvn1", Index: i, Entries: []*api.ChainEntryRecord[api.Record]{}}
	}

	before := runtime.NumGoroutine()
	const subscribers = 8
	var cancels []context.CancelFunc
	var chans []<-chan api.Event
	for i := 0; i < subscribers; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancels = append(cancels, cancel)
		ch, err := s.Subscribe(ctx, api.SubscribeOptions{})
		require.NoError(t, err)
		chans = append(chans, ch)
	}
	require.Eventually(t, func() bool { return s.subscribers.Load() == subscribers }, time.Second, time.Millisecond)

	// Two blocks with nobody reading: the first fills each one-slot channel,
	// the second parks every pump on its send.
	s.publish(loaded(1))
	require.Eventually(t, func() bool {
		for _, ch := range chans {
			if len(ch) != 1 {
				return false
			}
		}
		return true
	}, time.Second, time.Millisecond)
	s.publish(loaded(2))
	time.Sleep(50 * time.Millisecond)

	for _, cancel := range cancels {
		cancel()
	}
	require.Eventually(t, func() bool { return s.subscribers.Load() == 0 }, 5*time.Second, time.Millisecond,
		"cancelled subscribers still counted: %d", s.subscribers.Load())
	require.Eventually(t, func() bool { return runtime.NumGoroutine() <= before+1 }, 5*time.Second, time.Millisecond,
		"pump goroutines leaked: %d before, %d after", before, runtime.NumGoroutine())
}

// A live subscriber still receives every event, and the channel closes when
// it cancels.
func TestSubscribe_DeliversThenClosesOnCancel(t *testing.T) {
	s := NewEventService(EventServiceParams{Partition: "bvn1", EventBus: events.NewBus(nil)})
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := s.Subscribe(ctx, api.SubscribeOptions{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return s.subscribers.Load() == 1 }, time.Second, time.Millisecond)

	s.publish(&api.BlockEvent{Partition: "bvn1", Index: 1, Entries: []*api.ChainEntryRecord[api.Record]{}})
	select {
	case e := <-ch:
		require.Equal(t, uint64(1), e.(*api.BlockEvent).Index)
	case <-time.After(time.Second):
		t.Fatal("no event delivered")
	}

	cancel()
	select {
	case _, ok := <-ch:
		require.False(t, ok, "the channel closes on cancel")
	case <-time.After(time.Second):
		t.Fatal("channel not closed")
	}
}
