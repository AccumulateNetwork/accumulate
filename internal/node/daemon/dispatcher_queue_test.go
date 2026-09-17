// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package accumulated

import (
	"context"
	"crypto/ed25519"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

// fastTiming keeps the tests short: the dispatcher's retry schedule is a few
// blocks' worth of seconds in production.
func fastTiming() dispatchTiming {
	return dispatchTiming{
		attemptTimeout: 200 * time.Millisecond,
		minBackoff:     10 * time.Millisecond,
		maxBackoff:     50 * time.Millisecond,
		retryDeadline:  time.Second,
	}
}

func testEnvelope(t *testing.T) *messaging.Envelope {
	alice := url.MustParse("alice")
	_, aliceKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	return helpers.MustBuild(t,
		build.Transaction().For(alice, "tokens").BurnTokens(1, 0).
			SignWith(alice, "book", "1").Version(1).Timestamp(1).PrivateKey(aliceKey))
}

type testRouter func(*url.URL) (string, error)

func (f testRouter) RouteAccount(u *url.URL) (string, error) { return f(u) }
func (f testRouter) Route(env ...*messaging.Envelope) (string, error) {
	return routing.RouteEnvelopes(f, env...)
}

type testDialer func(context.Context, multiaddr.Multiaddr) (message.Stream, error)

func (fn testDialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
	return fn(ctx, addr)
}

// routeByPath routes an account to the partition named by its first path
// segment, so acc://foo/x goes to "foo".
func routeByPath(u *url.URL) (string, error) { return u.Authority, nil }

// answer reads one request from the stream and answers it with a plain
// SubmitResponse, delivering it to got.
func answer(ctx context.Context, s message.Stream, got chan<- message.Message) {
	m, err := s.Read()
	if err != nil {
		return
	}
	select {
	case got <- m:
	case <-ctx.Done():
		return
	}
	_ = s.Write(new(message.SubmitResponse))
}

// A destination that fails N dials and then accepts must still receive the
// envelope: a write that fails is retried (#4222).
func TestDispatcherRetriesFailedDestination(t *testing.T) {
	var dials atomic.Int32
	got := make(chan message.Message, 1)
	d := NewDispatcher(t.Name(), testRouter(routeByPath), testDialer(func(ctx context.Context, m multiaddr.Multiaddr) (message.Stream, error) {
		if dials.Add(1) <= 3 {
			return nil, errors.NoPeer.With("no live peers")
		}
		p, q := message.DuplexPipe(ctx)
		go answer(ctx, p, got)
		return q, nil
	}))
	d.timing = fastTiming()
	defer d.Close()

	require.NoError(t, d.Submit(context.Background(), url.MustParse("foo"), testEnvelope(t)))
	for range d.Send(context.Background()) { //nolint:revive // Send reports nothing; the queue does
	}

	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatalf("envelope never delivered after %d dials", dials.Load())
	}
	require.GreaterOrEqual(t, dials.Load(), int32(4))
	require.Zero(t, testutil.ToFloat64(dispatchDrops.WithLabelValues("foo", dropDeadline)))
}

// A destination that accepts a stream and never answers must not delay a
// destination that answers; each has its own queue and its own deadline.
func TestDispatcherStuckDestinationDoesNotDelayOthers(t *testing.T) {
	got := make(chan message.Message, 1)
	d := NewDispatcher(t.Name(), testRouter(routeByPath), testDialer(func(ctx context.Context, m multiaddr.Multiaddr) (message.Stream, error) {
		p, q := message.DuplexPipe(ctx)
		if strings.Contains(m.String(), "stuck") {
			// Read the request, never answer
			go func() { _, _ = p.Read() }()
			return q, nil
		}
		go answer(ctx, p, got)
		return q, nil
	}))
	d.timing = fastTiming()
	d.timing.attemptTimeout = 10 * time.Second // the stuck peer stays stuck for the whole test
	defer d.Close()

	ctx := context.Background()
	require.NoError(t, d.Submit(ctx, url.MustParse("stuck"), testEnvelope(t)))
	require.NoError(t, d.Submit(ctx, url.MustParse("fast"), testEnvelope(t)))
	for range d.Send(ctx) { //nolint:revive
	}

	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("the answering destination waited on the stuck one")
	}
}

