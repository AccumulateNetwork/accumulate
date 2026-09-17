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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Proving hashes changes no hashed state and writes nothing: the proven set
// is staging, and staging is memory (executor spec, "Sync").
func TestProvenSet_IsNotPartOfTheAccountHash(t *testing.T) {
	f := newStagingFixture(t, 3)
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = f.x.Describe.Synthetic()
	require.NoError(t, f.batch.Account(ledger.Url).Main().Put(ledger))
	require.NoError(t, f.batch.UpdateBPT())
	before, err := f.batch.GetBptRootHash()
	require.NoError(t, err)
	f.prove(t, 0, 2)
	require.NoError(t, f.batch.UpdateBPT())
	after, err := f.batch.GetBptRootHash()
	require.NoError(t, err)
	require.Equal(t, before, after, "proving hashes changes no hashed state")
	require.True(t, f.isProven(f.src[1]))
}

// Two proofs claiming the same indexes with different hashes are an attack
// on the stream; the first stands, the second proves nothing and is counted.
func TestProvenSet_ConflictingProofIsRefusedAndCounted(t *testing.T) {
	f := newStagingFixture(t, 3)
	f.prove(t, 0, 2)
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
	err = f.b.staging.Prove(f.stream(), forged)
	require.ErrorIs(t, err, errors.Conflict)
	require.True(t, f.isProven(f.src[1]), "the first proof stands")
	h := sha256.Sum256([]byte("forged 1"))
	require.False(t, f.isProven(h[:]), "the second proves nothing")
	require.NoError(t, f.b.proofValidated(f.source, &protocol.AnnotatedReceipt{ReceiptList: forged, Anchor: directoryAnchorMetadata(3)}))
	require.Equal(t, conflict0+1, count("conflict"))
}

// A collected entry run again before its proof arrives stays collected, and
// nothing is recorded for it anywhere.
func TestCollectedEntry_RerunBeforeProven_StaysCollected(t *testing.T) {
	f := newStagingFixture(t, 3)
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
	d := &bundle{Block: f.b, batch: f.batch, messages: []messaging.Message{member}}
	status, err := SyntheticMessage{}.Process(f.batch, &MessageContext{bundle: d, message: member})
	require.NoError(t, err)
	require.Equal(t, errors.Pending, status.Code, "still collected")
	h := member.Hash()
	st, err := f.batch.Transaction(h[:]).Status().Get()
	require.NoError(t, err)
	require.Zero(t, st.Code, "no status recorded at all")
	_, err = f.batch.Message(h).Main().Get()
	require.ErrorIs(t, err, errors.NotFound, "nothing written: a proof-less unproven entry waits, unrecorded")
}

// A proof below what is proven extends the set backwards; one straddling the
// origin compares only what is held.
func TestProvenSet_ExtendsBackwardsBelowTheOrigin(t *testing.T) {
	f := newStagingFixture(t, 300)
	f.prove(t, 290, 299)
	require.False(t, f.isProven(f.src[15]))
	f.prove(t, 10, 20)
	for i := 10; i <= 20; i++ {
		require.True(t, f.isProven(f.src[i]), "proven by the earlier proof: %d", i)
	}
	require.False(t, f.isProven(f.src[25]), "not covered by any proof")
	f.prove(t, 280, 295)
	require.True(t, f.isProven(f.src[285]))
	other := f.batch.Account(protocol.PartitionUrl("BVN2").JoinPath(protocol.Synthetic)).MainChain()
	c, err := other.Get()
	require.NoError(t, err)
	for i := 0; i < 21; i++ {
		h := sha256.Sum256([]byte(fmt.Sprintf("other %d", i)))
		require.NoError(t, c.AddEntry(h[:], false))
	}
	forged, err := merkle.GetReceiptList(other.Inner(), 10, 20)
	require.NoError(t, err)
	require.ErrorIs(t, f.b.staging.Prove(f.stream(), forged), errors.Conflict)
}

// A run never takes a collected entry until the proven set covers its hash.
func TestRun_DoesNotTakeACollectedEntryUntilProven(t *testing.T) {
	f := newStagingFixture(t, 3)
	ledgerUrl := f.x.Describe.Synthetic()
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = ledgerUrl
	require.NoError(t, f.batch.Account(ledgerUrl).Main().Put(ledger))
	str := stream{kind: streamSynthetic, ledger: ledgerUrl, source: f.source}
	var h1 [32]byte
	copy(h1[:], f.src[0])
	f.b.staging.Hold(f.stream(), 1, &execute.Held{ID: f.source.WithTxID([32]byte{1}), Collected: true, Hash: h1})
	pos, err := f.b.positionOf(str)
	require.NoError(t, err)
	run, _ := buildRun(pos, nil, 10)
	require.Empty(t, run, "collected and unproven: not runnable")
	f.prove(t, 0, 2)
	run, _ = buildRun(pos, nil, 10)
	require.Len(t, run, 1, "proven: runnable")
	require.Equal(t, uint64(1), run[0].number)
	f.b.staging.Hold(f.stream(), 2, &execute.Held{ID: f.source.WithTxID([32]byte{2})})
	run, _ = buildRun(pos, nil, 10)
	require.Len(t, run, 2)
}
