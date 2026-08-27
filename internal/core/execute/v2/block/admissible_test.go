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
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// isAdmissible answers "is this proven to have come from its source" for both
// the executor and staging (#4169 step 3). Staging needs it because a stream
// must not advance past a message that did not execute, and an unproven
// message does not execute.

// admissibleFixture returns a block and a hash that IS in the directory anchor
// chain.
func admissibleFixture(t *testing.T) (*Executor, *database.Batch, []byte) {
	t.Helper()
	x := streamTestExec(t)
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	known := make([]byte, 32)
	known[0] = 0xAB
	chain, err := batch.Account(x.Describe.AnchorPool()).
		AnchorChain(protocol.Directory).Root().Get()
	require.NoError(t, err)
	require.NoError(t, chain.AddEntry(known, false))

	return x, batch, known
}

func TestIsAdmissible_AnchorIsHeld(t *testing.T) {
	x, batch, known := admissibleFixture(t)

	ok, err := x.isAdmissible(batch, &protocol.AnnotatedReceipt{
		Receipt: &merkle.Receipt{Anchor: known},
	})
	require.NoError(t, err)
	assert.True(t, ok, "the proof terminates at an anchor we hold")
}

func TestIsAdmissible_AnchorHasNotArrived(t *testing.T) {
	x, batch, _ := admissibleFixture(t)

	missing := make([]byte, 32)
	missing[0] = 0xCD
	ok, err := x.isAdmissible(batch, &protocol.AnnotatedReceipt{
		Receipt: &merkle.Receipt{Anchor: missing},
	})
	require.NoError(t, err, "not-yet-anchored is an ANSWER, not an error — both callers have to act on it")
	assert.False(t, ok)
}

// A replica-accepted message (#4140) carries no proof of its own. Its proof was
// checked and absorbed into the stream's replica when it first arrived.
func TestIsAdmissible_NoProofIsReplicaAccepted(t *testing.T) {
	x, batch, _ := admissibleFixture(t)

	ok, err := x.isAdmissible(batch, nil)
	require.NoError(t, err)
	assert.True(t, ok, "no proof means replica-accepted, which is admissible by construction")
}

// A collection proof terminates at the same trust root as an individual one,
// so one check covers either form — including the continued-receipt case,
// where the terminal anchor is NOT the list's own receipt.
func TestIsAdmissible_CollectionProof(t *testing.T) {
	x, batch, known := admissibleFixture(t)

	ok, err := x.isAdmissible(batch, &protocol.AnnotatedReceipt{
		ReceiptList: &merkle.ReceiptList{Receipt: &merkle.Receipt{Anchor: known}},
	})
	require.NoError(t, err)
	assert.True(t, ok, "a collection proof terminates at the same trust root")

	other := make([]byte, 32)
	other[0] = 0xEF
	ok, err = x.isAdmissible(batch, &protocol.AnnotatedReceipt{
		ReceiptList: &merkle.ReceiptList{
			Receipt:          &merkle.Receipt{Anchor: other},
			ContinuedReceipt: &merkle.Receipt{Anchor: known},
		},
	})
	require.NoError(t, err)
	assert.True(t, ok, "when the list is continued, the CONTINUED receipt's anchor is the terminal one")
}

// An anchor's gate is a validator signature quorum, with a collection proof as
// a shortcut (#4169 step 3b). Staging needs one answer per message whichever
// kind of stream carries it, because an anchor that is not authorized never
// reaches the sequence check — so its stream must not advance over it.

// anchorFixture returns an executor whose BVN1 threshold is 2, a batch, a
// known directory anchor, and the anchor transaction.
func anchorFixture(t *testing.T) (*Executor, *database.Batch, []byte, *protocol.Transaction) {
	t.Helper()
	x, batch, known := admissibleFixture(t)

	// Three validators on BVN1 at a 2/3 accept threshold puts the quorum at 2.
	x.globals.Active.Globals = &protocol.NetworkGlobals{
		ValidatorAcceptThreshold: protocol.Rational{Numerator: 2, Denominator: 3},
	}
	x.globals.Active.Network = &protocol.NetworkDefinition{
		Validators: []*protocol.ValidatorInfo{
			{PublicKey: []byte{1}, Partitions: []*protocol.ValidatorPartitionInfo{{ID: "BVN1", Active: true}}},
			{PublicKey: []byte{2}, Partitions: []*protocol.ValidatorPartitionInfo{{ID: "BVN1", Active: true}}},
			{PublicKey: []byte{3}, Partitions: []*protocol.ValidatorPartitionInfo{{ID: "BVN1", Active: true}}},
		},
	}
	require.Equal(t, uint64(2), x.globals.Active.ValidatorThreshold("BVN1"), "fixture precondition")

	txn := new(protocol.Transaction)
	txn.Header.Principal = x.Describe.AnchorPool()
	txn.Body = new(protocol.BlockValidatorAnchor)
	return x, batch, known, txn
}

func addAnchorSigs(t *testing.T, batch *database.Batch, txn *protocol.Transaction, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		require.NoError(t, batch.Account(txn.Header.Principal).
			Transaction(txn.ID().Hash()).
			ValidatorSignatures().
			Add(&protocol.ED25519Signature{PublicKey: []byte{byte(i + 1)}, Signer: txn.Header.Principal}))
	}
}

