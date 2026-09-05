// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	testLedger = protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic)
	testSource = protocol.PartitionUrl("BVN1")
	testStream = StreamID{Ledger: testLedger, Source: testSource}
)

func held(n uint64) *Held {
	seq := &messaging.SequencedMessage{Number: n, Source: testSource, Destination: protocol.PartitionUrl("BVN0")}
	return &Held{ID: seq.ID(), Message: seq, Hash: seq.Hash()}
}

// A block sees what it holds while it runs; nothing is visible to the next
// block until it commits; a discarded block leaves nothing behind.
func TestStaging_BlockTransaction(t *testing.T) {
	s := NewStaging()
	tx := s.Begin()
	tx.Hold(testStream, 3, held(3))
	_, ok := tx.IDOf(testStream, 3)
	require.True(t, ok, "the block sees its own hold")
	require.Equal(t, uint64(3), tx.Sighted(testStream))

	other := s.Begin()
	_, ok = other.IDOf(testStream, 3)
	require.False(t, ok, "not committed yet")

	tx.Discard()
	_, ok = s.Begin().IDOf(testStream, 3)
	require.False(t, ok, "discarded")

	tx = s.Begin()
	tx.Hold(testStream, 3, held(3))
	tx.Commit()
	h, ok := s.Begin().IDOf(testStream, 3)
	require.True(t, ok)
	require.Equal(t, uint64(3), h.Message.(*messaging.SequencedMessage).Number)
	require.Equal(t, uint64(3), s.SightedOn(testStream))
	byID, ok := s.Begin().HeldByID(h.ID)
	require.True(t, ok)
	require.Same(t, h, byID)
}

// The first sighting of a number wins, and sighted never goes backwards.
func TestStaging_FirstSightingWins(t *testing.T) {
	s := NewStaging()
	tx := s.Begin()
	first := held(5)
	tx.Hold(testStream, 5, first)
	tx.Hold(testStream, 5, held(5))
	tx.Commit()
	tx = s.Begin()
	tx.Hold(testStream, 5, held(5))
	tx.Hold(testStream, 2, held(2))
	tx.Commit()
	h, _ := s.Begin().IDOf(testStream, 5)
	require.Same(t, first, h)
	require.Equal(t, uint64(5), s.SightedOn(testStream))
}

// Missing lists the holes between delivered and sighted, oldest first, and
// no more runs than asked.
func TestStaging_Missing(t *testing.T) {
	s := NewStaging()
	tx := s.Begin()
	for _, n := range []uint64{3, 4, 7, 10} {
		tx.Hold(testStream, n, held(n))
	}
	tx.Commit()
	tx = s.Begin()
	require.Equal(t, [][2]uint64{{1, 2}, {5, 6}, {8, 9}}, tx.Missing(testStream, 0, 10, 10))
	require.Equal(t, [][2]uint64{{1, 2}, {5, 6}}, tx.Missing(testStream, 0, 10, 2))
	require.Empty(t, tx.Missing(testStream, 10, 10, 10))
}

// Committing a delivery releases everything held at or below it, and the
// proven set below the last delivered entry's index.
func TestStaging_ReleaseOnCommit(t *testing.T) {
	s := NewStaging()
	list := merkle.NewReceiptList()
	list.MerkleState = new(merkle.State)
	var entries []*Held
	for n := uint64(1); n <= 4; n++ {
		h := held(n)
		entries = append(entries, h)
		list.Elements = append(list.Elements, h.Hash[:])
	}
	tx := s.Begin()
	for _, h := range entries {
		h.Collected = true
		tx.Hold(testStream, h.Message.(*messaging.SequencedMessage).Number, h)
	}
	require.NoError(t, tx.Prove(testStream, list))
	require.True(t, tx.IsProven(testStream, entries[0].Hash))
	tx.Commit()

	tx = s.Begin()
	tx.Release(testStream, 2)
	_, ok := tx.IDOf(testStream, 2)
	require.True(t, ok, "release applies at commit, not before")
	tx.Commit()

	tx = s.Begin()
	_, ok = tx.IDOf(testStream, 2)
	require.False(t, ok, "released")
	_, ok = tx.IDOf(testStream, 3)
	require.True(t, ok, "above the release")
	require.False(t, tx.IsProven(testStream, entries[1].Hash), "proven set pruned through the delivered entry")
	require.True(t, tx.IsProven(testStream, entries[2].Hash))
	_, ok = s.Begin().HeldByID(entries[1].ID)
	require.False(t, ok)
}

// Two proofs claiming the same index with different hashes conflict; the
// first stands. A proof below what is proven extends the set backwards.
func TestStaging_ProveConflictAndExtension(t *testing.T) {
	s := NewStaging()
	mk := func(start int64, hashes ...[32]byte) *merkle.ReceiptList {
		l := merkle.NewReceiptList()
		st := new(merkle.State)
		for i := int64(0); i < start; i++ {
			var pad [32]byte
			pad[0] = byte(i + 100)
			st.AddEntry(pad[:])
		}
		l.MerkleState = st
		for _, h := range hashes {
			h := h
			l.Elements = append(l.Elements, h[:])
		}
		return l
	}
	a, b, c := [32]byte{1}, [32]byte{2}, [32]byte{3}
	tx := s.Begin()
	require.NoError(t, tx.Prove(testStream, mk(2, a, b)))
	tx.Commit()

	tx = s.Begin()
	err := tx.Prove(testStream, mk(3, c))
	require.ErrorIs(t, err, errors.Conflict, "index 3 is proven as b")
	require.NoError(t, tx.Prove(testStream, mk(3, b)), "agreeing is fine")
	require.NoError(t, tx.Prove(testStream, mk(0, [32]byte{9}, [32]byte{8})), "extends backwards")
	idx, ok := tx.ProvenIndex(testStream, [32]byte{8})
	require.True(t, ok)
	require.Equal(t, int64(1), idx)
	tx.Commit()
	idx, ok = s.Begin().ProvenIndex(testStream, a)
	require.True(t, ok)
	require.Equal(t, int64(2), idx)
}

// Proofs wait under their anchor block per source; they are listed, decided
// and dropped; a dropped block is gone for the block that dropped it and for
// every block after.
func TestStaging_Proofs(t *testing.T) {
	s := NewStaging()
	p := func(b uint64) *protocol.AnnotatedReceipt {
		return &protocol.AnnotatedReceipt{Anchor: &protocol.AnchorMetadata{SourceBlock: b}}
	}
	tx := s.Begin()
	tx.StageProof(testSource, 7, p(7))
	tx.StageProof(testSource, 9, p(9))
	require.Equal(t, []uint64{7, 9}, tx.ProofBlocks(testSource))
	require.Len(t, tx.ProofSources(), 1)
	tx.Commit()

	tx = s.Begin()
	tx.StageProof(testSource, 7, p(7))
	require.Len(t, tx.Proofs(testSource, 7), 2, "committed plus this block's")
	tx.DropProofs(testSource, 7)
	require.Empty(t, tx.Proofs(testSource, 7))
	require.Equal(t, []uint64{9}, tx.ProofBlocks(testSource))
	tx.Commit()
	require.Equal(t, []uint64{9}, s.Begin().ProofBlocks(testSource))
}
