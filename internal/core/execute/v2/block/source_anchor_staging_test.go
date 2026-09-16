// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Dispatch at begin(N+1) sends a proof that stops at the SOURCE's own root,
// because the Directory has not receipted the block yet (#4253). The
// destination holds it and completes it from the Directory anchor it executes
// anyway, which carries a receipt from that source block's root to the
// Directory root. Nothing waits on the Directory round trip.

// sourceProofFor builds the begin(N+1) form: a valid package proof over
// [start, end] continued to the SOURCE's root, naming the source and the
// source block rather than a Directory block.
func (f *anchorStagingFixture) sourceProofFor(t *testing.T, start, end int64, srcBlock uint64) *protocol.AnnotatedReceipt {
	t.Helper()
	list := f.proof(t, start, end)
	rootChain, err := f.batch.Account(protocol.PartitionUrl("BVN1").JoinPath(protocol.Ledger)).RootChain().Get()
	require.NoError(t, err)
	cont, err := rootChain.Receipt(0, 0)
	require.NoError(t, err)
	list.ContinuedReceipt = cont
	require.True(t, list.Validate(nil))
	require.Equal(t, f.terminal, list.ContinuedReceipt.Anchor, "the proof must end in the source's root")
	return &protocol.AnnotatedReceipt{
		ReceiptList: list,
		Anchor:      sourceAnchorMetadata(protocol.PartitionUrl("BVN1"), srcBlock),
	}
}

// anchorExecutesWithReceipt is a Directory anchor that receipts one source
// block: its root chain carries the source's root, so the receipt runs from
// that root to the Directory root, and the Directory root lands on this
// partition's Directory anchor chain.
func (f *anchorStagingFixture) anchorExecutesWithReceipt(t *testing.T, dnBlock uint64, src *protocol.PartitionAnchor, srcRoot []byte) {
	t.Helper()
	dnRootChain, err := f.batch.Account(protocol.DnUrl().JoinPath(protocol.Ledger)).RootChain().Get()
	require.NoError(t, err)
	require.NoError(t, dnRootChain.AddEntry(srcRoot, false))
	rc, err := dnRootChain.Receipt(0, 0)
	require.NoError(t, err)
	dnRoot := dnRootChain.Anchor()

	c, err := f.batch.Account(f.x.Describe.AnchorPool()).AnchorChain(protocol.Directory).Root().Get()
	require.NoError(t, err)
	require.NoError(t, c.AddEntry(dnRoot, false))
	require.NoError(t, f.batch.Account(f.x.Describe.AnchorPool()).DirectoryAnchorBlock().Put(dnBlock))

	body := &protocol.DirectoryAnchor{}
	body.MinorBlockIndex = dnBlock
	copy(body.RootChainAnchor[:], dnRoot)
	body.Receipts = []*protocol.PartitionAnchorReceipt{{Anchor: src, RootChainReceipt: rc}}
	f.b.State.ReceivedAnchors = append(f.b.State.ReceivedAnchors, &chain.ReceivedAnchor{Partition: protocol.Directory, Body: body})
}

func TestSourceAnchorStaging_TheDirectoryReceiptCompletesTheProof(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")
	staged0, extended0, validated0 := count("staged-source"), count("extended"), count("validated")

	// Arrives before any Directory anchor: held, and proves nothing yet.
	require.NoError(t, f.b.intakeProof(source, f.sourceProofFor(t, 0, 2, 5), f.siblingsOf(0, 2)))
	require.Equal(t, staged0+1, count("staged-source"))
	require.False(t, f.isProven(f.src[1]), "a source-root proof proves nothing on its own")
	require.Equal(t, []uint64{5}, f.b.staging.SourceProofBlocks(source), "held under the SOURCE block")
	require.Empty(t, f.b.staging.ProofBlocks(source), "and not in the Directory-anchored space")

	// The Directory anchor receipting source block 5 completes it.
	f.anchorExecutesWithReceipt(t, 11, &protocol.PartitionAnchor{Source: source, MinorBlockIndex: 5}, f.terminal)
	require.NoError(t, f.b.validateStagedProofs(nil))
	require.Equal(t, extended0+1, count("extended"))
	require.Equal(t, validated0+1, count("validated"))
	for _, h := range f.src {
		require.True(t, f.isProven(h), "proven once the Directory receipt extended the proof")
	}
	require.Empty(t, f.b.staging.SourceProofBlocks(source), "nothing left waiting")
}

// A Directory anchor that receipts a DIFFERENT source block leaves the proof
// where it is: the receipt does not continue from the root the proof ends in,
// and a proof must never be completed by an unrelated receipt.
func TestSourceAnchorStaging_AnotherBlocksReceiptDoesNotComplete(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")
	extended0 := count("extended")

	require.NoError(t, f.b.intakeProof(source, f.sourceProofFor(t, 0, 2, 5), f.siblingsOf(0, 2)))
	f.anchorExecutesWithReceipt(t, 11, &protocol.PartitionAnchor{Source: source, MinorBlockIndex: 6}, f.terminal)
	require.NoError(t, f.b.validateStagedProofs(nil))

	require.Equal(t, extended0, count("extended"), "no proof was extended")
	require.False(t, f.isProven(f.src[1]), "still unproven")
	require.Equal(t, []uint64{5}, f.b.staging.SourceProofBlocks(source), "still waiting for its own block's receipt")
}

// The annotation must name the partition the messages came from. A proof
// claiming to be anchored in some other partition's root would be completed
// with that partition's receipt, which proves nothing about this source.
func TestSourceAnchorStaging_ProofMustNameItsOwnSource(t *testing.T) {
	f := newAnchorStagingFixture(t)
	source := protocol.PartitionUrl("BVN1")

	p := f.sourceProofFor(t, 0, 2, 5)
	p.Anchor = sourceAnchorMetadata(protocol.PartitionUrl("BVN9"), 5)
	err := f.b.intakeProof(source, p, f.siblingsOf(0, 2))
	require.Error(t, err, "a proof anchored in another partition is refused")
	require.Empty(t, f.b.staging.SourceProofBlocks(source))
}
