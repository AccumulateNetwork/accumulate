// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A NODE THAT STARTED FROM GENESIS ANSWERS THE READS THE SPEC RESERVES FOR
// `COMPLETE` (#4368).
//
// executor.md, "Sync", step 6: "In this phase a syncing node refuses every
// read and answers once it is fully synced ... `BOOTING` and `ACTIVE` refuse
// with `NotReady`, `COMPLETE` serves." The two reads named there are the two
// a pull makes — a BPT page, and an account read carrying a receipt
// (`servingFor`, internal/api/v3/querier.go) — and this node answers both.
//
// This pins the fact both candidate resolutions of #4368 keep, and only that
// fact: whether the state such a node reports is called `ACTIVE` (the code
// today) or `COMPLETE` (the spec's predicate made reachable), a node that
// took nothing from a peer serves. It does not assert which state it is in;
// that is #4368's decision and it belongs to the spec.
//
// Nothing is built by hand. The network is the daemon's own start path, the
// client is the real JSON-RPC client over HTTP against the node's own API,
// and the querier is the one `(*Querier).start` registered with whatever
// `nodestate.Serving` `dagbft.go` gave it.
func TestAFromGenesisNodeAnswersTheTwoPullReads(t *testing.T) {
	api := startNetsimAndExecute(t)
	c := jsonrpc.NewClient(api)
	ctx := context.Background()

	// The BPT page: what a joining peer's enumerate.Stale reads, and the
	// first of the two `servingFor` refuses while a node is joining.
	for _, part := range []string{protocol.Directory, "BVN1"} {
		_, err := c.Query(ctx, protocol.PartitionUrl(part), &apiv3.BptPageQuery{Count: 4})
		require.NoError(t, err, "a from-genesis node refused to page %s's BPT", part)
	}

	// The account read with a receipt: what pull.Fetch reads, and the one a
	// peer's state is proven from.
	for _, part := range []string{protocol.Directory, "BVN1"} {
		u := protocol.PartitionUrl(part).JoinPath(protocol.Ledger)
		r, err := c.Query(ctx, u, &apiv3.DefaultQuery{IncludeReceipt: &apiv3.ReceiptOptions{ForAny: true}})
		require.NoError(t, err, "a from-genesis node refused to prove %v", u)
		require.NotNil(t, r)
	}

	// And its state is on the wire for both partitions it runs, whatever the
	// value is: a reader that must decide whether this node can answer has
	// something to read (#4345a). The value itself is #4368's question.
	g := gaugeByPartition(t, "accumulate_node_state")
	require.Contains(t, g, "directory")
	require.Contains(t, g, "bvn1")
}
