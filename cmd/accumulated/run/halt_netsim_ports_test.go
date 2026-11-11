// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofrs/flock"
)

// Candidate blocks are probed on a stride wider than the span one devnet
// occupies (one partition offset per BVN, 100 apart), so neighbouring
// candidates cannot overlap.
const (
	portSearchLo     = 20000
	portSearchHi     = 60000
	portSearchStride = 1000
)

// freeDevnetBase returns a base port whose entire devnet port block is free
// and reserved for the lifetime of the test.
//
// Reserved means an advisory file lock, held until t.Cleanup, on a per-block
// file in os.TempDir. The probe below cannot reserve anything: it closes its
// sockets before the netsim binds, and the halt test sleeps up to ten seconds
// (startDelay) in between. Two concurrent `go test` processes whose PIDs are
// congruent modulo the number of blocks begin their search at the same block,
// both probe it free, and the second to bind fails with "address already in
// use" (#4383). With the lock, the second process fails to take the block's
// lock and moves on to the next one. The lock is released by the kernel if the
// process dies, so a crashed run cannot leak a block. The lock only excludes
// other users of this helper; a foreign process that binds a port is still
// caught by the probe, which runs after the lock is held.
//
// The halt tests used to hardcode 36656, 37656 and 38656, with a comment
// hoping the numbers were unusual enough to avoid conflicts. They are not.
// Nothing about a number makes it exclusive: two runs on one host bind the
// same port and the second devnet fails to start. On CI that is routine — a
// branch pipeline and the merge-result pipeline for the same change run
// concurrently, often on the same runner.
//
// The failure is easy to misread. It surfaces as require.NoError on
// Instance.Start with a bind error underneath, in a quarter of a second,
// while the test passes every time locally where nothing else holds the
// port. Reproduce CI's failure exactly by occupying the port first:
//
//	python3 -c 'import socket,time;s=socket.socket();s.bind(("0.0.0.0",38656));s.listen(5);time.sleep(60)' &
//	go test ./cmd/accumulated/run -run TestHaltDevNetAPIResponses
//
// A devnet binds a block, not a port: every partition takes base+offset for
// each service, so the whole block has to be free, not just the base.
func freeDevnetBase(t *testing.T, bvns int) int {
	t.Helper()

	// Begin the search at a per-process offset, so concurrent `go test` runs
	// on one host do not probe the same numbers in the same order and race
	// each other to the same block.
	n := (portSearchHi - portSearchLo) / portSearchStride
	first := os.Getpid() % n

	for i := 0; i < n; i++ {
		base := portSearchLo + ((first+i)%n)*portSearchStride
		lock := flock.New(filepath.Join(os.TempDir(), fmt.Sprintf("accumulate-devnet-ports-%d.lock", base)))
		if ok, err := lock.TryLock(); err != nil || !ok {
			continue
		}
		if !devnetBlockFree(base, bvns) {
			_ = lock.Unlock()
			continue
		}
		t.Cleanup(func() { _ = lock.Unlock() })
		return base
	}

	t.Fatalf("no free devnet port block in [%d,%d)", portSearchLo, portSearchHi)
	return 0
}

// devnetBlockFree reports whether every port a netsim of this size would bind
// is free — on TCP, and on UDP for the P2P port, which also serves QUIC.
//
// This is a probe, not a reservation: the ports are released before the
// netsim claims them. freeDevnetBase closes that window against other users of
// the helper by holding a per-block lock; it remains open only to an unrelated
// process that binds one of these ports in the meantime.
func devnetBlockFree(base, bvns int) bool {
	// Probe every port from the base through the Accumulate API port, walked
	// numerically: it is the range a node binds that matters, not the labels.
	for part := 0; part <= bvns; part++ {
		partBase := base + part*int(portBVN)
		for off := 0; off <= int(portAccAPI); off++ {
			port := partBase + off
			if !tcpFree(port) {
				return false
			}
			if off == int(portAccP2P) && !udpFree(port) {
				return false
			}
		}
	}
	return true
}

func tcpFree(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

func udpFree(port int) bool {
	c, err := net.ListenPacket("udp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}
