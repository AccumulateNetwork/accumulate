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

// heldQuerier holds every background query until release is closed, and
// answers a block's at once, busy for its first busyFor calls.
type heldQuerier struct {
	release           chan struct{}
	background, block atomic.Int64
	busyFor           int64
}

func (q *heldQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	if !isUrgent(ctx) {
		q.background.Add(1)
		select {
		case <-q.release:
		case <-ctx.Done():
		}
		return &api.ErrorRecord{}, nil
	}
	if q.block.Add(1) <= q.busyFor {
		return nil, errors.NotReady.With("query capacity exhausted, retry later")
	}
	return &api.ErrorRecord{}, nil
}

// TestABlocksQueryGoesBeforeTheBackground — #4557, run 20261010T194045Z: a
// follower's fill and look-ahead held every slot of its gate to a peer, and
// the section a block waited for queued behind them, was answered busy, and
// paused up to 10 s at a time, until its 30 s were gone and the block was
// held, twice in 26 joins. A block's query takes a slot the background never
// holds, no background query starts to the peer while one is waiting or in
// flight, and a busy answer is asked again after a short pause for as long as
// the block's context lasts.
func TestABlocksQueryGoesBeforeTheBackground(t *testing.T) {
	peer := &heldQuerier{release: make(chan struct{})}
	gate := NewGate()
	q := Gated(peer, gate)
	bg, stop := context.WithCancel(context.Background())
	defer stop()
	for i := 0; i < 10; i++ {
		go func() { _, _ = q.Query(bg, nil, &api.DefaultQuery{}) }()
	}
	require.Eventually(t, func() bool { return peer.background.Load() == PeerInFlight-1 }, time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, int64(PeerInFlight-1), peer.background.Load(), "the background took the slot kept for a block")

	ctx, cancel := context.WithTimeout(Urgent(context.Background()), time.Second)
	defer cancel()
	_, err := q.Query(ctx, nil, &api.DefaultQuery{})
	require.NoError(t, err, "a block's query waited behind the background")

	// While a block's query is waiting, as it is during a busy peer's pause,
	// the background held finishes and none starts.
	peer.busyFor = 1 << 30
	waiting, cancelWaiting := context.WithCancel(Urgent(context.Background()))
	done := make(chan struct{})
	go func() { defer close(done); _, _ = q.Query(waiting, nil, &api.DefaultQuery{}) }()
	require.Eventually(t, func() bool { return peer.block.Load() >= 3 }, time.Second, time.Millisecond)
	close(peer.release)
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, int64(PeerInFlight-1), peer.background.Load(), "a background query started while a block's was waiting")
	cancelWaiting()
	<-done
	require.Eventually(t, func() bool { return peer.background.Load() == 10 }, time.Second, time.Millisecond, "the background did not resume")

	// A busy peer is asked again quickly, past the background's give, for as
	// long as the block's context lasts.
	// Six busy answers: 3.5 s of a block's pauses; the background's would be
	// 15.75 s, past its 15 s give.
	busy := &heldQuerier{busyFor: 6}
	start := time.Now()
	_, err = Gated(busy, NewGate()).Query(Urgent(context.Background()), nil, &api.DefaultQuery{})
	require.NoError(t, err)
	require.Less(t, time.Since(start), 8*time.Second, "a block's query paused as the background does")
	require.Equal(t, int64(7), busy.block.Load())
}
