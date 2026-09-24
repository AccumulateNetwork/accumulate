// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Anchor staging (executor spec, "Anchor staging"): a collection proof waits
// under the Directory anchor block it terminates in until that anchor executes
// here; the anchor then validates it (its range becomes proven) or disproves
// it (it is discarded and counted). A proof whose anchor already executed is
// decided at intake.

type anchorStagingFixture struct {
	*stagingFixture
	terminal []byte
}

func newAnchorStagingFixture(t *testing.T) *anchorStagingFixture {
	t.Helper()
	f := newStagingFixture(t, 3)
	rootChain, err := f.batch.Account(protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)).RootChain().Get()
	require.NoError(t, err)
	require.NoError(t, rootChain.AddEntry(f.chain.Anchor(), false))
	return &anchorStagingFixture{stagingFixture: f, terminal: rootChain.Anchor()}
}

// siblingsOf returns the hashes of the source chain's entries in [start, end]:
// what the sequenced messages travelling with such a proof would hash to.
func (f *anchorStagingFixture) siblingsOf(start, end int) [][]byte {
	return f.src[start : end+1]
}

// proofFor builds a valid package proof over [start, end] of the source chain,
// continued to the fixture's root, naming Directory anchor block anchorBlock.
func (f *anchorStagingFixture) proofFor(t *testing.T, start, end int64, anchorBlock uint64) *protocol.AnnotatedReceipt {
	t.Helper()
	list := f.proof(t, start, end)
	rootChain, err := f.batch.Account(protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)).RootChain().Get()
	require.NoError(t, err)
	cont, err := rootChain.Receipt(0, 0)
	require.NoError(t, err)
	list.ContinuedReceipt = cont
	require.True(t, list.Validate(nil))
	return &protocol.AnnotatedReceipt{ReceiptList: list, Anchor: directoryAnchorMetadata(anchorBlock)}
}

// anchorExecutes is what executing a DirectoryAnchor does to the destination:
// its root lands on the Directory anchor chain, the block records it, and the
// newest executed anchor block advances.
func (f *anchorStagingFixture) anchorExecutes(t *testing.T, block uint64, root []byte) {
	t.Helper()
	chain2 := f.batch.Account(f.x.Describe.AnchorPool()).AnchorChain(protocol.Directory).Root()
	c, err := chain2.Get()
	require.NoError(t, err)
	require.NoError(t, c.AddEntry(root, false))
	require.NoError(t, f.batch.Account(f.x.Describe.AnchorPool()).DirectoryAnchorBlock().Put(block))
	body := &protocol.DirectoryAnchor{}
	body.MinorBlockIndex = block
	copy(body.RootChainAnchor[:], root)
	f.b.State.ReceivedAnchors = append(f.b.State.ReceivedAnchors, &chain.ReceivedAnchor{Partition: protocol.Directory, Body: body})
}

func count(outcome string) float64 {
	return testutil.ToFloat64(mExecStagedProofs.WithLabelValues(outcome))
}

func TestAnchorStaging_ProofWaitsForItsAnchorThenProvesItsRange(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")
	staged0 := count("staged")
	validated0 := count("validated")

	require.NoError(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, 7), f.siblingsOf(0, 2)))
	require.Equal(t, staged0+1, count("staged"))
	require.False(t, f.isProven(f.src[1]), "not proven until the anchor executes")
	blocks := f.b.staging.ProofBlocks(source)
	require.Equal(t, []uint64{7}, blocks)

	f.anchorExecutes(t, 7, f.terminal)
	require.NoError(t, f.b.validateStagedProofs(nil))
	require.Equal(t, validated0+1, count("validated"))
	for _, h := range f.src {
		require.True(t, f.isProven(h), "proven once the anchor validated the proof")
	}
	blocks = f.b.staging.ProofBlocks(source)
	require.Empty(t, blocks, "nothing left waiting")
	proofs := f.b.staging.Proofs(source, 7)
	require.Empty(t, proofs)
}

func TestAnchorStaging_AnchorAlreadyExecutedDecidesAtIntake(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")
	f.anchorExecutes(t, 9, f.terminal)
	validated0 := count("validated")

	require.NoError(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, 9), f.siblingsOf(0, 2)))
	require.Equal(t, validated0+1, count("validated"))
	require.True(t, f.isProven(f.src[0]))
	blocks := f.b.staging.ProofBlocks(source)
	require.Empty(t, blocks, "nothing was staged")
}

