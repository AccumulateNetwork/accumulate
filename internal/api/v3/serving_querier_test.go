// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

func servingQuerierDB(t *testing.T) *database.Database {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	return db
}

func joiningQuerier(t *testing.T) *Querier {
	t.Helper()
	return NewQuerier(QuerierParams{
		Database:  servingQuerierDB(t),
		Partition: "BVN0",
		// BOOTING: the state a node is in from the moment it starts joining
		// until its root matches one the Directory anchored.
		NodeState: nodestate.New(protocol.PartitionUrl("BVN0")),
	})
}

// TestQuerier_AJoiningNodeDoesNotServeAPull — #4297, the two reads that
// compound.
//
// A joining node's leaves and root are the half-filled ones its own pull is
// building. The two things another node's pull reads from a peer are a BPT
// page and an account with a receipt, and a joining node must answer neither:
// a second joining node would otherwise take its spine — unverified by
// construction — from the first, and never pull the spine again. (Since #4295
// it answers no read at all; these two are the ones whose refusal was
// already there.)
//
// NotReady, so the caller asks another node rather than believing the answer.
func TestQuerier_AJoiningNodeDoesNotServeAPull(t *testing.T) {
	q := joiningQuerier(t)
	ctx := context.Background()

	_, err := q.Query(ctx, protocol.PartitionUrl("BVN0"), &api.BptPageQuery{Count: 4})
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady), "BptPageQuery: got %v", err)
	require.Contains(t, err.Error(), "is joining", "it must say why it refused")

	_, err = q.Query(ctx, protocol.AccountUrl("alice"),
		&api.DefaultQuery{IncludeReceipt: &api.ReceiptOptions{ForAny: true}})
	require.Error(t, err)
	require.True(t, errors.Is(err, errors.NotReady), "account with a receipt: got %v", err)
}

// TestQuerier_AJoiningNodeRefusesAPlainReadToo — what #4295 changed.
//
// #4297 left a plain read open, weighing the cost of a joining node not
// answering ordinary questions about itself against the hazard of the two
// reads a pull takes. The spec settled it the other way: "in this phase a
// syncing node refuses every read and answers once it is fully synced"
// (executor.md, "Sync", step 6), because a plain read out of a half-filled
// store is some of one block's state and some of another's, and because
// BOOTING now ends at the root match rather than at a backfill that never
// comes. So the account here is not reported missing — the node declines to
// look.
func TestQuerier_AJoiningNodeRefusesAPlainReadToo(t *testing.T) {
	q := joiningQuerier(t)
	ctx := context.Background()
	count := uint64(4)

	for _, q2 := range []struct {
		name  string
		query api.Query
	}{
		{"plain account", &api.DefaultQuery{}},
		{"chain", &api.ChainQuery{Name: "main"}},
		{"directory", &api.DirectoryQuery{Range: &api.RangeOptions{Count: &count}}},
		{"pending", &api.PendingQuery{Range: &api.RangeOptions{Count: &count}}},
	} {
		_, err := q.Query(ctx, protocol.AccountUrl("alice"), q2.query)
		require.Error(t, err, "%s was answered by a joining node", q2.name)
		require.True(t, errors.Is(err, errors.NotReady), "%s: got %v", q2.name, err)
		require.Contains(t, err.Error(), "is joining", "%s: it must say why it refused", q2.name)
	}
}

// TestQuerier_ANodeThatNeverJoinedAnswersEverything — no node state means the
// node has always executed what it holds, here as for the sequencer.
func TestQuerier_ANodeThatNeverJoinedAnswersEverything(t *testing.T) {
	db := servingQuerierDB(t)
	page := &api.BptPageQuery{Count: 4}

	q := NewQuerier(QuerierParams{Database: db, Partition: "BVN0"})
	_, err := q.Query(context.Background(), protocol.PartitionUrl("BVN0"), page)
	require.False(t, errors.Is(err, errors.NotReady), "an ungated node refused a BPT page: %v", err)

	q = NewQuerier(QuerierParams{Database: db, Partition: "BVN0", NodeState: nodestate.Always{}})
	_, err = q.Query(context.Background(), protocol.PartitionUrl("BVN0"), page)
	require.False(t, errors.Is(err, errors.NotReady), "a node that never joined refused a BPT page: %v", err)
}
