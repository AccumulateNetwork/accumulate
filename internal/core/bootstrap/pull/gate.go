// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
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
type Gate struct{ slots chan struct{} }

// NewGate is a gate with PeerInFlight slots.
func NewGate() *Gate { return &Gate{slots: make(chan struct{}, PeerInFlight)} }

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
	pause := streamPause.first
	var waited time.Duration
	for {
		t0 := time.Now()
		select {
		case g.gate.slots <- struct{}{}:
		case <-ctx.Done():
			return nil, errors.UnknownError.Wrap(ctx.Err())
		}
		t1 := time.Now()
		mGateSlotWait.Add(t1.Sub(t0).Seconds())
		r, err := g.Querier.Query(ctx, scope, query)
		<-g.gate.slots
		mGateCalls.WithLabelValues(kind).Inc()
		mGateSeconds.WithLabelValues(kind).Add(time.Since(t1).Seconds())
		if err == nil || !isBusy(err) || waited+pause > streamPause.give {
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
		pause = min(2*pause, streamPause.most)
	}
}