func TestAnchorIsAdmissible_SignatureQuorum(t *testing.T) {
	x, batch, _, txn := anchorFixture(t)
	src := protocol.PartitionUrl("BVN1")

	ok, err := x.anchorIsAdmissible(batch, nil, txn, src)
	require.NoError(t, err)
	assert.False(t, ok, "no signatures at all is below the threshold")

	addAnchorSigs(t, batch, txn, 1)
	ok, err = x.anchorIsAdmissible(batch, nil, txn, src)
	require.NoError(t, err)
	assert.False(t, ok, "one of two is still below")

	addAnchorSigs(t, batch, txn, 2)
	ok, err = x.anchorIsAdmissible(batch, nil, txn, src)
	require.NoError(t, err)
	assert.True(t, ok, "at the threshold the anchor is authorized")
}

// A collection proof under a known directory root authorizes the anchor by
// itself (#4056) — no quorum needed.
func TestAnchorIsAdmissible_CollectionProofAuthorizesAlone(t *testing.T) {
	x, batch, known, txn := anchorFixture(t)

	ok, err := x.anchorIsAdmissible(batch, &protocol.AnnotatedReceipt{
		Receipt: &merkle.Receipt{Anchor: known},
	}, txn, protocol.PartitionUrl("BVN1"))
	require.NoError(t, err)
	assert.True(t, ok, "a proof under a known root stands in for the quorum, with zero signatures present")
}

// A proof whose anchor has NOT arrived must not reject the anchor — it falls
// through to the quorum, because healing resubmits until a current anchor
// extends our directory-root knowledge past the proven range.
func TestAnchorIsAdmissible_UnarrivedProofFallsThroughToTheQuorum(t *testing.T) {
	x, batch, _, txn := anchorFixture(t)
	src := protocol.PartitionUrl("BVN1")
	unknown := make([]byte, 32)
	unknown[0] = 0x77
	proof := &protocol.AnnotatedReceipt{Receipt: &merkle.Receipt{Anchor: unknown}}

	ok, err := x.anchorIsAdmissible(batch, proof, txn, src)
	require.NoError(t, err)
	assert.False(t, ok, "not authorized yet — but by the quorum, not by rejecting the proof")

	addAnchorSigs(t, batch, txn, 2)
	ok, err = x.anchorIsAdmissible(batch, proof, txn, src)
	require.NoError(t, err)
	assert.True(t, ok, "the quorum still authorizes it despite the unarrived proof")
}

func TestAnchorIsAdmissible_SourceMustBeAPartition(t *testing.T) {
	x, batch, _, txn := anchorFixture(t)
	_, err := x.anchorIsAdmissible(batch, nil, txn, protocol.AccountUrl("alice"))
	require.Error(t, err, "a non-partition source has no threshold to compare against")
}

// #4169 assumption 6.6: an anchor's positional run is safe because the quorum
// gate sits UPSTREAM of the sequence check — BlockAnchor.Process calls
// txnIsReady and records pending without ever reaching SequencedMessage, so an
// anchor that reaches the sequence check has already been authorized.
//
// That is read from the code rather than proven by it, so pin the half that
// can be: whichever way admissibility is asked, the two callers get the same
// answer for the same anchor. If they diverge, the positional run and the
// executor disagree about which anchors may execute.
func TestAnchorAdmissibility_OneAnswerForBothCallers(t *testing.T) {
	x, batch, known, txn := anchorFixture(t)
	src := protocol.PartitionUrl("BVN1")

	for _, c := range []struct {
		name  string
		proof *protocol.AnnotatedReceipt
		sigs  int
		want  bool
	}{
		{"no proof, no quorum", nil, 0, false},
		{"no proof, quorum met", nil, 2, true},
		{"proof under a known root", &protocol.AnnotatedReceipt{Receipt: &merkle.Receipt{Anchor: known}}, 0, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, b2, _, txn2 := anchorFixture(t)
			addAnchorSigs(t, b2, txn2, c.sigs)
			got, err := x.anchorIsAdmissible(b2, c.proof, txn2, src)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
	_ = batch
	_ = txn
}

// Admissibility is monotone: once an anchor is in the directory chain it stays
// there, because a chain is append-only. That is what lets a staged entry
// carry no admissibility flag (#4169 assumption 7.4) — admissible never
// becomes inadmissible, so a decision taken earlier cannot go stale.
func TestIsAdmissible_IsMonotone(t *testing.T) {
	x, batch, known := admissibleFixture(t)
	proof := &protocol.AnnotatedReceipt{Receipt: &merkle.Receipt{Anchor: known}}

	ok, err := x.isAdmissible(batch, proof)
	require.NoError(t, err)
	require.True(t, ok)

	// Extend the chain with unrelated anchors.
	chain, err := batch.Account(x.Describe.AnchorPool()).
		AnchorChain(protocol.Directory).Root().Get()
	require.NoError(t, err)
	for i := byte(1); i <= 5; i++ {
		h := make([]byte, 32)
		h[0] = 0x40 + i
		require.NoError(t, chain.AddEntry(h, false))
	}

	ok, err = x.isAdmissible(batch, proof)
	require.NoError(t, err)
	assert.True(t, ok, "an anchor already in the chain must stay admissible — the chain only grows")
}
