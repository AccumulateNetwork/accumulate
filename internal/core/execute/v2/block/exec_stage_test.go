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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Tests for the staging pieces' stated assumptions (#4169). Written after a
// review found the plan had proofs per step but no ledger of assumptions, and
// that seven of them had already turned out false.

func stageTestBlock(t *testing.T) *Block {
	t.Helper()
	x := streamTestExec(t)
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	return &Block{Batch: batch, Executor: x}
}

func synthEnv(n uint64, principal *url.URL) *messaging.Envelope {
	txn := new(protocol.Transaction)
	txn.Header.Principal = principal
	txn.Body = &protocol.SyntheticDepositCredits{Amount: n}
	return &messaging.Envelope{Messages: []messaging.Message{
		&messaging.SyntheticMessage{Message: &messaging.SequencedMessage{
			Message:     &messaging.TransactionMessage{Transaction: txn},
			Source:      protocol.PartitionUrl("BVN1"),
			Destination: protocol.PartitionUrl("BVN0"),
			Number:      n,
		}},
	}}
}

// userEnv builds a SIGNED user envelope. Unsigned is not merely unrealistic:
// Envelope.Normalize refuses it ("transaction X is not signed"), and classify
// skips an envelope it cannot normalize — so an unsigned fixture tests the
// skip path while looking like it tests the user path.
func userEnv(principal *url.URL) *messaging.Envelope {
	txn := new(protocol.Transaction)
	txn.Header.Principal = principal
	txn.Body = &protocol.SendTokens{}
	sig := &protocol.ED25519Signature{
		PublicKey:       []byte{1, 2, 3},
		Signer:          principal,
		SignerVersion:   1,
		Timestamp:       1,
		TransactionHash: txn.ID().Hash(),
	}
	return &messaging.Envelope{Messages: []messaging.Message{
		&messaging.TransactionMessage{Transaction: txn},
		&messaging.SignatureMessage{Signature: sig, TxID: txn.ID()},
	}}
}

// An envelope classify cannot normalize is not a user envelope and not an
// arrival — it is simply absent from staging. It is NOT lost: ProcessAll's
// envelope loop walks every envelope and skips only those already run, so an
// unnormalizable one still reaches Process and reports its own error.
func TestClassify_SkipsAnEnvelopeItCannotNormalize(t *testing.T) {
	b := stageTestBlock(t)
	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.AccountUrl("alice", "tokens")
	txn.Body = &protocol.SendTokens{}
	unsigned := &messaging.Envelope{Messages: []messaging.Message{
		&messaging.TransactionMessage{Transaction: txn},
	}}

	c := b.classify([]*messaging.Envelope{unsigned})
	assert.Empty(t, c.user, "an envelope that will not normalize is not classified as anything")
	assert.Empty(t, c.streams)
}

// An envelope carrying no stream message is a user envelope; one carrying a
// stream message is not, however many other messages it holds.
func TestClassify_SeparatesUserEnvelopes(t *testing.T) {
	b := stageTestBlock(t)
	alice := protocol.AccountUrl("alice", "tokens")

	c := b.classify([]*messaging.Envelope{
		userEnv(alice),
		synthEnv(1, alice),
		userEnv(alice),
	})

	assert.Equal(t, []int{0, 2}, c.user, "only envelopes with no stream message are user envelopes")
	require.Len(t, c.streams, 1)
	for _, arrivals := range c.arrivals {
		require.Len(t, arrivals, 1)
		require.Equal(t, 1, arrivals[1].envIdx, "an arrival remembers the envelope it came in")
	}
}

// Requirement 4, apply once: the same sequenced message can appear in more
// than one envelope of a block. It must be classified once, and the FIRST
// sighting must win so the choice does not depend on map order.
func TestClassify_TheSameMessageTwiceIsOneArrival(t *testing.T) {
	b := stageTestBlock(t)
	alice := protocol.AccountUrl("alice", "tokens")

	c := b.classify([]*messaging.Envelope{
		synthEnv(7, alice),
		synthEnv(7, alice),
	})

	require.Len(t, c.streams, 1)
	for _, arrivals := range c.arrivals {
		require.Len(t, arrivals, 1, "one message, one arrival")
		assert.Equal(t, 0, arrivals[7].envIdx, "the first sighting wins")
	}
}