// The queue for a destination holds at most dispatchQueueBlocks blocks of
// envelopes; older blocks are dropped, oldest first, and counted. The cache
// and the requester recover what is dropped — that is the spec's healing.
func TestDispatcherQueueBoundIsHonored(t *testing.T) {
	d := NewDispatcher(t.Name(), testRouter(routeByPath), testDialer(func(ctx context.Context, m multiaddr.Multiaddr) (message.Stream, error) {
		return nil, errors.NoPeer.With("no live peers")
	}))
	d.timing = fastTiming()
	d.timing.retryDeadline = time.Hour // only the block bound may drop
	defer d.Close()

	ctx := context.Background()
	env := testEnvelope(t)
	before := testutil.ToFloat64(dispatchDrops.WithLabelValues("bound", dropQueueFull))
	blocks := dispatchQueueBlocks + 4
	for i := 0; i < blocks; i++ {
		require.NoError(t, d.Submit(ctx, url.MustParse("bound"), env))
		require.NoError(t, d.Submit(ctx, url.MustParse("bound"), env))
		for range d.Send(ctx) { //nolint:revive
		}
	}

	require.Equal(t, 2*dispatchQueueBlocks, d.queueDepth("bound"), "queue depth")
	require.Equal(t, float64(2*4), testutil.ToFloat64(dispatchDrops.WithLabelValues("bound", dropQueueFull))-before, "drops")
}

// An envelope that outlives the retry deadline is dropped and counted; the
// queue does not hold it forever.
func TestDispatcherDropsAfterDeadline(t *testing.T) {
	d := NewDispatcher(t.Name(), testRouter(routeByPath), testDialer(func(ctx context.Context, m multiaddr.Multiaddr) (message.Stream, error) {
		return nil, errors.NoPeer.With("no live peers")
	}))
	d.timing = fastTiming()
	d.timing.retryDeadline = 100 * time.Millisecond
	defer d.Close()

	ctx := context.Background()
	before := testutil.ToFloat64(dispatchDrops.WithLabelValues("dead", dropDeadline))
	require.NoError(t, d.Submit(ctx, url.MustParse("dead"), testEnvelope(t)))
	for range d.Send(ctx) { //nolint:revive
	}

	require.Eventually(t, func() bool {
		return d.queueDepth("dead") == 0
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, float64(1), testutil.ToFloat64(dispatchDrops.WithLabelValues("dead", dropDeadline))-before)
}

// A destination that refuses an envelope as invalid settles it: no retry, a
// refusal counted. Back-pressure and server-side failures are retried.
func TestDispatcherSettlesRefusals(t *testing.T) {
	var dials atomic.Int32
	d := NewDispatcher(t.Name(), testRouter(routeByPath), testDialer(func(ctx context.Context, m multiaddr.Multiaddr) (message.Stream, error) {
		dials.Add(1)
		p, q := message.DuplexPipe(ctx)
		go func() {
			if _, err := p.Read(); err != nil {
				return
			}
			_ = p.Write(&message.ErrorResponse{Error: errors.BadRequest.With("invalid envelope")})
		}()
		return q, nil
	}))
	d.timing = fastTiming()
	defer d.Close()

	ctx := context.Background()
	before := testutil.ToFloat64(dispatchRefused.WithLabelValues("refuse"))
	require.NoError(t, d.Submit(ctx, url.MustParse("refuse"), testEnvelope(t)))
	for range d.Send(ctx) { //nolint:revive
	}

	require.Eventually(t, func() bool {
		return d.queueDepth("refuse") == 0
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, float64(1), testutil.ToFloat64(dispatchRefused.WithLabelValues("refuse"))-before)
	require.Equal(t, int32(1), dials.Load(), "a refusal is not retried")
}
