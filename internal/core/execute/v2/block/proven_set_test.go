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

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The proven set (executor spec, "Proof"): what validated proofs have proven,
// by index. It is staging, not state — unhashed — and two proofs that claim
// the same indexes with different hashes are an attack, counted and refused.

func TestProvenSet_IsNotPartOfTheAccountHash(t *testing.T) {
	x := new(Executor)
	x.Describe = execute.DescribeShim{NetworkType: protocol.PartitionTypeBlockValidator, PartitionId: "BVN0"}
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	batch := db.Begin(true)
	defer batch.Discard()

	// Give the synthetic account a main state so it has a hash at all.
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = x.Describe.Synthetic()
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))

	// The source chain is another account's state; build it before measuring.
	src := batch.Account(protocol.PartitionUrl("BVN1").JoinPath(protocol.Synthetic)).MainChain()
	chain, err := src.Get()
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("entry %d", i)))
		require.NoError(t, chain.AddEntry(h[:], false))
	}
	list, err := merkle.GetReceiptList(src.Inner(), 0, 2)
	require.NoError(t, err)
	require.NoError(t, batch.UpdateBPT())
	before, err := batch.GetBptRootHash()
	require.NoError(t, err)

	require.NoError(t, x.seedSyntheticReplica(batch, protocol.PartitionUrl("BVN1"), list))
	require.NoError(t, batch.UpdateBPT())
	after, err := batch.GetBptRootHash()
	require.NoError(t, err)
	require.Equal(t, before, after, "proving hashes changes no hashed state")
}

func TestProvenSet_ConflictingProofIsRefusedAndCounted(t *testing.T) {
	f := newReplicaFixture(t, 3)
	source := protocol.PartitionUrl("BVN1")
	f.seed(t, 0, 2)

	// A second, equally well-formed chain claiming the same indexes.
	other := f.batch.Account(protocol.PartitionUrl("BVN2").JoinPath(protocol.Synthetic)).MainChain()
	chain, err := other.Get()
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("forged %d", i)))
		require.NoError(t, chain.AddEntry(h[:], false))
	}
	forged, err := merkle.GetReceiptList(other.Inner(), 0, 2)
	require.NoError(t, err)
	require.True(t, forged.Validate(nil))

	conflict0 := count("conflict")
	err = f.x.seedSyntheticReplica(f.batch, source, forged)
	require.Error(t, err)
	require.ErrorContains(t, err, "conflict")
	require.True(t, f.x.replicaIncludes(f.batch, source, f.src[1]), "the first proof stands")
	h := sha256.Sum256([]byte("forged 1"))
	require.False(t, f.x.replicaIncludes(f.batch, source, h[:]), "the second proves nothing")

	// Through anchor staging the same proof is discarded and counted, never
	// an error the block sees.
	b := &Block{positions: new(positionCache), Executor: f.x, Batch: f.batch}
	require.NoError(t, b.proofValidated(source, &protocol.AnnotatedReceipt{ReceiptList: forged, Anchor: directoryAnchorMetadata(3)}))
	require.Equal(t, conflict0+1, count("conflict"))
}

// A held entry may be put in a run before its proof has been validated — runs
// take every held number. Re-running it then must leave it collected, never
// record a terminal status: a failed status is "delivered" to the stream and
// wedges it on a message nothing will ever retry.
func TestCollectedEntry_RerunBeforeProven_StaysCollected(t *testing.T) {
	f := newReplicaFixture(t, 3)
	f.x.globalsPtr.Store(&Globals{Active: core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionLatest}})
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = f.x.Describe.Synthetic()
	require.NoError(t, f.batch.Account(ledger.Url).Main().Put(ledger))

	txn := new(protocol.Transaction)
	txn.Header.Principal = protocol.AccountUrl("alice", "tokens")
	txn.Body = &protocol.SyntheticDepositCredits{Amount: 1}
	seq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: txn},
		Source:      protocol.PartitionUrl("BVN1"),
		Destination: protocol.PartitionUrl("BVN0"),
		Number:      1,
	}
	member := &messaging.SyntheticMessage{Message: seq,
		Signature: &protocol.ED25519Signature{PublicKey: make([]byte, 32), Signer: protocol.DnUrl().JoinPath(protocol.Network)}}

	// Not proven, no proof in hand: exactly what MessageIsReady presents.
	d := &bundle{Block: &Block{positions: new(positionCache), Executor: f.x, Batch: f.batch}, batch: f.batch, messages: []messaging.Message{member}}
	status, err := SyntheticMessage{}.Process(f.batch, &MessageContext{bundle: d, message: member})
	require.NoError(t, err)
	require.Equal(t, errors.Pending, status.Code, "still collected")
	h := member.Hash()
	st, err := f.batch.Transaction(h[:]).Status().Get()
	require.NoError(t, err)
	require.False(t, st.Delivered(), "nothing terminal recorded outside staging: code %v", st.Code)
	require.Zero(t, st.Code, "no status recorded at all")
}

