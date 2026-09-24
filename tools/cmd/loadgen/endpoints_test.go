// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// scriptedNode answers every query with one fixed result and counts the asks.
type scriptedNode struct {
	err   error
	asked int
}

func (n *scriptedNode) Query(context.Context, *url.URL, api.Query) (api.Record, error) {
	n.asked++
	if n.err != nil {
		return nil, n.err
	}
	return new(api.ChainRecord), nil
}

// joining is the error a joining node's answer becomes on this side of the
// JSON-RPC client: UnknownError "request failed: %w" around the node's own
// NotReady (pkg/api/v3/jsonrpc/client.go, sendRequest). Run 20260924T052134Z
// logged it 52 times: `request failed: BVN3 is joining and cannot answer for
// state it has not executed`.
func joining() error {
	return errors.UnknownError.WithFormat("request failed: %w",
		errors.NotReady.With("BVN3 is joining and cannot answer for state it has not executed"))
}

func pool(nodes ...*scriptedNode) *poolQuerier {
	qs := make([]api.Querier, len(nodes))
	for i, n := range nodes {
		qs[i] = n
	}
	// idx starts so that the first query goes to nodes[0].
	idx := new(atomic.Uint64)
	idx.Store(uint64(len(nodes) - 1))
	return &poolQuerier{clients: qs, idx: idx}
}

// TestPoolQuerier_NotReadyIsAskedElsewhere — #4404 item 5. A NotReady answer
// is the protocol's "ask someone else"; the pool used to move on only after a
// transport error and handed the refusal to the caller as a failed read.
func TestPoolQuerier_NotReadyIsAskedElsewhere(t *testing.T) {
	a, b := &scriptedNode{err: joining()}, &scriptedNode{}
	p := pool(a, b)

	_, err := p.Query(context.Background(), url.MustParse("acc://x.acme"), new(api.DefaultQuery))
	require.NoError(t, err, "a joining node's refusal must be retried at another endpoint")
	require.Equal(t, 1, a.asked)
	require.Equal(t, 1, b.asked)
	require.Equal(t, queryStats{NotReadyRetried: 1}, p.counts.snapshot(),
		"the hand-off is counted")
}

// TestPoolQuerier_NotReadyEverywhereIsReturnedAndCounted — when no endpoint
// will answer, the caller gets the NotReady and the run gets a count of it.
func TestPoolQuerier_NotReadyEverywhereIsReturnedAndCounted(t *testing.T) {
	a, b := &scriptedNode{err: joining()}, &scriptedNode{err: joining()}
	p := pool(a, b)

	_, err := p.Query(context.Background(), url.MustParse("acc://x.acme"), new(api.DefaultQuery))
	require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
	require.Equal(t, queryStats{NotReadyRetried: 1, NotReadyExhausted: 1}, p.counts.snapshot())
}

// TestPoolQuerier_ABusinessErrorIsAnAnswer — a node that answered, with a
// real result such as not-found, is not asked again anywhere.
func TestPoolQuerier_ABusinessErrorIsAnAnswer(t *testing.T) {
	a := &scriptedNode{err: errors.UnknownError.WithFormat("request failed: %w",
		errors.NotFound.With("account not found"))}
	b := &scriptedNode{}
	p := pool(a, b)

	_, err := p.Query(context.Background(), url.MustParse("acc://x.acme"), new(api.DefaultQuery))
	require.True(t, errors.Is(err, errors.NotFound), "got %v", err)
	require.Equal(t, 0, b.asked)
	require.Equal(t, queryStats{}, p.counts.snapshot())
}

// TestPoolQuerier_TransportErrorsStillRotateAndAreCounted — the rule that was
// already there, now counted beside the new one.
func TestPoolQuerier_TransportErrorsStillRotateAndAreCounted(t *testing.T) {
	a := &scriptedNode{err: errors.UnknownError.With("dial tcp 127.0.0.1:26680: connect: connection refused")}
	b := &scriptedNode{}
	p := pool(a, b)

	_, err := p.Query(context.Background(), url.MustParse("acc://x.acme"), new(api.DefaultQuery))
	require.NoError(t, err)
	require.Equal(t, queryStats{NetErrRetried: 1}, p.counts.snapshot())
}
