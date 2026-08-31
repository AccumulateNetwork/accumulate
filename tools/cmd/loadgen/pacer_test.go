// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/ed25519"
	mrand "math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The pacer used to sleep the period after each tick, so every sleep's
// overshoot pushed the whole schedule back: measured 13% under target at
// 100 tps. Against an absolute schedule, overshoot on one tick shortens the
// next sleep, and the achieved rate is the target.
func TestPacer_OvershootDoesNotAccumulate(t *testing.T) {
	clock := time.Unix(0, 0)
	p := &pacer{next: clock, maxCatchUp: time.Second,
		now:   func() time.Time { return clock },
		sleep: func(d time.Duration) { clock = clock.Add(d + 2*time.Millisecond) }, // the scheduler always overshoots by 2ms
	}
	for i := 0; i < 1000; i++ {
		p.wait(1, 100)
	}
	// 1000 ticks at 100/s is 10 s. The old pacer would have taken 12 s.
	require.InDelta(t, 10.0, clock.Sub(time.Unix(0, 0)).Seconds(), 0.01)
}

// A pause longer than the catch-up window resyncs rather than bursting:
// after a 5 s stall the pacer does not emit 500 ticks at once.
func TestPacer_ResyncsAfterAStall(t *testing.T) {
	clock := time.Unix(0, 0)
	var slept time.Duration
	p := &pacer{next: clock, maxCatchUp: time.Second,
		now:   func() time.Time { return clock },
		sleep: func(d time.Duration) { slept += d; clock = clock.Add(d) },
	}
	p.wait(1, 100)
	clock = clock.Add(5 * time.Second) // the stall
	p.wait(1, 100)
	require.Equal(t, clock, p.next, "resynced to now")
	p.wait(1, 100)
	require.InDelta(t, 0.01, slept.Seconds()-0.01, 0.001, "the tick after the resync waits a full period again")
}

// The generator must not draw work it already knows is impossible: an
// action whose prerequisites are absent stays out of the draw entirely,
// so a run-time skip can only be a race.
func TestPick_OnlyDrawsAvailableActions(t *testing.T) {
	u := newUniverse(mrand.New(mrand.NewSource(1)))
	e := &env{u: u}

	// One identity with a full second page, no custom tokens, no majors.
	full := &keyPage{threshold: 1}
	for i := 0; i < maxPageKeys; i++ {
		full.keys = append(full.keys, newKey())
	}
	book := &keyBook{pages: []*keyPage{{keys: []ed25519.PrivateKey{newKey()}, threshold: 1}, full}}
	adi := &identity{books: []*keyBook{book, {}, {}, {}}} // at the book cap too
	u.addIdentity(adi)

	never := map[string]bool{
		"add-page-key": true, "lock-account": true, "issue-tokens": true,
		"send-tokens-custom": true, "burn-tokens-custom": true, "add-key-book": true,
	}
	for i := 0; i < 2000; i++ {
		a := e.pick()
		require.Falsef(t, never[a.name], "pick drew %q whose prerequisites do not exist", a.name)
	}

	// remove-page-key IS available (5 keys, threshold 1) and must appear.
	seen := map[string]bool{}
	for i := 0; i < 5000; i++ {
		seen[e.pick().name] = true
	}
	require.True(t, seen["remove-page-key"], "an available action must be drawn")
}
