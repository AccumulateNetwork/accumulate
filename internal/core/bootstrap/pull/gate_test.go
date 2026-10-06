// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// gaugedQuerier counts its calls and how many are in flight at once, and
// answers busy for the first busyFor calls.
type gaugedQuerier struct {
	calls, inflight, most atomic.Int64
	busyFor               int64
}

func (q *gaugedQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	n := q.calls.Add(1)
	in := q.inflight.Add(1)
	defer q.inflight.Add(-1)
	for {
		m := q.most.Load()
		if in <= m || q.most.CompareAndSwap(m, in) {
			break
		}
	}
	time.Sleep(2 * time.Millisecond)
	if n <= q.busyFor {
		return nil, errors.NotReady.With("query capacity exhausted, retry later")
	}
	return &api.ErrorRecord{}, nil
}

// TestQueriesToOnePeerAreGatedAndABusyAnswerIsAskedAgain — #4536: every
// query the node sends one peer goes through that peer's gate, PeerInFlight
// at a time whoever asks, and a busy answer is asked again after a pause
// instead of failing the caller.
func TestQueriesToOnePeerAreGatedAndABusyAnswerIsAskedAgain(t *testing.T) {
	defer func(p streamPauses) { streamPause = p }(streamPause)
	streamPause = streamPauses{time.Millisecond, 4 * time.Millisecond, 50 * time.Millisecond}

	peer := new(gaugedQuerier)
	gate := NewGate()
	a, b := Gated(peer, gate), Gated(peer, gate)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(q api.Querier) {
			defer wg.Done()
			_, err := q.Query(context.Background(), nil, &api.DefaultQuery{})
			require.NoError(t, err)
		}(map[bool]api.Querier{true: a, false: b}[i%2 == 0])
	}
	wg.Wait()
	require.Equal(t, int64(40), peer.calls.Load())
	require.LessOrEqual(t, peer.most.Load(), int64(PeerInFlight), "more queries in flight to the peer than it admits")
	require.GreaterOrEqual(t, peer.most.Load(), int64(2), "the queries were sent one at a time")

	busy := &gaugedQuerier{busyFor: 2}
	_, err := Gated(busy, NewGate()).Query(context.Background(), nil, &api.DefaultQuery{})
	require.NoError(t, err, "a busy answer failed the caller")
	require.Equal(t, int64(3), busy.calls.Load(), "the busy query was not asked again until answered")

	always := &gaugedQuerier{busyFor: 1 << 30}
	_, err = Gated(always, NewGate()).Query(context.Background(), nil, &api.DefaultQuery{})
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady), "a peer busy past the pauses was not reported busy: %v", err)
}
