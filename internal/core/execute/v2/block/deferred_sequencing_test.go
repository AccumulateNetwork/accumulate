// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4144: sequencing is deferred to one sorted pass at block end. The sort
// key — (producer transaction ID, emission index), system-produced messages
// after all attributed ones — is derivable from the produced set alone, so
// sequence numbers no longer depend on delivery order, which under sharded
// execution (#4145) is a scheduling accident.

// producedBy builds a ProducedMessage attributed to the given producer hash,
// carrying a distinguishable body.
func producedBy(producer byte, n byte) *ProducedMessage {
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.AccountUrl("bob", "tokens")
	txn.Body = &protocol.SyntheticDepositCredits{Amount: uint64(producer)<<8 | uint64(n)}
	return &ProducedMessage{
		Producer:    protocol.AccountUrl("alice", "tokens").WithTxID([32]byte{producer}),
		Destination: txn.Header.Principal,
		Message:     &messaging.TransactionMessage{Transaction: txn},
	}
}

func systemProduced(n byte) *ProducedMessage {
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.AcmeUrl()
	txn.Body = &protocol.SyntheticBurnTokens{}
	txn.Header.Memo = string(rune('a' + n)) // distinguish the messages
	return &ProducedMessage{
		Destination: txn.Header.Principal,
		Message:     &messaging.TransactionMessage{Transaction: txn},
	}
}

func TestSortProduced_KeyIsProducerTxIdThenIndex(t *testing.T) {
	// Producer 2's messages, then producer 1's — the sort orders by producer
	// ID first, and within a producer by emission order.
	p1a, p1b := producedBy(1, 1), producedBy(1, 2)
	p2a, p2b := producedBy(2, 1), producedBy(2, 2)

	got := []*ProducedMessage{p2a, p1a, p2b, p1b}
	sortProduced(got)
	assert.Equal(t, []*ProducedMessage{p1a, p1b, p2a, p2b}, got,
		"the key is (producer TxID, emission index)")
}

// The property the whole phase rests on: any delivery order yields one
// canonical output. Table-driven over many permutations.
func TestSortProduced_IndependentOfDeliveryOrder(t *testing.T) {
	msgs := []*ProducedMessage{
		producedBy(1, 1), producedBy(1, 2), producedBy(1, 3),
		producedBy(2, 1), producedBy(2, 2),
		producedBy(3, 1),
		systemProduced(0), systemProduced(1),
	}
	canonical := append([]*ProducedMessage(nil), msgs...)
	sortProduced(canonical)

	rng := rand.New(rand.NewSource(4144))
	for perm := 0; perm < 100; perm++ {
		shuffled := append([]*ProducedMessage(nil), msgs...)
		// Shuffle DELIVERIES, not messages: each producer's messages keep
		// their emitted order, because a producer's output is appended
		// atomically when its delivery executes. Shuffling individual
		// messages would break an invariant the accumulator provides.
		byProducer := map[string][]*ProducedMessage{}
		var producers []string
		for _, m := range msgs {
			key := "system"
			if m.Producer != nil {
				key = m.Producer.String()
			}
			if len(byProducer[key]) == 0 {
				producers = append(producers, key)
			}
			byProducer[key] = append(byProducer[key], m)
		}
		rng.Shuffle(len(producers), func(i, j int) { producers[i], producers[j] = producers[j], producers[i] })
		shuffled = shuffled[:0]
		for _, p := range producers {
			shuffled = append(shuffled, byProducer[p]...)
		}

		sortProduced(shuffled)
		require.Equal(t, canonical, shuffled,
			"permutation %d: the sorted order must be canonical regardless of delivery order", perm)
	}
}

func TestSortProduced_StableForRepeatedProducers(t *testing.T) {
	a, b, c := producedBy(7, 1), producedBy(7, 2), producedBy(7, 3)
	got := []*ProducedMessage{a, b, c}
	sortProduced(got)
	assert.Equal(t, []*ProducedMessage{a, b, c}, got,
		"one producer's messages keep their emitted order — the stable sort IS the index half of the key")
}

