// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
)

// Query service gates. An unauthenticated query path with no admission
// control is a denial-of-service surface: in run 20260824T141031Z, query
// load (transaction trackers and synthetic-healing pulls, polling harder as
// their answers lagged) drove 45% of validator CPU into state reads, rounds
// stretched, dispatch backlogged, and the network collapsed — the queriers
// amplified the very lag they were measuring. The protocol must bound the
// resources queries may consume so consensus and execution always keep
// their headroom; a client that polls harder gets cheap rejections, not a
// bigger share of the node (#4164).
//
// The gates are PER PROCESS (a dual node's Directory and BVN services share
// one CPU budget) and deliberately small: queries are allowed a bounded
// slice of the node, never the node itself. Saturation returns NotReady —
// the retry-later signal every internal client already handles: the
// healers' claim windows pace their retries, and NotReady is exempt from
// their circuit breakers.
var (
	// queryGate bounds concurrent Query executions.
	queryGate = NewQueryGate()

	// sequenceGate bounds concurrent Sequence/SequenceRange executions.
	// These build merkle proofs and range packages — the most expensive
	// queries a node serves, and the healers' bulk traffic — so they get
	// their own, smaller gate: a heal storm cannot starve user queries,
	// and user queries cannot starve healing.
	sequenceGate = newGate(2, 50*time.Millisecond)
)

// A Gate is a bounded-concurrency admission control with a short grace wait.
type Gate struct {
	slots chan struct{}
	wait  time.Duration
}

func newGate(n int, wait time.Duration) *Gate {
	return &Gate{slots: make(chan struct{}, n), wait: wait}
}

// NewQueryGate is a gate with a node's query capacity: four queries at once,
// and a query that waits 50 ms for a slot refused. A node's queriers share
// the process's (queryGate). Two peers served from one process, as in a
// test, each need their own, as two nodes have (#4602).
func NewQueryGate() *Gate { return newGate(4, 50*time.Millisecond) }

// enter acquires a slot, waiting at most the grace period. Rejection is
// CHEAP — that is the point: shedding a query costs a channel poll, serving
// one costs database reads.
func (g *Gate) enter(ctx context.Context) error {
	select {
	case g.slots <- struct{}{}:
		return nil
	default:
	}
	t := time.NewTimer(g.wait)
	defer t.Stop()
	select {
	case g.slots <- struct{}{}:
		return nil
	case <-t.C:
		return errors.NotReady.With("query capacity exhausted, retry later")
	case <-ctx.Done():
		return errors.NotReady.WithFormat("query canceled: %v", ctx.Err())
	}
}

func (g *Gate) exit() { <-g.slots }
