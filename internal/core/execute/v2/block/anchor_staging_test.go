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
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
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
	*replicaFixture
	b        *Block
	terminal []byte
}

func newAnchorStagingFixture(t *testing.T) *anchorStagingFixture {
	t.Helper()
	f := newReplicaFixture(t, 3)
	f.x.globalsPtr.Store(&Globals{Active: core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionLatest}})
	rootChain, err := f.batch.Account(protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)).RootChain().Get()
	require.NoError(t, err)
	require.NoError(t, rootChain.AddEntry(f.chain.Anchor(), false))
	b := &Block{positions: new(positionCache), Executor: f.x, Batch: f.batch}
	return &anchorStagingFixture{replicaFixture: f, b: b, terminal: rootChain.Anchor()}
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

	require.NoError(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, 7)))
	require.Equal(t, staged0+1, count("staged"))
	require.False(t, f.x.replicaIncludes(f.batch, source, f.src[1]), "not proven until the anchor executes")
	blocks, err := f.batch.Account(f.x.Describe.Synthetic()).StagedProofBlocks(source).Get()
	require.NoError(t, err)
	require.Equal(t, []uint64{7}, blocks)

	f.anchorExecutes(t, 7, f.terminal)
	require.NoError(t, f.b.validateStagedProofs())
	require.Equal(t, validated0+1, count("validated"))
	for _, h := range f.src {
		require.True(t, f.x.replicaIncludes(f.batch, source, h), "proven once the anchor validated the proof")
	}
	blocks, err = f.batch.Account(f.x.Describe.Synthetic()).StagedProofBlocks(source).Get()
	require.NoError(t, err)
	require.Empty(t, blocks, "nothing left waiting")
	proofs, err := f.batch.Account(f.x.Describe.Synthetic()).StagedProofs(source, 7).Get()
	require.NoError(t, err)
	require.Empty(t, proofs)
}

func TestAnchorStaging_AnchorAlreadyExecutedDecidesAtIntake(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")
	f.anchorExecutes(t, 9, f.terminal)
	validated0 := count("validated")

	require.NoError(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, 9)))
	require.Equal(t, validated0+1, count("validated"))
	require.True(t, f.x.replicaIncludes(f.batch, source, f.src[0]))
	blocks, err := f.batch.Account(f.x.Describe.Synthetic()).StagedProofBlocks(source).Get()
	require.NoError(t, err)
	require.Empty(t, blocks, "nothing was staged")
}

func TestAnchorStaging_DisprovedProofIsDiscardedAndCounted(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")
	disproved0 := count("disproved")

	// Waiting on anchor 8, whose root turns out to be something else.
	require.NoError(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, 8)))
	other := make([]byte, 32)
	other[0] = 0xEE
	f.anchorExecutes(t, 8, other)
	require.NoError(t, f.b.validateStagedProofs())
	require.Equal(t, disproved0+1, count("disproved"))
	require.False(t, f.x.replicaIncludes(f.batch, source, f.src[0]), "a disproved proof proves nothing")
	blocks, err := f.batch.Account(f.x.Describe.Synthetic()).StagedProofBlocks(source).Get()
	require.NoError(t, err)
	require.Empty(t, blocks)

	// A late proof for an anchor that already executed with a different root
	// is disproved at intake — "never", not "not yet".
	require.NoError(t, f.b.intakeProof(source, f.proofFor(t, 0, 2, 8)))
	require.Equal(t, disproved0+2, count("disproved"))
}

func TestAnchorStaging_InvalidListIsRefusedNotStaged(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")
	invalid0 := count("invalid")
	proof := f.proofFor(t, 0, 2, 7)
	proof.ReceiptList.Elements[1] = make([]byte, 32) // corrupt
	require.Error(t, f.b.intakeProof(source, proof))
	require.Equal(t, invalid0+1, count("invalid"))
	blocks, err := f.batch.Account(f.x.Describe.Synthetic()).StagedProofBlocks(source).Get()
	require.NoError(t, err)
	require.Empty(t, blocks)
}

var _ = merkle.GetReceiptList
