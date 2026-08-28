// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

// TestForwardUntilDoesNotLeakOnAbandonedReader is the regression test for the
// goroutine leak found by the #4087 soak.
//
// Peer discovery callers stop reading as soon as they have a usable peer —
// dial.DiscoveredPeers does precisely that — so the forwarder is routinely
// abandoned mid-send. With a bare `out <- v` the goroutine parks in chansend
// forever, and neither the timeout nor the context can reach it, because a
// blocked send never re-examines them. On the soak that was 3,582 of 4,106
// goroutines on one node, one parked for 164 minutes.
//
// The test abandons the reader deliberately and then asserts the forwarder still
// exits. Closing the output channel is the observable proof it returned.
func TestForwardUntilDoesNotLeakOnAbandonedReader(t *testing.T) {
	// More values than the reader will consume, so the forwarder is guaranteed
	// to be blocked on a send when it is abandoned.
	src := make(chan peer.AddrInfo, 4)
	for i := 0; i < 4; i++ {
		src <- peer.AddrInfo{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	// A timeout far longer than the test, so that only cancellation can free the
	// forwarder. Otherwise the test would pass on the timeout alone and prove
	// nothing about abandonment.
	out := forwardUntil(ctx, src, time.Hour)

	// Read exactly one value, then walk away.
	<-out

	cancel()

	select {
	case _, ok := <-out:
		for ok {
			_, ok = <-out
		}
	case <-time.After(5 * time.Second):
		t.Fatal("forwarder did not exit after the reader abandoned it and the context was cancelled")
	}
}

// TestForwardUntilStopsOnTimeout covers the other exit: the reader is still
// present but the deadline passes.
func TestForwardUntilStopsOnTimeout(t *testing.T) {
	src := make(chan peer.AddrInfo) // never written to
	out := forwardUntil(context.Background(), src, 50*time.Millisecond)

	select {
	case _, ok := <-out:
		require.False(t, ok, "channel should be closed, not carrying a value")
	case <-time.After(5 * time.Second):
		t.Fatal("forwarder did not exit when its timeout expired")
	}
}

// TestForwardUntilForwardsAndClosesWithSource checks the ordinary path still
// works: everything is delivered, and closing the source closes the output.
func TestForwardUntilForwardsAndClosesWithSource(t *testing.T) {
	src := make(chan peer.AddrInfo, 3)
	for i := 0; i < 3; i++ {
		src <- peer.AddrInfo{}
	}
	close(src)

	out := forwardUntil(context.Background(), src, time.Hour)

	n := 0
	for range out {
		n++
	}
	require.Equal(t, 3, n, "every value should be forwarded before the channel closes")
}

// TestForwardUntilLeavesNoGoroutines is the blunt check: abandon many
// forwarders and confirm the goroutine count returns to baseline. Guards the
// case where forwardUntil exits but leaves something else behind.
func TestForwardUntilLeavesNoGoroutines(t *testing.T) {
	settle := func() int {
		for i := 0; i < 50; i++ {
			runtime.Gosched()
			time.Sleep(10 * time.Millisecond)
		}
		return runtime.NumGoroutine()
	}

	before := settle()

	for i := 0; i < 200; i++ {
		src := make(chan peer.AddrInfo, 2)
		src <- peer.AddrInfo{}
		src <- peer.AddrInfo{}
		ctx, cancel := context.WithCancel(context.Background())
		out := forwardUntil(ctx, src, time.Hour)
		<-out // take one, abandon the rest
		cancel()
	}

	after := settle()
	require.Less(t, after-before, 50,
		"abandoned forwarders leaked goroutines: %d before, %d after 200 abandonments", before, after)
}
