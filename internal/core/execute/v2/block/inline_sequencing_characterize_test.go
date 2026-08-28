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
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4144 Stage 0: characterization of INLINE sequencing, written before the
// move to a sorted block-end pass. These pin what produceSynthetic does
// today — sequence numbers assigned in delivery (slice) order, the ledger's
// Produced counter incremented per message as it is built. When deferral
// lands, DeferredSequencingMatchesInlineSequencing compares against exactly
// this behaviour; it cannot be written unless this is pinned first.

// routeEverythingTo routes every account to one partition.
type routeEverythingTo string

func (r routeEverythingTo) RouteAccount(*url.URL) (string, error)        { return string(r), nil }
func (r routeEverythingTo) Route(...*messaging.Envelope) (string, error) { return string(r), nil }

// routeByAuthority routes each account by its root identity's hostname, so a
// test can split traffic across destinations.
type routeByAuthority map[string]string

func (r routeByAuthority) RouteAccount(u *url.URL) (string, error) {
	return r[u.RootIdentity().Authority], nil
}
func (r routeByAuthority) Route(...*messaging.Envelope) (string, error) {
	panic("not used")
}

func synthTestExecutor(t *testing.T, router interface {
	RouteAccount(*url.URL) (string, error)
	Route(...*messaging.Envelope) (string, error)
}) (*Executor, *database.Batch) {
	t.Helper()
	x := new(Executor)
	x.Describe = execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}
	x.Router = router

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	ledger := new(protocol.SyntheticLedger)
	ledger.Url = x.Describe.Synthetic()
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))
	return x, batch
}

func producedDeposit(t *testing.T, n byte) *ProducedMessage {
	t.Helper()
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.AccountUrl("bob", "tokens")
	txn.Body = &protocol.SyntheticDepositCredits{Amount: uint64(n)}
	return &ProducedMessage{
		Producer:    protocol.AccountUrl("alice", "tokens").WithTxID([32]byte{n}),
		Destination: txn.Header.Principal,
		Message:     &messaging.TransactionMessage{Transaction: txn},
	}
}

// seqNumberOf finds the sequence number produceSynthetic assigned to a
// message by probing for the sequenced form in the database — the number is
// part of the SequencedMessage's hash, so existence is proof of assignment.
func seqNumberOf(t *testing.T, batch *database.Batch, x *Executor, p *ProducedMessage, dest string, max uint64) uint64 {
	t.Helper()
	for n := uint64(1); n <= max; n++ {
		seq := &messaging.SequencedMessage{
			Message:     p.Message,
			Source:      x.Describe.NodeUrl(),
			Destination: protocol.PartitionUrl(dest),
			Number:      n,
		}
		_, err := batch.Message(seq.Hash()).Main().Get()
		if err == nil {
			return n
		}
	}
	t.Fatalf("message %x was never sequenced", p.Message.Hash())
	return 0
}

// Today's behaviour, stated as fact not preference: sequence numbers follow
// the order messages appear in the produced slice — which is delivery order,
// which under DAG-BFT depends on certificate arrival. This is exactly what
// the sorted block-end pass will replace, and what its equivalence test must
// reproduce over sorted input.
func TestCharacterize_InlineSequencingAssignsNumbersInDeliveryOrder(t *testing.T) {
	a, b, c := producedDeposit(t, 1), producedDeposit(t, 2), producedDeposit(t, 3)

	x1, batch1 := synthTestExecutor(t, routeEverythingTo("BVN1"))
	require.NoError(t, x1.produceSynthetic(batch1, []*ProducedMessage{a, b, c}, 1))
	require.Equal(t, uint64(1), seqNumberOf(t, batch1, x1, a, "BVN1", 3))
	require.Equal(t, uint64(2), seqNumberOf(t, batch1, x1, b, "BVN1", 3))
	require.Equal(t, uint64(3), seqNumberOf(t, batch1, x1, c, "BVN1", 3))

	// The same messages in a different delivery order get DIFFERENT numbers:
	// inline sequencing is not content-deterministic. This is the property
	// that makes execution order consensus-critical today.
	x2, batch2 := synthTestExecutor(t, routeEverythingTo("BVN1"))
	require.NoError(t, x2.produceSynthetic(batch2, []*ProducedMessage{c, a, b}, 1))
	assert.Equal(t, uint64(1), seqNumberOf(t, batch2, x2, c, "BVN1", 3))
	assert.Equal(t, uint64(2), seqNumberOf(t, batch2, x2, a, "BVN1", 3))
	assert.Equal(t, uint64(3), seqNumberOf(t, batch2, x2, b, "BVN1", 3))
}

