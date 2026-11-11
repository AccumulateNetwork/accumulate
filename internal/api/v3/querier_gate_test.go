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
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestQuerierGate_NoNodeStateRefuses -- #4335.
//
// The joining gate fails closed: a querier given no node state is not a node
// that never joined, it is a node that has not said, and it refuses what a
// pull reads. Serving everything is a choice made by passing nodestate.Always.
func TestQuerierGate_NoNodeStateRefuses(t *testing.T) {
	q := NewQuerier(QuerierParams{Database: servingQuerierDB(t), Partition: "BVN0"})
	ctx := context.Background()

	_, err := q.Query(ctx, protocol.PartitionUrl("BVN0"), &api.BptPageQuery{Count: 4})
	require.True(t, errors.Is(err, errors.NotReady), "BptPageQuery: got %v", err)
	require.Contains(t, err.Error(), "is joining")

	_, err = q.Query(ctx, protocol.AccountUrl("alice"),
		&api.DefaultQuery{IncludeReceipt: &api.ReceiptOptions{ForAny: true}})
	require.True(t, errors.Is(err, errors.NotReady), "account with a receipt: got %v", err)
}

func TestQuerierGate_AlwaysAnswers(t *testing.T) {
	q := NewQuerier(QuerierParams{Database: servingQuerierDB(t), Partition: "BVN0", NodeState: nodestate.Always{}})
	ctx := context.Background()

	_, err := q.Query(ctx, protocol.PartitionUrl("BVN0"), &api.BptPageQuery{Count: 4})
	require.False(t, errors.Is(err, errors.NotReady), "BptPageQuery: got %v", err)

	_, err = q.Query(ctx, protocol.AccountUrl("alice"),
		&api.DefaultQuery{IncludeReceipt: &api.ReceiptOptions{ForAny: true}})
	require.False(t, errors.Is(err, errors.NotReady), "account with a receipt: got %v", err)
}
