// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
)

// Locating the stream retention behind #4085.
//
// A 2h15m soak of the API node ended in a kernel OOM with ~1.1M live yamux
// streams (counted from live makePipeDeadline objects, two per stream) while
// goroutines stayed flat at ~420. Every one of those streams had Close()
// called on it, and yamux's Close() frees its own side immediately --
// CloseWrite() sees readState already halfReset and runs cleanup(), which
// calls session.closeStream(id). So yamux is not the retainer; something above
// it holds the reference.
//
// This test opens and closes many streams at two layers and reports what each
// retains after GC, to say which layer holds them:
//
//	raw libp2p     host.NewStream + Close      -- retention here is go-libp2p's
//	accumulate     getPeerService + ctx cancel -- extra retention here is ours
//
// Run with:
//
//	go test ./pkg/api/v3/p2p/ -run TestStreamRetention -v -streams 5000

func retained(t *testing.T, label string, n int, f func(i int)) (bytesPer float64, objsPer float64) {
	t.Helper()

	// Settle and take a baseline.
	runtime.GC()
	runtime.GC()
	var m0 runtime.MemStats
	runtime.ReadMemStats(&m0)

	for i := 0; i < n; i++ {
		f(i)
	}

	// Give asynchronous cleanup (watchdog goroutines, yamux frame processing)
	// a chance to run before measuring, so we do not accuse it of leaking
	// something it was merely slow to release.
	time.Sleep(2 * time.Second)
	runtime.GC()
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	dBytes := int64(m1.HeapAlloc) - int64(m0.HeapAlloc)
	dObjs := int64(m1.HeapObjects) - int64(m0.HeapObjects)
	bytesPer = float64(dBytes) / float64(n)
	objsPer = float64(dObjs) / float64(n)
	t.Logf("%-22s %6d streams -> retained %8.1f KB total, %7.0f B/stream, %5.1f objects/stream",
		label, n, float64(dBytes)/1024, bytesPer, objsPer)
	return
}

func TestStreamRetention(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	const netName = "leaktest"
	listen := multiaddr.StringCast("/ip4/127.0.0.1/tcp/0")

	server, err := New(Options{Network: netName, Listen: []multiaddr.Multiaddr{listen}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })

	client, err := New(Options{Network: netName, Listen: []multiaddr.Multiaddr{listen}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	// Connect the client to the server directly; discovery is not the subject.
	require.NoError(t, client.host.Connect(ctx, peer.AddrInfo{
		ID:    server.host.ID(),
		Addrs: server.host.Addrs(),
	}))

	const n = 3000

	// Layer 1: raw libp2p. The server handler closes immediately, so both ends
	// close, exactly as the real service handler does via `defer s.Close()`.
	const rawProto = "/leaktest/raw/1.0.0"
	server.host.SetStreamHandler(rawProto, func(s network.Stream) { _ = s.Close() })

	rawBytes, rawObjs := retained(t, "raw libp2p", n, func(i int) {
		s, err := client.host.NewStream(ctx, server.host.ID(), rawProto)
		if err != nil {
			t.Fatalf("raw NewStream: %v", err)
		}
		_ = s.Close()
	})

	// Layer 2: the accumulate path. getPeerService attaches a watchdog that
	// closes the stream when the context is cancelled, which is how every
	// caller releases a stream today (transport.go's `defer cancel()`).
	// A distinct address: the node registers ServiceTypeNode for itself.
	sa := &api.ServiceAddress{Type: api.ServiceTypeQuery, Argument: "leaktest"}
	require.True(t, server.RegisterService(sa, func(s message.Stream) {
		// Read until the peer goes away, mirroring Handler.Handle's loop.
		for {
			if _, err := s.Read(); err != nil {
				return
			}
		}
	}))

	// Let the service registration propagate.
	time.Sleep(time.Second)

	accBytes, accObjs := retained(t, "accumulate getPeerService", n, func(i int) {
		sctx, scancel := context.WithCancel(ctx)
		_, err := client.getPeerService(sctx, server.host.ID(), sa, nil)
		if err != nil {
			scancel()
			t.Fatalf("getPeerService: %v", err)
		}
		scancel() // fires the watchdog, which calls Close
	})

	// Report rather than assert a hard threshold: the point is to attribute the
	// retention to a layer, and a flaky byte threshold would obscure that.
	fmt.Printf("\n=== retention per stream ===\n")
	fmt.Printf("  raw libp2p                 %7.0f B  %5.1f objects\n", rawBytes, rawObjs)
	fmt.Printf("  accumulate getPeerService  %7.0f B  %5.1f objects\n", accBytes, accObjs)
	fmt.Printf("  field observation was ~1600 B/stream over 1.1M streams\n\n")
}
