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

// The pre-pass (#4145) decides stream readiness serially, in arrival order,
// before any shard runs. These tests pin the boundary of what it may speak
// for.
//
// The boundary exists because a stream does not advance one message per
// delivery. When a message delivers, the cascade drains the ALREADY-RECEIVED
// tail behind it — messages sitting undelivered in the ledger's pending
// window — so the stream can run far past what the block's envelopes contain.
// The pre-pass cannot follow that without reimplementing the cascade, and it
// must not guess: guessing the drain happens is a wrong ADMIT, the one
// direction that corrupts a watermark.
//
// Both scenarios below were measured as disagreements between the pre-pass
// and the live check across the e2e suite before this rule existed.

func prepassSeq(t *testing.T, principal *url.URL, n uint64) *messaging.SequencedMessage {
	t.Helper()
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

// prepassBlock builds a BVN0 block whose synthetic ledger records `delivered`
// as the BVN1 stream's watermark and `received` as its high-water receipt —
// so numbers in (delivered, received] sit in the pending window, exactly as
// they would after arriving out of order in an earlier block.
func prepassBlock(t *testing.T, delivered, received uint64, pending ...uint64) *Block {
	t.Helper()
	// A real executor always has globals; streamOf consults the executor
	// version to decide whether a remote stub may be resolved. The earlier
	// version of this helper left them nil, which only worked because the
	// pre-pass carried its own classification rule that asked nothing.
	x := streamTestExec(t)

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	ledger := new(protocol.SyntheticLedger)
	ledger.Url = x.Describe.Synthetic()
	part := ledger.Partition(protocol.PartitionUrl("BVN1"))
	part.Delivered, part.Received = delivered, received
	part.Pending = make([]*url.TxID, received-delivered)
	for _, n := range pending {
		require.Greater(t, n, delivered)
		require.LessOrEqual(t, n, received)
		part.Pending[n-delivered-1] = prepassSeq(t, protocol.AccountUrl("alice", "tokens"), n).ID()
	}
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))

	return &Block{Batch: batch, Executor: x}
}

func prepassEnvelopes(seqs ...*messaging.SequencedMessage) []*messaging.Envelope {
	envs := make([]*messaging.Envelope, len(seqs))
	for i, seq := range seqs {
		envs[i] = &messaging.Envelope{Messages: []messaging.Message{
			&messaging.SyntheticMessage{Message: seq},
		}}
	}
	return envs
}

// The plain case, and the reason the pre-pass exists: a run arriving in order
// on a stream with nothing pending behind it. Every message is decided here,
// and the answers are the ones serial execution would produce.
func TestPrepass_DecidesAnUncontestedRun(t *testing.T) {
	b := prepassBlock(t, 0, 0)
	alice := protocol.AccountUrl("alice", "tokens")
	one, two, four := prepassSeq(t, alice, 1), prepassSeq(t, alice, 2), prepassSeq(t, alice, 4)

	b.decideSequencedReadiness(prepassEnvelopes(one, two, four))

	ready, ok := b.seqReadyFor(one.Hash())
	assert.True(t, ok && ready, "#1 is next on an empty stream")
	ready, ok = b.seqReadyFor(two.Hash())
	assert.True(t, ok && ready, "#2 is next once the pass has admitted #1")
	ready, ok = b.seqReadyFor(four.Hash())
	assert.True(t, ok && !ready, "#3 is missing, so #4 is a gap the pass may refuse")
}

// Measured on the e2e suite: BVN1→BVN0 at Delivered=0 with #2 and #3 already
// received, envelopes carrying #1 and #4. The pass admitted #1; the tail
// drained to 3 inline behind it; and #4 — refused against a watermark of 1 —
// was next by the time it ran.
//
// The pass must not answer for #4 at all. Admitting it would be a guess about
// a drain it cannot see; refusing it is a lost delivery the moment the ledger
// write follows this verdict.
func TestPrepass_StopsSpeakingWhenAnAdmittedMessageHasAReceivedTail(t *testing.T) {
	b := prepassBlock(t, 0, 3, 2, 3)
	alice := protocol.AccountUrl("alice", "tokens")
	one, four := prepassSeq(t, alice, 1), prepassSeq(t, alice, 4)

	b.decideSequencedReadiness(prepassEnvelopes(one, four))

	ready, ok := b.seqReadyFor(one.Hash())
	assert.True(t, ok && ready, "#1 is still next: the drain happens BEHIND it, not before")

	_, ok = b.seqReadyFor(four.Hash())
	assert.False(t, ok, "the tail drains to 3 during execution — the pass cannot see that, so it must not answer")
}

// The same hazard reached through the block's own refusals. Envelopes arrive
// [#4, #1, #2, #3] on an empty stream: #4 is refused — correctly, against a
// watermark of 0 — and the executor records it pending, which puts it in the
// very window the drain reaches into. #3's delivery then cascades into it.
//
// So a refusal is not permanently safe either. When the stream becomes
// undetermined the pass must TAKE BACK every refusal it made on that stream,
// because each one may be revisited by the drain with the same message hash.
func TestPrepass_WithdrawsRefusalsTheDrainCanReach(t *testing.T) {
	b := prepassBlock(t, 0, 0)
	alice := protocol.AccountUrl("alice", "tokens")
	four := prepassSeq(t, alice, 4)
	one, two, three := prepassSeq(t, alice, 1), prepassSeq(t, alice, 2), prepassSeq(t, alice, 3)

	b.decideSequencedReadiness(prepassEnvelopes(four, one, two, three))

	for _, seq := range []*messaging.SequencedMessage{one, two, three} {
		ready, ok := b.seqReadyFor(seq.Hash())
		assert.Truef(t, ok && ready, "#%d is next at its own arrival", seq.Number)
	}

	_, ok := b.seqReadyFor(four.Hash())
	assert.False(t, ok, "#4's refusal is withdrawn: #3's delivery cascades into the pending slot #4 now occupies")
}

// Anchors and synthetics keep separate ledgers, so one stream going
// undetermined must not silence the other.
func TestPrepass_UndeterminedIsPerStream(t *testing.T) {
	b := prepassBlock(t, 0, 2, 2)
	alice := protocol.AccountUrl("alice", "tokens")
	one, three := prepassSeq(t, alice, 1), prepassSeq(t, alice, 3)

	// A second source, untouched by the first stream's pending tail.
	other := prepassSeq(t, alice, 1)
	other.Source = protocol.PartitionUrl("BVN2")

	b.decideSequencedReadiness(prepassEnvelopes(one, three, other))

	_, ok := b.seqReadyFor(three.Hash())
	assert.False(t, ok, "BVN1's stream is undetermined once #1 is admitted over a received #2")

	ready, ok := b.seqReadyFor(other.Hash())
	assert.True(t, ok && ready, "BVN2's stream is a different stream and is still fully determined")
}
