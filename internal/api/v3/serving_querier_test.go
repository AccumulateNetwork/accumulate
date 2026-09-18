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

// TestQuerier_AJoiningNodeDoesNotServeAPull — #4297.
//
// A joining node's leaves and root are the half-filled ones its own pull is
// building. The two things another node's pull reads from a peer are a BPT
// page and an account with a receipt, and a joining node must answer neither:
// a second joining node would otherwise take its spine — unverified by
// construction — from the first, and never pull the spine again.
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

// TestQuerier_AJoiningNodeStillAnswersForItself — the cost #4297 weighs.
// Gating everything would stop a node answering ordinary questions about
// itself, on a node people query. A plain read is not what a pull takes, so it
// stays open; the answer here is the store's (the account does not exist),
// which is the point.
func TestQuerier_AJoiningNodeStillAnswersForItself(t *testing.T) {
	q := joiningQuerier(t)
	_, err := q.Query(context.Background(), protocol.AccountUrl("alice"), &api.DefaultQuery{})
	require.Error(t, err)
	require.False(t, errors.Is(err, errors.NotReady), "a plain read was refused by the join gate: %v", err)
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