func TestSortProduced_NilProducerOrdersAfterAttributedMessages(t *testing.T) {
	sys := systemProduced(0)
	usr := producedBy(0xFF, 1) // even the largest producer ID sorts before system
	got := []*ProducedMessage{sys, usr}
	sortProduced(got)
	assert.Equal(t, []*ProducedMessage{usr, sys}, got,
		"system-produced messages have no producer and order after every attributed one")
}

func TestSortProduced_NilProducerGroupIsItselfOrdered(t *testing.T) {
	s0, s1, s2 := systemProduced(0), systemProduced(1), systemProduced(2)
	want := []*ProducedMessage{s0, s1, s2}
	sortProduced(want) // canonical order of the group, by message hash

	got := []*ProducedMessage{s2, s0, s1}
	sortProduced(got)
	assert.Equal(t, want, got,
		"producer-less messages have a fixed key of their own (message hash) — not arrival order")
}

func TestSortProduced_EmptyProducedSetSortsWithoutError(t *testing.T) {
	assert.NotPanics(t, func() {
		sortProduced(nil)
		sortProduced([]*ProducedMessage{})
	})
}

// runDeferred accumulates the deliveries the way exec_process does, then
// runs the block-end pass: sort, then one produceSynthetic call.
func runDeferred(t *testing.T, deliveries [][]*ProducedMessage) (*Executor, *database.Batch) {
	t.Helper()
	x, batch := synthTestExecutor(t, routeEverythingTo("BVN1"))
	var accumulated []*ProducedMessage
	for _, d := range deliveries {
		accumulated = append(accumulated, d...)
	}
	sortProduced(accumulated)
	require.NoError(t, x.produceSynthetic(batch, accumulated, 1))
	return x, batch
}

func TestDeferredSequencing_NumbersAssignedInSortedOrder(t *testing.T) {
	p1, p2a, p2b := producedBy(1, 1), producedBy(2, 1), producedBy(2, 2)

	// Delivered 2 before 1 — numbers still follow the SORTED order.
	x, batch := runDeferred(t, [][]*ProducedMessage{{p2a, p2b}, {p1}})
	assert.Equal(t, uint64(1), seqNumberOf(t, batch, x, p1, "BVN1", 3))
	assert.Equal(t, uint64(2), seqNumberOf(t, batch, x, p2a, "BVN1", 3))
	assert.Equal(t, uint64(3), seqNumberOf(t, batch, x, p2b, "BVN1", 3))
}

