// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package abci

import (
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestMempoolBackpressureReject covers the load-shed decision: the default
// (50%) threshold, a configured threshold from the network global, the
// recheck and not-wired exemptions, and the boundary (>=).
func TestMempoolBackpressureReject(t *testing.T) {
	const poolCap = 5000
	cases := []struct {
		name    string
		wire    bool
		size    int
		cap     int
		pct     int64 // configured global; 0 => default (50)
		recheck bool
		want    bool
	}{
		{name: "limiter not wired", wire: false, size: poolCap, want: false},
		{name: "cap zero", wire: true, size: poolCap, cap: 0, want: false},
		{name: "recheck never rejected", wire: true, size: poolCap, cap: poolCap, recheck: true, want: false},
		{name: "empty mempool", wire: true, size: 0, cap: poolCap, want: false},

		// Default threshold (pct unset => 50%).
		{name: "default just below half", wire: true, size: 2499, cap: poolCap, want: false},
		{name: "default exactly half", wire: true, size: 2500, cap: poolCap, want: true},
		{name: "default above half", wire: true, size: 3000, cap: poolCap, want: true},

		// Configured threshold via the network global.
		{name: "custom 70 below", wire: true, size: 3499, cap: poolCap, pct: 70, want: false},
		{name: "custom 70 at", wire: true, size: 3500, cap: poolCap, pct: 70, want: true},
		{name: "custom 70 above", wire: true, size: 4200, cap: poolCap, pct: 70, want: true},
		{name: "custom 100 below full", wire: true, size: 4999, cap: poolCap, pct: 100, want: false},
		{name: "custom 100 full", wire: true, size: 5000, cap: poolCap, pct: 100, want: true},
		{name: "custom 10 sheds early", wire: true, size: 600, cap: poolCap, pct: 10, want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := new(Accumulator)
			if c.wire {
				size := c.size
				app.SetMempoolLimiter(func() int { return size }, c.cap)
			}
			app.mempoolBackpressurePct.Store(c.pct)

			msg, got := app.mempoolBackpressureReject(c.recheck)
			if got != c.want {
				t.Fatalf("reject=%v, want %v (msg=%q)", got, c.want, msg)
			}
			if got && msg == "" {
				t.Fatalf("expected a non-empty rejection message")
			}
			if !got && msg != "" {
				t.Fatalf("expected empty message when not rejecting, got %q", msg)
			}
		})
	}
}

func TestSetMempoolLimiter(t *testing.T) {
	app := new(Accumulator)
	app.SetMempoolLimiter(func() int { return 42 }, 5000)
	if app.mempoolSize == nil || app.mempoolSize() != 42 {
		t.Fatalf("mempoolSize not wired")
	}
	if app.mempoolCap != 5000 {
		t.Fatalf("mempoolCap = %d, want 5000", app.mempoolCap)
	}
}

// TestWillChangeGlobalsCachesBackpressure verifies the threshold is sourced
// from the network globals via the same event the oracle/fee schedule use,
// and is captured on first load (Old == nil).
func TestWillChangeGlobalsCachesBackpressure(t *testing.T) {
	app := new(Accumulator)
	err := app.willChangeGlobals(events.WillChangeGlobals{
		New: &core.GlobalValues{
			Globals: &protocol.NetworkGlobals{
				Limits: &protocol.NetworkLimits{MempoolBackpressurePercent: 73},
			},
		},
	})
	if err != nil {
		t.Fatalf("willChangeGlobals: %v", err)
	}
	if got := app.mempoolBackpressurePct.Load(); got != 73 {
		t.Fatalf("cached pct = %d, want 73", got)
	}
}

// TestWillChangeGlobalsNilSafe ensures missing globals/limits don't panic and
// leave the cached threshold untouched (so CheckTx falls back to the default).
func TestWillChangeGlobalsNilSafe(t *testing.T) {
	app := new(Accumulator)
	for _, e := range []events.WillChangeGlobals{
		{},
		{New: &core.GlobalValues{}},
		{New: &core.GlobalValues{Globals: &protocol.NetworkGlobals{}}},
	} {
		if err := app.willChangeGlobals(e); err != nil {
			t.Fatalf("willChangeGlobals: %v", err)
		}
	}
	if got := app.mempoolBackpressurePct.Load(); got != 0 {
		t.Fatalf("expected pct unchanged (0), got %d", got)
	}
}
