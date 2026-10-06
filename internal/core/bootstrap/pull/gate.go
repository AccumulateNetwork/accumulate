// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"time"

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

func (g gatedQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	pause := streamPause.first
	var waited time.Duration
	for {
		select {
		case g.gate.slots <- struct{}{}:
		case <-ctx.Done():
			return nil, errors.UnknownError.Wrap(ctx.Err())
		}
		r, err := g.Querier.Query(ctx, scope, query)
		<-g.gate.slots
		if err == nil || !isBusy(err) || waited+pause > streamPause.give {
			return r, err
		}
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
