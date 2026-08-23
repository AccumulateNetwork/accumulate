// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package block

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4140: the destination's replica of a source's synthetic main chain. A
// replica is a chain plus a merkle state, both constructible without a
// network, so nearly everything here is pure. The end-to-end acceptance path
// (a proof-less, signature-less message accepted because the replica
// includes it) is covered through SyntheticMessage.check.

// replicaFixture builds an executor, a batch, a fake source chain with n
// entries, and returns a proof builder over it.
type replicaFixture struct {
	x      *Executor
	batch  *database.Batch
	src    [][]byte // entry hashes of the fake source chain
	chain  *database.Chain
	chain2 *database.Chain2
}

func newReplicaFixture(t *testing.T, entries int) *replicaFixture {
	t.Helper()
	x := new(Executor)
	x.Describe = execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	// A stand-in for the SOURCE partition's synthetic main chain.
	chain2 := batch.Account(protocol.PartitionUrl("BVN1").JoinPath(protocol.Synthetic)).MainChain()
	chain, err := chain2.Get()
	require.NoError(t, err)

	f := &replicaFixture{x: x, batch: batch, chain: chain, chain2: chain2}
	for i := 0; i < entries; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("synthetic message %d", i)))
		f.src = append(f.src, h[:])
		require.NoError(t, chain.AddEntry(h[:], false))
	}
	return f
}

// proof builds a ReceiptList over source entries [start, end] — the same
// call the sequencer service uses.
func (f *replicaFixture) proof(t *testing.T, start, end int64) *merkle.ReceiptList {
	t.Helper()
	list, err := merkle.GetReceiptList(f.chain2.Inner(), start, end)
	require.NoError(t, err)
	require.True(t, list.Validate(nil), "the fixture's proof must be valid")
	return list
}

func (f *replicaFixture) replica() *database.Chain2 {
	return f.batch.Account(f.x.Describe.Synthetic()).SyntheticReplica("bvn1")
}

func (f *replicaFixture) seed(t *testing.T, start, end int64) {
	t.Helper()
	require.NoError(t, f.x.seedSyntheticReplica(f.batch, protocol.PartitionUrl("BVN1"), f.proof(t, start, end)))
}

func replicaHeight(t *testing.T, f *replicaFixture) int64 {
	t.Helper()
	c, err := f.replica().Get()
	require.NoError(t, err)
	return c.Height()
}

// A replica can begin from any accepted proof — the ReceiptList carries the
// state at start-1, so no prior entries are required.
func TestReplica_SeedsFromProofMerkleStateAtStartMinusOne(t *testing.T) {
	f := newReplicaFixture(t, 20)
	f.seed(t, 10, 15)

	assert.EqualValues(t, 16, replicaHeight(t, f), "height is proven-through")
	assert.True(t, f.x.replicaIncludes(f.batch, protocol.PartitionUrl("BVN1"), f.src[12]),
		"an element the proof covered matches")
	assert.False(t, f.x.replicaIncludes(f.batch, protocol.PartitionUrl("BVN1"), f.src[5]),
		"elements behind the seeding state were never absorbed — the state stands in for them")
}

// Replaying the proof's elements onto its state reproduces the committed
// anchor — the replica's head anchor equals the source chain's at the same
// height.
func TestReplica_ReplayingElementsReproducesCommittedAnchor(t *testing.T) {
	f := newReplicaFixture(t, 10)
	f.seed(t, 0, 9)

	rc, err := f.replica().Get()
	require.NoError(t, err)
	require.EqualValues(t, 10, rc.Height())
	assert.Equal(t, f.chain.Anchor(), rc.Anchor(),
		"the replica IS the source chain over the proven span")
}

func TestReplica_SeedingTwiceFromOverlappingProofsIsIdempotent(t *testing.T) {
	f := newReplicaFixture(t, 20)
	f.seed(t, 0, 9)
	f.seed(t, 5, 14) // overlaps 5-9, extends 10-14
	f.seed(t, 0, 9)  // entirely behind — no-op

	assert.EqualValues(t, 15, replicaHeight(t, f))
	rc, err := f.replica().Get()
	require.NoError(t, err)
	sub, err := f.chain.Entries(0, 15)
	require.NoError(t, err)
	got, err := rc.Entries(0, 15)
	require.NoError(t, err)
	assert.Equal(t, sub, got, "overlap extends only the missing tail — entry for entry")
}

// The state's count must agree with its structure (#4106) — a lying count is
// rejected rather than seeded.
func TestReplica_IndexIsBoundByStructure(t *testing.T) {
	f := newReplicaFixture(t, 10)
	list := f.proof(t, 5, 9)
	list.MerkleState.Count += 7 // lie

	err := f.x.seedSyntheticReplica(f.batch, protocol.PartitionUrl("BVN1"), list)
	require.Error(t, err)
	assert.ErrorContains(t, err, "inconsistent")
}

func TestReplica_EntryAboveHeightIsRefused(t *testing.T) {
	f := newReplicaFixture(t, 20)
	f.seed(t, 0, 9)

	assert.False(t, f.x.replicaIncludes(f.batch, protocol.PartitionUrl("BVN1"), f.src[15]),
		"never accept beyond what is proven")
}