// buildSynthTxn does destLedger.Produced++ inline, per message, as each is
// built — the counter and the assignment are the same operation today.
func TestCharacterize_LedgerProducedIncrementsInline(t *testing.T) {
	x, batch := synthTestExecutor(t, routeEverythingTo("BVN1"))

	produced := []*ProducedMessage{producedDeposit(t, 1), producedDeposit(t, 2), producedDeposit(t, 3)}
	require.NoError(t, x.produceSynthetic(batch, produced, 1))

	var ledger *protocol.SyntheticLedger
	require.NoError(t, batch.Account(x.Describe.Synthetic()).Main().GetAs(&ledger))
	assert.Equal(t, uint64(3), ledger.Partition(protocol.PartitionUrl("BVN1")).Produced,
		"Produced counts every message emitted to the destination")
}

// Numbers are per DESTINATION stream and contiguous within each: splitting a
// block's production across two partitions numbers each stream 1..n
// independently. The sorted pass must preserve this exactly.
func TestCharacterize_NumbersAreContiguousPerDestination(t *testing.T) {
	router := routeByAuthority{"bob.acme": "BVN1", "carol.acme": "BVN2"}
	x, batch := synthTestExecutor(t, router)

	toBob1, toBob2 := producedDeposit(t, 1), producedDeposit(t, 2)
	toCarol := producedDeposit(t, 3)
	carolTxn := new(protocol.Transaction)
	carolTxn.Header.Principal = protocol.AccountUrl("carol", "tokens")
	carolTxn.Body = &protocol.SyntheticDepositCredits{Amount: 3}
	toCarol.Destination = carolTxn.Header.Principal
	toCarol.Message = &messaging.TransactionMessage{Transaction: carolTxn}

	// Interleaved delivery: bob, carol, bob.
	require.NoError(t, x.produceSynthetic(batch, []*ProducedMessage{toBob1, toCarol, toBob2}, 1))

	assert.Equal(t, uint64(1), seqNumberOf(t, batch, x, toBob1, "BVN1", 3))
	assert.Equal(t, uint64(2), seqNumberOf(t, batch, x, toBob2, "BVN1", 3), "bob's stream is 1,2 — no gap for carol")
	assert.Equal(t, uint64(1), seqNumberOf(t, batch, x, toCarol, "BVN2", 3), "carol's stream starts at 1")

	var ledger *protocol.SyntheticLedger
	require.NoError(t, batch.Account(x.Describe.Synthetic()).Main().GetAs(&ledger))
	assert.Equal(t, uint64(2), ledger.Partition(protocol.PartitionUrl("BVN1")).Produced)
	assert.Equal(t, uint64(1), ledger.Partition(protocol.PartitionUrl("BVN2")).Produced)
}

// A producer-less message (system production) is sequenced like any other;
// only the producer→produced index entry is skipped. The sort key design in
// #4144 has to give these a defined home, so pin what they do today.
func TestCharacterize_NilProducerIsSequencedButNotIndexed(t *testing.T) {
	x, batch := synthTestExecutor(t, routeEverythingTo("BVN1"))

	p := producedDeposit(t, 1)
	p.Producer = nil
	require.NoError(t, x.produceSynthetic(batch, []*ProducedMessage{p}, 1))

	assert.Equal(t, uint64(1), seqNumberOf(t, batch, x, p, "BVN1", 1),
		"a system-produced message still takes the next sequence number")
}
