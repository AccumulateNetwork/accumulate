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
