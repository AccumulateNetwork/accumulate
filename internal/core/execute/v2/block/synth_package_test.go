// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	dagconfig "gitlab.com/accumulatenetwork/accumulate/pkg/consensus/config"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4141: the package budget and the accept-side sibling-proof lookup.

// The budget is DERIVED from the transport's batch limit — no literal
// survives. Main's 3 MiB constant came from CometBFT's max_tx_bytes and was
// six times this transport's entire batch limit; changing the limit must
// move the budget.
func TestSynthPackage_BudgetDerivedFromMaxBatchBytes(t *testing.T) {
	x := new(Executor)

	x.MaxEnvelopeSize = 100_000
	assert.Equal(t, 75_000, x.synthPackageBudget(), "the budget moves with the limit")

	x.MaxEnvelopeSize = 200_000
	assert.Equal(t, 150_000, x.synthPackageBudget())

	// Unset falls back to the DAG-BFT default limit — still derived, and
	// still strictly below it: a full package plus proof state, continuation,
	// signatures and framing must fit in ONE batch.
	x.MaxEnvelopeSize = 0
	assert.Less(t, x.synthPackageBudget(), dagconfig.DefaultMaxBatchBytes)
	assert.Equal(t, dagconfig.DefaultMaxBatchBytes-dagconfig.DefaultMaxBatchBytes/4, x.synthPackageBudget())
}

// A 4096-element proof is ~128 KB of hashes on its own. The packing loop
// charges span*32 per candidate, and the budget reserves headroom for the
// rest of the proof — so the proof is accounted for, not discovered.
func TestSynthPackage_ProofAloneIsAccountedFor(t *testing.T) {
	x := new(Executor)
	x.MaxEnvelopeSize = dagconfig.DefaultMaxBatchBytes

	budget := x.synthPackageBudget()
	proofHashes := protocol.MaxReceiptListElements * 32
	assert.Greater(t, x.MaxEnvelopeSize-budget, 0,
		"headroom exists for what is not known until the package closes")
	assert.Less(t, proofHashes, dagconfig.DefaultMaxBatchBytes,
		"a max-span proof's hashes alone are ~128 KB — a quarter of the default batch — which is why the span is charged per message rather than assumed")
}

// packageCtx builds a message context whose bundle carries the given
// messages — the shape findProofInBundle sees.
func packageCtx(msgs ...messaging.Message) *MessageContext {
	d := &bundle{Block: &Block{Executor: new(Executor)}, messages: msgs}
	return &MessageContext{bundle: d, message: msgs[len(msgs)-1]}
}

func testList(t *testing.T, hashes ...[32]byte) *merkle.ReceiptList {
	t.Helper()
	list := merkle.NewReceiptList()
	list.MerkleState = new(merkle.State)
	for _, h := range hashes {
		list.Elements = append(list.Elements, h[:])
	}
	return list
}

func TestFindProofInBundle_MatchesByInclusionNotPosition(t *testing.T) {
	mine := sha256.Sum256([]byte("mine"))
	other := sha256.Sum256([]byte("someone else's, interleaved on the source chain"))

	proof := &messaging.SyntheticProof{Proof: &protocol.AnnotatedReceipt{
		ReceiptList: testList(t, other, mine, other),
	}}
	ctx := packageCtx(proof)

	got := findProofInBundle(ctx, mine)
	require.NotNil(t, got, "the span may cover other destinations' elements — matching is by inclusion")
	assert.Same(t, proof.Proof, got)
}

func TestFindProofInBundle_ReturnsNilWhenNoProofCoversTheMessage(t *testing.T) {
	covered := sha256.Sum256([]byte("covered"))
	mine := sha256.Sum256([]byte("mine"))

	ctx := packageCtx(&messaging.SyntheticProof{Proof: &protocol.AnnotatedReceipt{
		ReceiptList: testList(t, covered),
	}})
	assert.Nil(t, findProofInBundle(ctx, mine),
		"a proof from another package must not cover this message — packages are self-contained")
}

func TestFindProofInBundle_IgnoresListLongerThanMaxReceiptListElements(t *testing.T) {
	mine := sha256.Sum256([]byte("mine"))
	list := testList(t, mine)
	for i := 0; i < protocol.MaxReceiptListElements; i++ {
		h := sha256.Sum256([]byte{byte(i), byte(i >> 8)})
		list.Elements = append(list.Elements, h[:])
	}

	ctx := packageCtx(&messaging.SyntheticProof{Proof: &protocol.AnnotatedReceipt{ReceiptList: list}})
	assert.Nil(t, findProofInBundle(ctx, mine),
		"bound before hashing — a sibling must not depend on the proof's own executor having rejected it first")
}

func TestFindProofInBundle_PicksTheCoveringProofWhenAnEnvelopeCarriesSeveral(t *testing.T) {
	a := sha256.Sum256([]byte("a"))
	b := sha256.Sum256([]byte("b"))

	proofA := &messaging.SyntheticProof{Proof: &protocol.AnnotatedReceipt{ReceiptList: testList(t, a)}}
	proofB := &messaging.SyntheticProof{Proof: &protocol.AnnotatedReceipt{ReceiptList: testList(t, b)}}
	ctx := packageCtx(proofA, proofB)

	assert.Same(t, proofB.Proof, findProofInBundle(ctx, b))
	assert.Same(t, proofA.Proof, findProofInBundle(ctx, a))
}

// A package member — signed, proof-less — is accepted by resolving the
// SyntheticProof travelling in the same envelope, and the resolution is
// confined to that envelope. This also pins precedence: with a signature
// present, the bundle proof path is used, not the replica's.
func TestPackageMember_AcceptedViaBundleProof(t *testing.T) {
	f := newReplicaFixture(t, 0)
	f.x.globals = &Globals{Active: core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionLatest}}

	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.AccountUrl("alice", "tokens")
	txn.Body = &protocol.SyntheticDepositCredits{Amount: 1}
	seq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: txn},
		Source:      protocol.PartitionUrl("BVN1"),
		Destination: protocol.PartitionUrl("BVN0"),
		Number:      1,
	}
	h := seq.Hash()
	require.NoError(t, f.chain.AddEntry(h[:], false))
	list := f.proof(t, 0, 0)

	proofMsg := &messaging.SyntheticProof{Proof: &protocol.AnnotatedReceipt{
		ReceiptList: list,
		Anchor:      &protocol.AnchorMetadata{Account: protocol.DnUrl()},
	}}
	member := &messaging.SyntheticMessage{
		Message:   seq,
		Signature: &protocol.ED25519Signature{PublicKey: make([]byte, 32), Signer: protocol.DnUrl().JoinPath(protocol.Network)},
	}

	d := &bundle{Block: &Block{Executor: f.x, Batch: f.batch}, batch: f.batch,
		messages: []messaging.Message{proofMsg, member}}
	ctx := &MessageContext{bundle: d, message: member}

	syn, err := SyntheticMessage{}.check(f.batch, ctx)
	require.NoError(t, err, "a package member resolves its proof from the bundle")
	require.NotNil(t, syn.Proof)
	assert.Same(t, list, syn.Proof.ReceiptList, "the bundle's proof, not a rebuilt one")

	// Alone — outside its package — the same signed member is refused: the
	// replica does not vouch for it (nothing was absorbed), and proofs are
	// never borrowed across envelopes.
	d2 := &bundle{Block: &Block{Executor: f.x, Batch: f.batch}, batch: f.batch,
		messages: []messaging.Message{member}}
	_, err = SyntheticMessage{}.check(f.batch, &MessageContext{bundle: d2, message: member})
	require.ErrorContains(t, err, "missing proof")
}