func TestReplica_HashMismatchIsRejected(t *testing.T) {
	f := newReplicaFixture(t, 10)
	f.seed(t, 0, 9)

	fake := sha256.Sum256([]byte("substituted entry"))
	assert.False(t, f.x.replicaIncludes(f.batch, protocol.PartitionUrl("BVN1"), fake[:]),
		"a substituted entry does not match")
}

// Discard entries behind a retained state, keep verifying forward: a later
// proof that begins past the replica's height re-seeds from ITS state, and
// the replica keeps answering for the new window.
func TestReplica_PruneBehindRetainedStateStillVerifiesForward(t *testing.T) {
	f := newReplicaFixture(t, 40)
	f.seed(t, 0, 9)
	f.seed(t, 30, 39) // gap: re-seed from the newer proof's state

	assert.EqualValues(t, 40, replicaHeight(t, f))
	assert.True(t, f.x.replicaIncludes(f.batch, protocol.PartitionUrl("BVN1"), f.src[35]))

	// Entries absorbed before the re-seed still answer truthfully — the
	// element index survives — while entries never absorbed do not.
	assert.True(t, f.x.replicaIncludes(f.batch, protocol.PartitionUrl("BVN1"), f.src[5]),
		"a previously proven entry remains a true answer")
	assert.False(t, f.x.replicaIncludes(f.batch, protocol.PartitionUrl("BVN1"), f.src[20]),
		"the gap the re-seed skipped was never proven here and must not match")
}

// Two streams from one partition's ledger do not collide, and the
// destination participates through the account the replica lives on.
func TestReplica_KeyedOnStreamIdentityNotPartitionUrl(t *testing.T) {
	f := newReplicaFixture(t, 10)
	f.seed(t, 0, 4)

	other := f.batch.Account(f.x.Describe.Synthetic()).SyntheticReplica("bvn1-w1")
	c, err := other.Get()
	require.NoError(t, err)
	assert.Zero(t, c.Height(), "a different stream identity is a different replica")
}

// Seeding the replica changes the account's BPT entry: replica state is
// consensus state. (This is why, on a gated line, this change would need a
// version gate — this line ships ungated on fresh installs by decision.)
func TestReplica_AddingTheChainChangesTheAccountHash(t *testing.T) {
	f := newReplicaFixture(t, 10)

	require.NoError(t, f.batch.UpdateBPT())
	before, err := f.batch.GetBptRootHash()
	require.NoError(t, err)

	f.seed(t, 0, 4)
	require.NoError(t, f.batch.UpdateBPT())
	after, err := f.batch.GetBptRootHash()
	require.NoError(t, err)

	assert.NotEqual(t, before, after, "the replica participates in the state root")
}

// The payoff: a synthetic message carrying NO proof and NO signature is
// accepted when the destination's replica already contains its hash — the
// proof that covers it was checked, anchored, and absorbed when it first
// arrived. A message the replica does not contain is still refused.
func TestReplica_MessageUnderTheReplicaNeedsNoProofAndNoSignature(t *testing.T) {
	f := newReplicaFixture(t, 0)
	f.x.globals = &Globals{Active: core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionLatest}}

	mkSeq := func(n byte) *messaging.SequencedMessage {
		txn := new(protocol.Transaction)
		txn.Header.Principal = protocol.AccountUrl("alice", "tokens")
		txn.Body = &protocol.SyntheticDepositCredits{Amount: uint64(n)}
		return &messaging.SequencedMessage{
			Message:     &messaging.TransactionMessage{Transaction: txn},
			Source:      protocol.PartitionUrl("BVN1"),
			Destination: protocol.PartitionUrl("BVN0"),
			Number:      uint64(n),
		}
	}

	// The source chain holds the sequenced-message hashes, exactly as
	// buildSynthTxn records them; prove and absorb the first two.
	proven, unproven := mkSeq(1), mkSeq(2)
	h1, h2 := proven.Hash(), unproven.Hash()
	require.NoError(t, f.chain.AddEntry(h1[:], false))
	require.NoError(t, f.chain.AddEntry(h2[:], false))
	f.seed(t, 0, 0) // absorb only the first

	ctx := &MessageContext{
		bundle:  &bundle{Block: &Block{Executor: f.x, Batch: f.batch}, batch: f.batch},
		message: &messaging.SyntheticMessage{Message: proven},
	}
	syn, err := SyntheticMessage{}.check(f.batch, ctx)
	require.NoError(t, err, "a message the replica contains needs no proof and no signature")
	require.NotNil(t, syn)

	ctx = &MessageContext{
		bundle:  &bundle{Block: &Block{Executor: f.x, Batch: f.batch}, batch: f.batch},
		message: &messaging.SyntheticMessage{Message: unproven},
	}
	_, err = SyntheticMessage{}.check(f.batch, ctx)
	require.Error(t, err, "a message the replica does not contain is refused")
	require.ErrorContains(t, err, "missing proof")
}