// Same block, shuffled deliveries, same numbers for the same messages — the
// deferred pass's whole reason to exist.
func TestDeferredSequencing_NumbersAreIndependentOfDeliveryOrder(t *testing.T) {
	mk := func() (a, b, c, d *ProducedMessage) {
		return producedBy(1, 1), producedBy(1, 2), producedBy(2, 1), producedBy(3, 1)
	}

	orders := [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	for _, order := range orders {
		a, b, c, d := mk()
		deliveries := [][]*ProducedMessage{{a, b}, {c}, {d}}
		permuted := make([][]*ProducedMessage, len(deliveries))
		for i, j := range order {
			permuted[i] = deliveries[j]
		}
		x, batch := runDeferred(t, permuted)
		assert.Equal(t, uint64(1), seqNumberOf(t, batch, x, a, "BVN1", 4), "order %v", order)
		assert.Equal(t, uint64(2), seqNumberOf(t, batch, x, b, "BVN1", 4), "order %v", order)
		assert.Equal(t, uint64(3), seqNumberOf(t, batch, x, c, "BVN1", 4), "order %v", order)
		assert.Equal(t, uint64(4), seqNumberOf(t, batch, x, d, "BVN1", 4), "order %v", order)
	}
}

func TestDeferredSequencing_NumbersAreContiguousPerDestination(t *testing.T) {
	router := routeByAuthority{"bob.acme": "BVN1", "carol.acme": "BVN2"}
	x, batch := synthTestExecutor(t, router)

	toBob1, toBob2 := producedBy(1, 1), producedBy(2, 1)
	toCarol := producedBy(1, 2)
	carolTxn := new(protocol.Transaction)
	carolTxn.Header.Principal = protocol.AccountUrl("carol", "tokens")
	carolTxn.Body = &protocol.SyntheticDepositCredits{Amount: 99}
	toCarol.Destination = carolTxn.Header.Principal
	toCarol.Message = &messaging.TransactionMessage{Transaction: carolTxn}

	all := []*ProducedMessage{toBob2, toCarol, toBob1}
	sortProduced(all)
	require.NoError(t, x.produceSynthetic(batch, all, 1))

	assert.Equal(t, uint64(1), seqNumberOf(t, batch, x, toBob1, "BVN1", 3))
	assert.Equal(t, uint64(2), seqNumberOf(t, batch, x, toBob2, "BVN1", 3), "bob's stream has no gap for carol")
	assert.Equal(t, uint64(1), seqNumberOf(t, batch, x, toCarol, "BVN2", 3))
}

func TestDeferredSequencing_LedgerProducedCounterMatchesEmittedCount(t *testing.T) {
	x, batch := runDeferred(t, [][]*ProducedMessage{
		{producedBy(1, 1), producedBy(1, 2)}, {producedBy(2, 1)},
	})
	var ledger *protocol.SyntheticLedger
	require.NoError(t, batch.Account(x.Describe.Synthetic()).Main().GetAs(&ledger))
	assert.Equal(t, uint64(3), ledger.Partition(protocol.PartitionUrl("BVN1")).Produced)
}

// The key regression test: over identical input in canonical order, the
// deferred pass produces EXACTLY what inline sequencing produced — same
// sequence numbers, same synthetic ledger, same chain, same state root. The
// move is behaviour-preserving; only the ORDER, where delivery order used to
// leak in, is newly pinned. The inline half is the Stage 0 characterization
// (TestCharacterize_InlineSequencingAssignsNumbersInDeliveryOrder) run live.
func TestDeferredSequencingMatchesInlineSequencing(t *testing.T) {
	mk := func() [][]*ProducedMessage {
		return [][]*ProducedMessage{
			{producedBy(1, 1), producedBy(1, 2)},
			{producedBy(2, 1)},
			{producedBy(3, 1), producedBy(3, 2), producedBy(3, 3)},
		}
	}

	// Inline: one produceSynthetic call per delivery, in delivery order —
	// exactly what exec_process.go did before #4144. Deliveries arrive in
	// canonical order, the case the two implementations must agree on.
	inlineX, inlineBatch := synthTestExecutor(t, routeEverythingTo("BVN1"))
	for _, d := range mk() {
		require.NoError(t, inlineX.produceSynthetic(inlineBatch, d, 1))
	}

	// Deferred: accumulate, sort, sequence once.
	deferredX, deferredBatch := runDeferred(t, mk())

	// Same numbers for the same messages.
	for _, d := range mk() {
		for _, p := range d {
			in := seqNumberOf(t, inlineBatch, inlineX, p, "BVN1", 6)
			def := seqNumberOf(t, deferredBatch, deferredX, p, "BVN1", 6)
			require.Equal(t, in, def, "message %x must get the same sequence number", p.Message.Hash())
		}
	}

	// Same ledger state.
	var inLedger, defLedger *protocol.SyntheticLedger
	require.NoError(t, inlineBatch.Account(inlineX.Describe.Synthetic()).Main().GetAs(&inLedger))
	require.NoError(t, deferredBatch.Account(deferredX.Describe.Synthetic()).Main().GetAs(&defLedger))
	require.True(t, inLedger.Equal(defLedger), "the synthetic ledger must be identical")

	// Same chain — entry for entry, in order.
	inChain, err := inlineBatch.Account(inlineX.Describe.Synthetic()).MainChain().Get()
	require.NoError(t, err)
	defChain, err := deferredBatch.Account(deferredX.Describe.Synthetic()).MainChain().Get()
	require.NoError(t, err)
	require.Equal(t, inChain.Height(), defChain.Height())
	inEntries, err := inChain.Entries(0, inChain.Height())
	require.NoError(t, err)
	defEntries, err := defChain.Entries(0, defChain.Height())
	require.NoError(t, err)
	require.Equal(t, inEntries, defEntries, "the synthetic main chain must be identical, entry for entry")

	// Same state root.
	inRoot, err := inlineBatch.GetBptRootHash()
	require.NoError(t, err)
	defRoot, err := deferredBatch.GetBptRootHash()
	require.NoError(t, err)
	assert.Equal(t, inRoot, defRoot, "the state trees must agree")
}