// A proof that overlaps indexes below the replica's seed origin cannot be
// compared there — the replica never stored those entries — and that is not
// an error and not a conflict: the range is simply already proven or not ours
// to judge.
func TestProvenSet_ProofBelowTheSeedOriginIsNotAnError(t *testing.T) {
	f := newReplicaFixture(t, 300)
	source := protocol.PartitionUrl("BVN1")
	f.seed(t, 290, 299) // the replica begins at 290
	require.NoError(t, f.x.seedSyntheticReplica(f.batch, source, f.proof(t, 10, 20)),
		"a proof entirely below the origin is a no-op, not an error")
	require.NoError(t, f.x.seedSyntheticReplica(f.batch, source, f.proof(t, 280, 295)),
		"a proof straddling the origin compares only what is held")
	require.True(t, f.x.replicaIncludes(f.batch, source, f.src[295]))
}

// The run builder never takes a collected entry until the proven set covers
// its hash: the synth stage runs only validated entries (executor spec,
// "Collection").
func TestRun_DoesNotTakeACollectedEntryUntilProven(t *testing.T) {
	f := newReplicaFixture(t, 3)
	source := protocol.PartitionUrl("BVN1")
	ledgerUrl := f.x.Describe.Synthetic()
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = ledgerUrl
	require.NoError(t, f.batch.Account(ledgerUrl).Main().Put(ledger))
	str := stream{kind: streamSynthetic, ledger: ledgerUrl, source: source}
	id := execute.StreamID{Ledger: ledgerUrl, Source: source}

	// Number 1 is held and collected: its hash is the source chain's entry 0,
	// which nothing has proven yet.
	require.NoError(t, execute.Hold(f.batch, id, 1, source.WithTxID([32]byte{1})))
	var h [32]byte
	copy(h[:], f.src[0])
	require.NoError(t, f.batch.Account(ledgerUrl).Collected(source, 1).Put(h))

	b := &Block{positions: new(positionCache), Executor: f.x, Batch: f.batch}
	pos, err := b.positionOf(str)
	require.NoError(t, err)
	run, _ := buildRun(pos, nil, 10)
	require.Empty(t, run, "collected and unproven: not runnable")

	f.seed(t, 0, 2)
	run, _ = buildRun(pos, nil, 10)
	require.Len(t, run, 1, "proven: runnable")
	require.Equal(t, uint64(1), run[0].number)

	// An entry held by the sequenced layer (no Collected mark) was proven
	// when it was held and is always runnable.
	require.NoError(t, execute.Hold(f.batch, id, 2, source.WithTxID([32]byte{2})))
	run, _ = buildRun(pos, nil, 10)
	require.Len(t, run, 2)
}

// A proof for an earlier span than the proven set's origin proves what it
// covers: its elements are recorded below the origin so the entries it names
// are proven wherever the proof lands, and a later contradicting proof for the
// same indexes is still a conflict.
func TestProvenSet_ExtendsBackwardsBelowTheOrigin(t *testing.T) {
	f := newReplicaFixture(t, 300)
	source := protocol.PartitionUrl("BVN1")
	f.seed(t, 290, 299) // the proven set begins at 290
	require.False(t, f.x.replicaIncludes(f.batch, source, f.src[15]))

	require.NoError(t, f.x.seedSyntheticReplica(f.batch, source, f.proof(t, 10, 20)))
	for i := 10; i <= 20; i++ {
		require.True(t, f.x.replicaIncludes(f.batch, source, f.src[i]), "proven by the earlier proof: %d", i)
	}
	require.False(t, f.x.replicaIncludes(f.batch, source, f.src[25]), "not covered by any proof")

	// A contradicting proof for those indexes is a conflict, as above the origin.
	other := f.batch.Account(protocol.PartitionUrl("BVN2").JoinPath(protocol.Synthetic)).MainChain()
	c, err := other.Get()
	require.NoError(t, err)
	for i := 0; i < 21; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("other %d", i)))
		require.NoError(t, c.AddEntry(h[:], false))
	}
	forged, err := merkle.GetReceiptList(other.Inner(), 10, 20)
	require.NoError(t, err)
	err = f.x.seedSyntheticReplica(f.batch, source, forged)
	require.ErrorContains(t, err, "conflict")
}
