// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// THREAT MODEL DEMONSTRATION -- not to be kept. Shows what Staging.Load takes
// on a peer's word.
func TestThreat_LoadTakesValidatedHashesOnTheirWord(t *testing.T) {
	dest := protocol.PartitionUrl("BVN1")
	src := protocol.PartitionUrl("BVN0")
	ledger := dest.JoinPath(protocol.Synthetic)
	id := StreamID{Ledger: ledger, Source: src}

	// A message no source ever produced: a synthetic deposit invented by the
	// serving peer.
	body := new(protocol.SyntheticDepositTokens)
	body.Token = protocol.AcmeUrl()
	body.Amount.SetUint64(1e12)
	txn := new(protocol.Transaction)
	txn.Header.Principal = url.MustParse("acc://attacker.acme/tokens")
	txn.Body = body
	seq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: txn},
		Source:      src,
		Destination: dest,
		Number:      101,
	}
	h := seq.Hash()

	snap := &private.StagingSnapshot{
		Block: 500,
		Streams: []*private.StagedStream{{
			Ledger:    ledger,
			Source:    src,
			Delivered: 100,
			Sighted:   101,
			// The peer asserts the hash is validated. No proof accompanies it.
			Validated: []*private.StagedHash{{Number: 101, Hash: h}},
			Entries: []*private.StagedEntry{{
				Number:    101,
				Message:   seq,
				Collected: true,
				Hash:      h,
			}},
		}},
	}

	s := NewStaging()
	require.NoError(t, s.Load(snap))

	tx := s.Begin()
	got, ok := tx.Validated(id, 101)
	require.True(t, ok, "the fabricated hash is recorded as validated")
	require.Equal(t, h, got)
	require.True(t, tx.IsValidated(id, 101, h),
		"IsValidated -- what makes a collected entry runnable and what makes "+
			"SyntheticMessage.check accept a message with no proof and no signature")
	held, ok := tx.IDOf(id, 101)
	require.True(t, ok)
	require.True(t, held.Collected)
}

// A peer that overstates Delivered raises the stream's floor permanently:
// nothing at or below it can ever be held again, and Release only moves
// forward, so SettleStaging against the real (lower) pulled Delivered does
// not undo it.
func TestThreat_LoadTakesDeliveredOnTheirWord(t *testing.T) {
	dest := protocol.PartitionUrl("BVN1")
	src := protocol.PartitionUrl("BVN0")
	ledger := dest.JoinPath(protocol.Synthetic)
	id := StreamID{Ledger: ledger, Source: src}

	snap := &private.StagingSnapshot{
		Block: 500,
		Streams: []*private.StagedStream{{
			Ledger:    ledger,
			Source:    src,
			Delivered: 1_000_000, // the truth is 100
		}},
	}
	s := NewStaging()
	require.NoError(t, s.Load(snap))

	// SettleStaging releases through the PULLED ledger's Delivered, which is
	// the truth -- but Release only moves forward.
	tx := s.Begin()
	tx.Release(id, 100)
	tx.Commit()

	tx = s.Begin()
	seq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: new(protocol.Transaction)},
		Source:      src,
		Destination: dest,
		Number:      101,
	}
	tx.Hold(id, 101, &Held{ID: seq.ID(), Message: seq})
	tx.Commit()

	tx = s.Begin()
	_, ok := tx.IDOf(id, 101)
	require.False(t, ok, "number 101 can never be held again: the peer's Delivered is the floor")
}

