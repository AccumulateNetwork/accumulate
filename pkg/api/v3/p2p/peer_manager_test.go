// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

// The consumer of getPeers is not obliged to drain the channel: the dialer
// takes the first usable peer and returns, and its deferred drain is bounded
// and non-blocking. Before #4085 the forwarding goroutine guarded only its
// receive, so an undrained channel parked it on the send forever — one leaked
// goroutine per discovery-backed dial, each pinning a DHT query's libp2p
// streams. That OOM-killed the API node after eight hours of ordinary load.
//
// These tests assert the goroutine always terminates, which is the property
// that failed. Each uses a real timeout/cancel rather than asserting on
// internal state, so they would fail against the original code.

func TestForwardPeers_AbandonedChannelStopsAtTimeout(t *testing.T) {
	// A producer with more to say than the consumer ever reads.
	ch := make(chan peer.AddrInfo)
	go func() {
		for {
			select {
			case ch <- peer.AddrInfo{}:
			case <-time.After(2 * time.Second):
				return
			}
		}
	}()

	ch2 := make(chan peer.AddrInfo)
	done := make(chan struct{})
	go func() {
		forwardPeers(context.Background(), ch, ch2, time.After(50*time.Millisecond))
		close(done)
	}()

	// Read exactly one peer, then abandon the channel — the dialer's pattern.
	<-ch2

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("forwardPeers did not return after its timeout; it is parked on the send")
	}
}

func TestForwardPeers_AbandonedChannelStopsOnCancel(t *testing.T) {
	ch := make(chan peer.AddrInfo)
	go func() {
		for {
			select {
			case ch <- peer.AddrInfo{}:
			case <-time.After(2 * time.Second):
				return
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	ch2 := make(chan peer.AddrInfo)
	done := make(chan struct{})
	go func() {
		// A timeout far beyond the test, so cancellation is what must release it.
		forwardPeers(ctx, ch, ch2, time.After(time.Hour))
		close(done)
	}()

	<-ch2
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("forwardPeers did not return after cancellation; it is parked on the send")
	}
}

func TestForwardPeers_DeliversEveryPeerWhenDrained(t *testing.T) {
	// The fix must not cost deliveries on the ordinary path.
	const count = 8
	ch := make(chan peer.AddrInfo, count)
	for i := 0; i < count; i++ {
		ch <- peer.AddrInfo{}
	}
	close(ch)

	ch2 := make(chan peer.AddrInfo)
	go forwardPeers(context.Background(), ch, ch2, time.After(time.Hour))

	var got int
	for range ch2 {
		got++
	}
	require.Equal(t, count, got, "every peer should be forwarded when the consumer drains")
}

func TestForwardPeers_ClosesOutputWhenInputCloses(t *testing.T) {
	ch := make(chan peer.AddrInfo)
	close(ch)

	ch2 := make(chan peer.AddrInfo)
	go forwardPeers(context.Background(), ch, ch2, time.After(time.Hour))

	select {
	case _, ok := <-ch2:
		require.False(t, ok, "output channel should be closed once the input closes")
	case <-time.After(5 * time.Second):
		t.Fatal("forwardPeers did not close its output channel")
	}
}