func TestAnchorStaging_DisprovedProofIsDiscardedAndCounted(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")
	disproved0 := count("disproved")

	// Waiting on anchor 8, whose root turns out to be something else.
	require.NoError(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, 8), f.siblingsOf(0, 2)))
	other := make([]byte, 32)
	other[0] = 0xEE
	f.anchorExecutes(t, 8, other)
	require.NoError(t, f.b.validateStagedProofs(nil))
	require.Equal(t, disproved0+1, count("disproved"))
	require.False(t, f.isProven(f.src[0]), "a disproved proof proves nothing")
	blocks := f.b.staging.ProofBlocks(source)
	require.Empty(t, blocks)

	// A late proof for an anchor that already executed with a different root
	// is disproved at intake — "never", not "not yet".
	require.NoError(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, 8), f.siblingsOf(0, 2)))
	require.Equal(t, disproved0+2, count("disproved"))
}

func TestAnchorStaging_InvalidListIsRefusedNotStaged(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")
	invalid0 := count("invalid")
	proof := f.proofFor(t, 0, 2, 7)
	proof.ReceiptList.Elements[1] = make([]byte, 32) // corrupt
	require.Error(t, f.b.intakeProof(source, proof, f.siblingsOf(0, 2)))
	require.Equal(t, invalid0+1, count("invalid"))
	blocks := f.b.staging.ProofBlocks(source)
	require.Empty(t, blocks)
}

var _ = merkle.GetReceiptList

// A proof lifted from another source's package cannot be staged under this
// source: it covers none of the messages from this source it travels with.
func TestAnchorStaging_ProofMustCoverAMessageFromItsSource(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")
	unbound0 := count("unbound")
	foreign := [][]byte{make([]byte, 32)}
	require.Error(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, 7), foreign))
	require.Equal(t, unbound0+1, count("unbound"))
	blocks := f.b.staging.ProofBlocks(source)
	require.Empty(t, blocks)
}

// Anchor staging is bounded: a proof claiming an anchor further ahead than the
// horizon is refused, and a source whose waiting proofs already cost more than
// the budget has its next proof dropped. The budget is in bytes, so the bound
// binds on what a source actually costs rather than on how far behind this
// node has fallen (#4282).
func TestAnchorStaging_IsBounded(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")
	refused0, dropped0 := count("refused"), count("dropped")
	require.Error(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, maxAnchorAhead+5), f.siblingsOf(0, 2)))
	require.Equal(t, refused0+1, count("refused"))

	restore := execute.MaxStagedProofBytes
	defer func() { execute.MaxStagedProofBytes = restore }()
	execute.MaxStagedProofBytes = 1 // any staged proof exceeds it

	require.NoError(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, 1), f.siblingsOf(0, 2)),
		"the first proof is staged: the budget is measured before it, not after")
	require.NoError(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, 2), f.siblingsOf(0, 2)))
	require.Equal(t, dropped0+1, count("dropped"))
	require.Equal(t, []uint64{1}, f.b.staging.ProofBlocks(source), "the proof over budget is not staged")
}

// A destination that has fallen behind holds proofs for many Directory blocks
// at once. That is a backlog, not a flood: every one of them is bound to
// entries this node has already accepted and is holding. Dropping them leaves
// every entry they prove waiting for its stream to stop and the proof to be
// fetched back from the source (#4282, #4439), so the budget must not bind on
// an honest backlog.
func TestAnchorStaging_AnHonestBacklogIsNotRefused(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")
	refused0, dropped0 := count("refused"), count("dropped")

	for b := uint64(1); b <= stagedProofBlockBacklog; b++ {
		err := f.b.intakeProof(source, f.proofFor(t, 0, 2, b), f.siblingsOf(0, 2))
		require.NoError(t, err, "proof for Directory block %d refused", b)
	}
	require.Equal(t, refused0, count("refused"),
		"a proof bound to entries this node kept must not be refused")
	require.Equal(t, dropped0, count("dropped"),
		"a proof bound to entries this node kept must not be dropped")
	require.Len(t, f.b.staging.ProofBlocks(source), int(stagedProofBlockBacklog))
}

// stagedProofBlockBacklog is how far behind this test drives the destination:
// comfortably past the old 256-block cap, and well inside maxAnchorAhead.
const stagedProofBlockBacklog = 1024

// When the byte budget does bind, the proof is dropped and nothing else
// happens: the budget is this node's memory, and a restarted node's differs
// from its peers', so it must not decide anything the block records (#4439).
// The intake is not a refusal -- the envelope's entries are not refused with
// the proof -- and the source is not marked. The entries are held as any
// others are; the test that they are held, counted, fetched and executed is
// TestADroppedProofIsFetchedAndTheEntryExecutes (test/e2e).
func TestAnchorStaging_BudgetDropsOnlyTheProof(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")

	restore := execute.MaxStagedProofBytes
	defer func() { execute.MaxStagedProofBytes = restore }()
	execute.MaxStagedProofBytes = 1

	require.NoError(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, 1), f.siblingsOf(0, 2)))
	before := f.b.staging.StagedProofBytes(source)
	require.NoError(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, 2), f.siblingsOf(0, 2)),
		"a proof over budget is dropped, not refused")
	require.Equal(t, before, f.b.staging.StagedProofBytes(source), "the dropped proof costs nothing")
}
