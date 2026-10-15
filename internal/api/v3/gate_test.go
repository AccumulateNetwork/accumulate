// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The query gates are the DoS boundary (#4164): admission must be bounded,
// rejection must be cheap and carry the retry-later code every internal
// client already handles, and slots must recycle.
func TestGate_BoundsAdmissionAndRecycles(t *testing.T) {
	g := newGate(2, 20*time.Millisecond)
	ctx := context.Background()

	require.NoError(t, g.enter(ctx))
	require.NoError(t, g.enter(ctx))

	start := time.Now()
	err := g.enter(ctx)
	require.Error(t, err, "a full gate must reject")
	require.True(t, errors.Is(err, errors.NotReady), "rejection is NotReady — the retry-later signal, breaker-exempt for healers")
	require.Less(t, time.Since(start), 200*time.Millisecond, "rejection must be cheap and prompt")

	g.exit()
	require.NoError(t, g.enter(ctx), "a freed slot readmits")
	g.exit()
	g.exit()
}

// A caller that arrives during the grace window gets the slot when one
// frees — brief contention does not shed load unnecessarily.
func TestGate_GraceWindowAdmitsOnRelease(t *testing.T) {
	g := newGate(1, 300*time.Millisecond)
	require.NoError(t, g.enter(context.Background()))
	go func() { time.Sleep(50 * time.Millisecond); g.exit() }()
	require.NoError(t, g.enter(context.Background()), "a slot freed inside the grace window is taken, not rejected")
	g.exit()
}

// A querier given its own gate is admitted by that gate alone: two peers
// served from one process each have their own capacity, as two nodes do
// (#4602). A querier given none is behind the process's gate.
func TestAQuerierIsAdmittedByItsOwnGate(t *testing.T) {
	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	full, free := NewQueryGate(), NewQueryGate()
	for i := 0; i < cap(full.slots); i++ {
		require.NoError(t, full.enter(ctx))
	}
	defer func() {
		for i := 0; i < cap(full.slots); i++ {
			full.exit()
		}
	}()

	u := protocol.AccountUrl("nobody")
	_, err := NewQuerier(QuerierParams{Database: db, Partition: "BVN0", QueryGate: full}).Query(ctx, u, nil)
	require.ErrorContains(t, err, "query capacity exhausted", "the querier whose gate is full must refuse")
	_, err = NewQuerier(QuerierParams{Database: db, Partition: "BVN0", QueryGate: free}).Query(ctx, u, nil)
	require.False(t, errors.Is(err, errors.NotReady), "the querier with a free gate must not refuse: %v", err)

	require.Same(t, queryGate, NewQuerier(QuerierParams{Database: db, Partition: "BVN0"}).gate)
}