// A proof that arrives in a snapshot is never validated. Staging.Load stores
// it as given; SettleStaging -> decideProofs -> provingAnchorIndex only reads
// AnnotatedReceipt.TerminalAnchor() (a plain field read) and looks it up in
// the local DN anchor pool; proofValidated then calls Prove, which records the
// list's Elements as validated hashes. Nothing on that path calls
// ReceiptList.Validate -- only intakeProof does, and a snapshot bypasses it.
func TestThreat_SnapshotProofIsNeverValidated(t *testing.T) {
	dest := protocol.PartitionUrl("BVN1")
	src := protocol.PartitionUrl("BVN0")
	ledger := dest.JoinPath(protocol.Synthetic)
	id := StreamID{Ledger: ledger, Source: src}

	// Elements are whatever the attacker wants validated. MerkleState.Count
	// is 1 and Pending has one slot filled, so countFromPending agrees --
	// the only structural check Prove makes.
	evil := [32]byte{0xEE}
	list := &merkle.ReceiptList{
		MerkleState: &merkle.State{Count: 1, Pending: [][]byte{bytes.Repeat([]byte{1}, 32)}},
		Elements:    [][]byte{evil[:]},
		Receipt:     &merkle.Receipt{Start: evil[:], Anchor: bytes.Repeat([]byte{2}, 32)},
		// The only field provingAnchorIndex reads: set it to a DN anchor root
		// the victim has executed, copied from any block.
		ContinuedReceipt: &merkle.Receipt{Anchor: bytes.Repeat([]byte{3}, 32)},
	}
	require.False(t, list.Validate(nil), "this list is nonsense and Validate says so")

	proof := &protocol.AnnotatedReceipt{
		ReceiptList: list,
		Anchor:      &protocol.AnchorMetadata{Account: protocol.DnUrl(), SourceBlock: 900},
	}

	s := NewStaging()
	require.NoError(t, s.Load(&private.StagingSnapshot{
		Block: 500,
		Streams: []*private.StagedStream{{
			Ledger: ledger, Source: src, Delivered: 1,
			Proofs: []*private.StagedProof{{AnchorBlock: 900, Proof: proof}},
		}},
	}))

	tx := s.Begin()
	require.Len(t, tx.Proofs(src, 900), 1, "Load stored an invalid proof without checking it")
	require.Equal(t, bytes.Repeat([]byte{3}, 32), proof.TerminalAnchor(),
		"TerminalAnchor is a field read; provingAnchorIndex looks up exactly this")

	// What decideProofs -> proofValidated does once the named anchor block has
	// executed: no Validate, straight to Prove.
	require.NoError(t, tx.Prove(id, list))
	tx.Commit()

	tx = s.Begin()
	got, ok := tx.Validated(id, 2)
	require.True(t, ok, "an attacker-chosen hash is now validated at number 2")
	require.Equal(t, evil, got)
}

// The cheapest shape of all: Collected=false. streamPosition.runnable
// (stream_position.go:103-107) returns true immediately for an entry that is
// not marked collected -- "an entry held by the sequenced layer passed its
// proof when it was held" -- so no validated hash and no proof are needed.
// The entry then runs through MessageIsReady, which loads h.Message from
// staging and calls the executor on it (msg_is_ready.go:41-57), and
// SequencedMessage.check accepts a non-anchor payload because
// isWithin(MessageIsReady) is true (msg_sequenced.go:81-88).
func TestThreat_LoadAcceptsUncollectedEntries(t *testing.T) {
	dest := protocol.PartitionUrl("BVN1")
	src := protocol.PartitionUrl("BVN0")
	ledger := dest.JoinPath(protocol.Synthetic)
	id := StreamID{Ledger: ledger, Source: src}

	body := new(protocol.SyntheticDepositTokens)
	body.Token = protocol.AcmeUrl()
	body.Amount.SetUint64(1e12)
	txn := new(protocol.Transaction)
	txn.Header.Principal = url.MustParse("acc://attacker.acme/tokens")
	txn.Body = body
	seq := &messaging.SequencedMessage{
		Message:     &messaging.TransactionMessage{Transaction: txn},
		Source:      src,
		Destination: dest,
		Number:      101,
	}

	s := NewStaging()
	require.NoError(t, s.Load(&private.StagingSnapshot{
		Block: 500,
		Streams: []*private.StagedStream{{
			Ledger: ledger, Source: src, Delivered: 100, Sighted: 101,
			Entries: []*private.StagedEntry{{
				Number:    101,
				Message:   seq,
				Collected: false, // no proof, no validated hash, no signature
			}},
		}},
	}))

	tx := s.Begin()
	held, ok := tx.IDOf(id, 101)
	require.True(t, ok)
	require.False(t, held.Collected, "runnable() short-circuits to true on this")
	_, ok = tx.Validated(id, 101)
	require.False(t, ok, "and nothing validated it")
}