// Anchors and synthetics from one source are SEPARATE streams. Classifying
// them together would let an anchor's position gate a synthetic's.
func TestClassify_AnchorAndSyntheticAreSeparateStreams(t *testing.T) {
	b := stageTestBlock(t)
	alice := protocol.AccountUrl("alice", "tokens")

	anchorTxn := new(protocol.Transaction)
	anchorTxn.Header.Principal = protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)
	anchorTxn.Body = new(protocol.BlockValidatorAnchor)
	anchorEnv := &messaging.Envelope{Messages: []messaging.Message{
		&messaging.BlockAnchor{Anchor: &messaging.SequencedMessage{
			Message:     &messaging.TransactionMessage{Transaction: anchorTxn},
			Source:      protocol.PartitionUrl("BVN1"),
			Destination: protocol.PartitionUrl("BVN0"),
			Number:      1,
		}},
	}}

	c := b.classify([]*messaging.Envelope{synthEnv(1, alice), anchorEnv})

	require.Len(t, c.streams, 2, "same source, but two streams")
	kinds := map[streamKind]bool{}
	for _, s := range c.streams {
		kinds[s.kind] = true
	}
	assert.True(t, kinds[streamAnchor] && kinds[streamSynthetic])
}

// The canonical stream order: anchors before synthetics, the directory before
// partitions, then by partition. Any fixed rule would do — what matters is
// that it is total and the same everywhere.
func TestLessStream_CanonicalOrder(t *testing.T) {
	mk := func(k streamKind, src *url.URL) stream {
		return stream{kind: k, ledger: protocol.PartitionUrl("BVN0"), source: src}
	}
	dn := protocol.DnUrl()
	bvn0 := protocol.PartitionUrl("BVN0")
	bvn1 := protocol.PartitionUrl("BVN1")

	assert.True(t, lessStream(mk(streamAnchor, bvn1), mk(streamSynthetic, dn)),
		"anchors come before synthetics regardless of source")
	assert.True(t, lessStream(mk(streamSynthetic, dn), mk(streamSynthetic, bvn0)),
		"the directory comes before the partitions")
	assert.True(t, lessStream(mk(streamSynthetic, bvn0), mk(streamSynthetic, bvn1)),
		"partitions in order")

	// Totality: never both ways round.
	for _, a := range []stream{mk(streamAnchor, dn), mk(streamSynthetic, bvn0), mk(streamSynthetic, bvn1)} {
		for _, b := range []stream{mk(streamAnchor, dn), mk(streamSynthetic, bvn0), mk(streamSynthetic, bvn1)} {
			if lessStream(a, b) {
				assert.False(t, lessStream(b, a), "the order must be antisymmetric")
			}
		}
	}
}

// #4169 assumption 6.4, FALSIFIED and pinned here so it cannot quietly come
// back. anyPending is what executeRuns uses to decide whether a run entry
// delivered — and it cannot see an ERROR status. MessageIsReady turns a
// missing message into protocol.NewErrorStatus, which is neither an error
// return nor a pending status, so a run entry that achieved nothing is
// counted as progress. Measured: drain rounds reported 80 deliveries each and
// ran to the round bound while the ledger did not move at all.
//
// The fix is to measure progress from the LEDGER, not from statuses. Until
// then this test states exactly what the signal is worth.
func TestAnyPending_CannotSeeAnErrorStatus(t *testing.T) {
	pending := new(protocol.TransactionStatus)
	pending.Code = errors.Pending
	assert.True(t, anyPending([]*protocol.TransactionStatus{pending}))

	delivered := new(protocol.TransactionStatus)
	delivered.Code = errors.Delivered
	assert.False(t, anyPending([]*protocol.TransactionStatus{delivered}))

	assert.False(t, anyPending(nil),
		"no statuses reads as success — a run entry that returns nothing is indistinguishable from one that delivered")

	failed := protocol.NewErrorStatus(
		protocol.AccountUrl("alice").WithTxID([32]byte{1}),
		errors.NotFound.With("message not found"))
	assert.False(t, anyPending([]*protocol.TransactionStatus{failed}),
		"THE DEFECT: an error status is not pending, so a run entry that failed outright counts as delivered")
}
