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
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
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
	api, _ := startNetsimAndExecuteWith(t, nil)
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

// ONE FACT, NOT TWO: the gauge says what the object that answers says
// (#4295).
//
// accumulate_node_state and the gate used to be two objects with nothing
// keeping them in agreement. nodestate.Report took whatever State its caller
// named; serving/nodeState decided the answers; and a node that started from
// genesis has no state machine at all, so the daemon ASSERTED ACTIVE on the
// gauge at dagbft.go's !joining branch and built nodestate.Always{} for its
// services a hundred and twenty lines later. A one-line change to either left
// a monitor reading a state that was not the state the node answered by.
//
// Report now takes the Serving, and nothing else writes the series. This
// asserts the property on a running node: the gauge for each partition this
// process runs equals the state of the very object its services were
// registered with, taken out of the ioc registry the daemon put it in.
//
// Nothing is built by hand: the network is the daemon's own start path and
// the Serving is the one dagbft.go registered.
func TestTheGaugeSaysWhatTheNodeAnswersBy(t *testing.T) {
	_, inst := startNetsimAndExecuteWith(t, nil)

	g := gaugeByPartition(t, "accumulate_node_state")
	for part, label := range map[string]string{protocol.Directory: "directory", "BVN1": "bvn1"} {
		serving := servingFor(t, inst, part)

		require.Contains(t, g, label, "%s exports no state (#4345a)", part)
		require.Equal(t, nodestate.Number(nodestate.StateOf(serving)), g[label],
			"%s: the gauge says %v and the object its services answer by says %v",
			part, g[label], nodestate.StateOf(serving))

		// And for a node that took nothing from a peer that value is ACTIVE,
		// which is what #4364's node-state row reads as healthy.
		require.True(t, serving.CanServeCurrent(),
			"a from-genesis node does not answer for the state it holds")
		require.Equal(t, float64(2), g[label])
	}
}

// servingFor finds the nodestate.Serving the daemon registered for a
// partition. A netsim runs each node in an Instance of its own with its own
// service registry (subnode.go), so this walks the tree the run started.
func servingFor(t *testing.T, inst *Instance, partition string) nodestate.Serving {
	t.Helper()
	if s, err := ioc.Get[nodestate.Serving](inst.services, partition); err == nil {
		return s
	}
	for _, sub := range inst.subnodes {
		if s, err := ioc.Get[nodestate.Serving](sub.services, partition); err == nil {
			return s
		}
	}
	t.Fatalf("no node ran %s: nothing registered a nodestate.Serving for it", partition)
	return nil
}
