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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4153: the inline-vs-cascade decision compares the NEXT message's inner
// principal, not the pending ID's account — which is the local partition URL
// and matched nothing but anchors, leaving the inline branch dead and a
// pending tail draining one message per block.

func cascadeSeq(principal *url.URL, n uint64) *messaging.SequencedMessage {
	txn := new(protocol.Transaction)
	txn.Header.Principal = principal
	txn.Body = &protocol.SyntheticDepositCredits{Amount: n}
	return &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: txn},
		Source:      protocol.PartitionUrl("BVN1"),
		Destination: protocol.PartitionUrl("BVN0"),
		Number:      n,
	}
}

func TestCascade_SameIdentityTailIsInline(t *testing.T) {
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	cur := cascadeSeq(protocol.AccountUrl("alice", "tokens"), 1)
	next := cascadeSeq(protocol.AccountUrl("alice", "book"), 2)
	require.NoError(t, batch.Message(next.Hash()).Main().Put(next))

	// The pending ledger stores seq.ID() — Destination.WithTxID — so the
	// TxID's account is the PARTITION, never the principal. The decision
	// must still find the principal.
	assert.Equal(t, protocol.PartitionUrl("BVN0").String(), next.ID().Account().String(),
		"precondition: the pending ID's account is the local partition, not the principal")
	assert.True(t, SequencedMessage{}.nextTargetsSameIdentity(batch, next.ID(), cur),
		"a same-identity pending tail drains inline, in this block")
}

func TestCascade_DifferentIdentityDefersToTheQueue(t *testing.T) {
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	cur := cascadeSeq(protocol.AccountUrl("alice", "tokens"), 1)
	next := cascadeSeq(protocol.AccountUrl("bob", "tokens"), 2)
	require.NoError(t, batch.Message(next.Hash()).Main().Put(next))

	assert.False(t, SequencedMessage{}.nextTargetsSameIdentity(batch, next.ID(), cur),
		"a cross-identity cascade defers to the next block's queue (#4146)")
}

func TestCascade_UnknownNextMessageDefersToTheQueue(t *testing.T) {
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	cur := cascadeSeq(protocol.AccountUrl("alice", "tokens"), 1)
	missing := cascadeSeq(protocol.AccountUrl("alice", "book"), 2)
	// NOT stored — the decision cannot see a principal, so it must take the
	// conservative lane rather than guessing.
	assert.False(t, SequencedMessage{}.nextTargetsSameIdentity(batch, missing.ID(), cur))
}

func TestCascade_AnchorFastPathStaysInline(t *testing.T) {
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	// Every anchor's principal is the LOCAL anchor pool — same identity as
	// the pending ID's partition account — and the fast path answers without
	// loading anything.
	mkAnchor := func(n uint64) *messaging.SequencedMessage {
		txn := new(protocol.Transaction)
		txn.Header.Principal = protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
		txn.Body = &protocol.DirectoryAnchor{}
		return &messaging.SequencedMessage{
			Message:     &messaging.TransactionMessage{Transaction: txn},
			Source:      protocol.DnUrl(),
			Destination: protocol.PartitionUrl("BVN0"),
			Number:      n,
		}
	}
	cur, next := mkAnchor(1), mkAnchor(2)
	// Deliberately not stored: the fast path must not need the body.
	assert.True(t, SequencedMessage{}.nextTargetsSameIdentity(batch, next.ID(), cur))
}

// #4163: one delivery schedules the whole contiguous received run (bounded),
// not just the immediate successor. One-per-block was a real ceiling — a
// ~600-message backlog at 10 tps drained at 1.04/block, barely above its own
// refill rate.

func cascadeLedger(delivered uint64, received ...uint64) *protocol.PartitionSyntheticLedger {
	l := new(protocol.PartitionSyntheticLedger)
	l.Url = protocol.PartitionUrl("BVN1")
	l.Delivered = delivered
	for _, n := range received {
		l.Add(false, n, protocol.PartitionUrl("BVN0").WithTxID([32]byte{byte(n), byte(n >> 8)}))
	}
	return l
}

func TestCascade_SchedulesTheWholeContiguousRun(t *testing.T) {
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	queue := batch.Account(protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic)).CascadeDeliveryQueue()

	// Received: 2-6 contiguous, hole at 7, 8 received.
	ledger := cascadeLedger(1, 2, 3, 4, 5, 6, 8)
	require.NoError(t, scheduleCascadeRun(queue, ledger, 1))

	queued, err := queue.Get()
	require.NoError(t, err)
	require.Len(t, queued, 5, "2-6 scheduled; the hole at 7 ends the run — 8 must NOT be scheduled ahead of order")
	for i, n := range []uint64{2, 3, 4, 5, 6} {
		id, _ := ledger.Get(n)
		assert.True(t, queued[i].Equal(id), "queue[%d] is seq %d", i, n)
	}
}

func TestCascade_WindowBoundsTheRun(t *testing.T) {
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	queue := batch.Account(protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic)).CascadeDeliveryQueue()

	seqs := make([]uint64, 100)
	for i := range seqs {
		seqs[i] = uint64(i + 2)
	}
	require.NoError(t, scheduleCascadeRun(queue, cascadeLedger(1, seqs...), 1))

	queued, err := queue.Get()
	require.NoError(t, err)
	assert.Len(t, queued, cascadeDeliveryWindow, "a long backlog is scheduled window-at-a-time")
}

func TestCascade_DoesNotDoubleScheduleWithinABlock(t *testing.T) {
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	queue := batch.Account(protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic)).CascadeDeliveryQueue()

	ledger := cascadeLedger(1, 2, 3, 4, 5)
	require.NoError(t, scheduleCascadeRun(queue, ledger, 1))
	// A later delivery in the same block (seq 2 delivered by the queue's own
	// bundle NEXT block would restart at 2; here simulate seq 2's delivery
	// scheduling from 3 while 3-5 are already queued).
	require.NoError(t, scheduleCascadeRun(queue, ledger, 2))

	queued, err := queue.Get()
	require.NoError(t, err)
	assert.Len(t, queued, 4, "3-5 were already scheduled; nothing is added twice")
}
