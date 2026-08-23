// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4146's one correctness window: routing can change between produce and
// drain, because globals change on block boundaries. A message queued as
// local whose destination is no longer local must be PROMOTED to the
// cross-partition path — re-joined to the block's produced set so it is
// sequenced at block end — never executed locally against an account this
// partition no longer holds, and never dropped.
func TestLocalQueue_RoutingChangeMidQueuePromotesTheMessage(t *testing.T) {
	x := new(Executor)
	x.Describe = execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	b := &Block{Batch: batch, Executor: x}

	// Produce a local message while bob routes to BVN0.
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.AccountUrl("bob", "tokens")
	txn.Body = &protocol.SyntheticDepositCredits{Amount: 3}
	msg := &messaging.TransactionMessage{Transaction: txn}

	x.Router = routeEverythingTo("BVN0")
	remote, err := b.splitLocalDeliveries([]*ProducedMessage{{
		Producer:    protocol.AccountUrl("alice", "tokens").WithTxID([32]byte{1}),
		Destination: txn.Header.Principal,
		Message:     msg,
	}})
	require.NoError(t, err)
	require.Empty(t, remote, "bob routes here — the message must queue, not sequence")

	queued, err := batch.Account(x.Describe.Synthetic()).LocalDeliveryQueue().Get()
	require.NoError(t, err)
	require.Len(t, queued, 1)

	// Between produce and drain, bob moves to BVN1.
	x.Router = routeEverythingTo("BVN1")

	require.NoError(t, b.drainDeliveryQueues())

	// The message was neither executed locally nor dropped: it rejoined the
	// produced set for this block's sorted sequencing pass.
	require.Len(t, b.produced, 1, "the promoted message must rejoin the produced set")
	assert.True(t, b.produced[0].Destination.Equal(txn.Header.Principal))
	assert.Equal(t, msg.Hash(), b.produced[0].Message.Hash())
	assert.Nil(t, b.produced[0].Producer,
		"the queue does not carry producer attribution — the promotion takes the producer-less slot in the canonical order")

	// And the queue is clear.
	queued, err = batch.Account(x.Describe.Synthetic()).LocalDeliveryQueue().Get()
	require.NoError(t, err)
	assert.Empty(t, queued)
}
