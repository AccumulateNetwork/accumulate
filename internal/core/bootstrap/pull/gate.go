// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// PeerInFlight is how many queries this node has in flight to one peer at
// once: a validator admits four (api/v3 queryGate). A node's streams, states
// and fill each asked every peer four at a time, so one joining node alone
// had twelve in flight to a validator and most answers were refusals; a
// refusal of a single query -- the anchor pool's index-chain window search --
// then lost the account for the round (#4536, run 20261006T022626Z).
const PeerInFlight = 4

// A Gate is the queries in flight to one peer. Every querier of that peer the
// node holds shares it.
//
// A block's queries go first (#4557, run 20261010T194045Z): the fill and the
// look-ahead held every slot to a peer, and the section a block waited for
// queued behind them until its time was gone and the block was held. The
// background holds at most PeerInFlight-1 slots, so one is always free for a
// block, and starts nothing to the peer while a block's query is waiting or in
// flight, so what the peer admits goes to the block.
type Gate struct {
	mu       sync.Mutex
	inFlight int           // queries holding a slot
	urgent   int           // a block's queries, waiting, paused or holding a slot
	changed  chan struct{} // closed, and made again, when either falls
}

// NewGate is a gate with PeerInFlight slots.
func NewGate() *Gate { return &Gate{changed: make(chan struct{})} }

// enter takes a slot, waiting for one the caller may hold.
func (g *Gate) enter(ctx context.Context, urgent bool) error {
	g.mu.Lock()
	for g.inFlight >= PeerInFlight || !urgent && (g.urgent > 0 || g.inFlight >= PeerInFlight-1) {
		changed := g.changed
		g.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return errors.UnknownError.Wrap(ctx.Err())
		}
		g.mu.Lock()
	}
	g.inFlight++
	g.mu.Unlock()
	return nil
}

func (g *Gate) exit() {
	g.mu.Lock()
	g.inFlight--
	g.signal()
	g.mu.Unlock()
}

// claim and release count a block's query from its first slot to its answer,
// pauses included.
func (g *Gate) claim() {
	g.mu.Lock()
	g.urgent++
	g.mu.Unlock()
}

func (g *Gate) release() {
	g.mu.Lock()
	g.urgent--
	g.signal()
	g.mu.Unlock()
}

func (g *Gate) signal() {
	close(g.changed)
	g.changed = make(chan struct{})
}

// urgentPause is how long a block's query waits after a busy answer, doubling
// from first up to most, for as long as the block's context lasts: the
// background's pauses, up to 10 s each, spent a block's 30 s in three.
var urgentPause = struct{ first, most time.Duration }{100 * time.Millisecond, time.Second}

// Gated returns q behind gate: a query waits for a slot, and one the peer
// answers busy is asked again after a pause that doubles, streamPause.give in
// all, so no caller has to know a busy answer from a refusal.
func Gated(q api.Querier, gate *Gate) api.Querier { return gatedQuerier{q, gate} }

type gatedQuerier struct {
	api.Querier
	gate *Gate
}

// What the gate sees, per query type (#4531: no side had numbers for where a
// join's time went): calls and their time at the peer, time spent waiting for
// a slot, and busy answers.
var (
	mGateCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "accumulate", Subsystem: "pull", Name: "queries_total",
		Help: "Queries this node sent its peers, by query type",
	}, []string{"query"})
	mGateSeconds = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "accumulate", Subsystem: "pull", Name: "query_seconds_total",
		Help: "Time the peers took to answer, by query type",
	}, []string{"query"})
	mGateSlotWait = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate", Subsystem: "pull", Name: "gate_slot_wait_seconds_total",
		Help: "Time queries waited for one of a peer's slots",
	})
	mGateBusy = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "accumulate", Subsystem: "pull", Name: "busy_answers_total",
		Help: "Answers of 'query capacity exhausted' from the peers, by query type",
	}, []string{"query"})
	mGateBusyWait = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "accumulate", Subsystem: "pull", Name: "busy_wait_seconds_total",
		Help: "Time queries paused after a busy answer before asking again",
	})
)

func (g gatedQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	kind := "unknown"
	if query != nil {
		kind = query.QueryType().String()
	}
	urgent := isUrgent(ctx)
	pause, most := streamPause.first, streamPause.most
	if urgent {
		pause, most = urgentPause.first, urgentPause.most
		g.gate.claim()
		defer g.gate.release()
	}
	var waited time.Duration
	for {
		t0 := time.Now()
		if err := g.gate.enter(ctx, urgent); err != nil {
			return nil, err
		}
		t1 := time.Now()
		mGateSlotWait.Add(t1.Sub(t0).Seconds())
		r, err := g.Querier.Query(ctx, scope, query)
		g.gate.exit()
		mGateCalls.WithLabelValues(kind).Inc()
		mGateSeconds.WithLabelValues(kind).Add(time.Since(t1).Seconds())
		if err == nil || !isBusy(err) || !urgent && waited+pause > streamPause.give {
			return r, err
		}
		mGateBusy.WithLabelValues(kind).Inc()
		mGateBusyWait.Add(pause.Seconds())
		t := time.NewTimer(pause)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, errors.UnknownError.Wrap(ctx.Err())
		case <-t.C:
		}
		waited += pause
		pause = min(2*pause, most)
	}
}

type urgentKey struct{}

// Urgent marks ctx as a block's: a block is waiting on its queries, which go
// before the background's and ask a busy peer again until ctx ends, so ctx
// must have a deadline (#4557).
func Urgent(ctx context.Context) context.Context { return context.WithValue(ctx, urgentKey{}, true) }

func isUrgent(ctx context.Context) bool {
	u, _ := ctx.Value(urgentKey{}).(bool)
	return u
}
