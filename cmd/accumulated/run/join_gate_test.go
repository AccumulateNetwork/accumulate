// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestQuerierServiceCarriesTheJoinGate is the guard on the #4297 / #4307 fix
// that had none (#4317, #4320): THE DAEMON GIVES THE QUERIER THIS NODE'S JOIN
// STATE, AND A JOINING NODE ANSWERS NEITHER OF THE TWO READS ANOTHER NODE'S
// PULL MAKES.
//
// The auditor's mutation run established that the daemon can stop passing
// NodeState to the querier, the submitter and the validator and the whole
// suite stays green, ./cmd/accumulated/run included. The gate itself has a
// test -- internal/api/v3/serving_querier_test.go -- but it builds the querier
// by hand, so it proves the library and not the caller: a nil NodeState means
// "serve everything" (internal/api/v3/querier.go, servingFor), which is
// exactly what dropping the line at api.go produces, silently.
//
// So this test does not build a querier. It runs the daemon's own
// (*Querier).start against an Instance, takes back the service that start
// registered, and asks it the two questions a pulling node asks: a BPT page,
// and an account read carrying a receipt. Both must be refused while this
// node is BOOTING.
func TestQuerierServiceCarriesTheJoinGate(t *testing.T) {
	part := "BVN0"
	partUrl := protocol.PartitionUrl(part)

	newInstance := func(t *testing.T) *Instance {
		t.Helper()
		node, err := p2p.New(p2p.Options{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = node.Close() })
		return &Instance{
			logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			services: ioc.Registry{},
			p2p:      node,
		}
	}

	// A node that is joining. nodestate.New starts at BOOTING, which is what
	// the daemon hands over while join.Run is still pulling.
	t.Run("Joining", func(t *testing.T) {
		inst := newInstance(t)
		q := &Querier{Partition: part, Storage: &StorageOrRef{value: &MemoryStorage{}}}
		require.NoError(t, querierWantsNodeState.Register(inst.services, q, nodestate.New(partUrl)))
		require.NoError(t, q.start(inst))

		svc, err := querierProvides.Get(inst.services, q)
		require.NoError(t, err)

		// The BPT page diff: what a joining peer's walk reads (enumerate.ReadPage).
		_, err = svc.Query(context.Background(), partUrl, &apiv3.BptPageQuery{Count: 4})
		require.Error(t, err, "a joining node paged its BPT for another node's pull")
		require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
		require.Contains(t, err.Error(), "is joining")

		// The account read with a receipt: what pull.Fetch reads, and the one
		// a peer's state is proven from.
		_, err = svc.Query(context.Background(), protocol.AccountUrl("alice"),
			&apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
		require.Error(t, err, "a joining node proved state it has not executed")
		require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
		require.Contains(t, err.Error(), "is joining")
	})

	// And the control, so the test is not satisfied by a querier that refuses
	// everything: a node that never joined answers for itself. That is what
	// the daemon registers when there is no join (nodestate.Always).
	t.Run("NotJoining", func(t *testing.T) {
		inst := newInstance(t)
		q := &Querier{Partition: part, Storage: &StorageOrRef{value: &MemoryStorage{}}}
		require.NoError(t, querierWantsNodeState.Register(inst.services, q, nodestate.Always{}))
		require.NoError(t, q.start(inst))

		svc, err := querierProvides.Get(inst.services, q)
		require.NoError(t, err)

		_, err = svc.Query(context.Background(), partUrl, &apiv3.BptPageQuery{Count: 4})
		require.NoError(t, err, "a node that never joined refused to page its BPT")
	})
}
