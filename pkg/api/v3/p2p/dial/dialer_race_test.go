// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dial

import (
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
)

// TestTryDial_ConcurrentAttemptRace pins the fix for the fleet-wide crash in
// run 20260820T054217Z: two goroutines racing through tryDial's LoadOrStore
// could observe an Attempt whose time pointer was still nil and dereference
// it — a panic on the conductor and dispatcher paths that killed nodes
// outright. The lastTry.Delete on successful dials (the #4115 backoff reset)
// reopens the window constantly, so this hammers tryDial with interleaved
// deletes. Run with -race to also catch data races; without the fix this
// panics almost immediately.
func TestTryDial_ConcurrentAttemptRace(t *testing.T) {
	d := new(dialer)
	svc := api.ServiceTypeSubmit.AddressFor("directory")
	id := peer.ID("test-peer")

	var race sync.WaitGroup
	for g := 0; g < 8; g++ {
		race.Add(1)
		go func(g int) {
			defer race.Done()
			for i := 0; i < 500; i++ {
				d.tryDial(id, svc, nil, nil)
				if g == 0 && i%3 == 0 {
					// The success path forgets the peer's backoff history —
					// this is what keeps the LoadOrStore window open.
					d.lastTry.Delete(id)
				}
			}
		}(g)
	}
	race.Wait()
	// Reaching here without a panic is the assertion.
}

// TestTryDial_BackoffReturnDoesNotStrandWaitGroup pins the wg-leak fixed
// alongside the race: tryDial used to wg.Add(1) before the backoff check, so
// an early return stranded the caller's Wait forever. Every Add must be
// balanced by the spawned goroutine's Done.
func TestTryDial_BackoffReturnDoesNotStrandWaitGroup(t *testing.T) {
	d := new(dialer)
	svc := api.ServiceTypeSubmit.AddressFor("directory")
	id := peer.ID("test-peer")

	var wg sync.WaitGroup
	// The first call commits an attempt; the immediate repeats hit the
	// backoff and return early. None may strand the WaitGroup.
	for i := 0; i < 50; i++ {
		d.tryDial(id, svc, nil, &wg)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("wg.Wait never returned — an early return in tryDial stranded an Add")
	}
}
