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

// Committing a delivery releases everything at or below it from both lists.
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
	require.True(t, tx.IsValidated(testStream, 1, entries[0].Hash))
	require.Equal(t, uint64(4), tx.Reach(testStream))
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
	require.False(t, tx.IsValidated(testStream, 2, entries[1].Hash), "validated hashes pruned through the delivered number")
	require.True(t, tx.IsValidated(testStream, 3, entries[2].Hash))
	require.Equal(t, uint64(4), tx.Reach(testStream))
	_, ok = s.Begin().HeldByID(entries[1].ID)
	require.False(t, ok)
}

// Two proofs claiming different hashes for one number conflict; the first
// stands. A proof's element i at chain index s is number s+i+1, and a proof
// below what is validated fills in behind.
func TestStaging_ProveConflictAndExtension(t *testing.T) {
	s := NewStaging()
	a, b, c := [32]byte{1}, [32]byte{2}, [32]byte{3}
	tx := s.Begin()
	require.NoError(t, tx.Prove(testStream, proofList(2, a, b))) // numbers 3, 4
	tx.Commit()

	tx = s.Begin()
	err := tx.Prove(testStream, proofList(3, c))
	require.ErrorIs(t, err, errors.Conflict, "number 4 is validated as b")
	require.NoError(t, tx.Prove(testStream, proofList(3, b)), "agreeing is fine")
	require.NoError(t, tx.Prove(testStream, proofList(0, [32]byte{9}, [32]byte{8})), "fills in behind")
	v, ok := tx.Validated(testStream, 2)
	require.True(t, ok)
	require.Equal(t, [32]byte{8}, v)
	tx.Commit()
	v, ok = s.Begin().Validated(testStream, 3)
	require.True(t, ok)
	require.Equal(t, a, v)
	require.Equal(t, uint64(4), s.Begin().Reach(testStream))
}

// A collected entry a validated proof contradicts is not the stream's entry:
// it is dropped (or refused) so the number is a hole to ask for again, and the
// entry with the validated hash is held when it comes. An entry that proved
// itself is left alone.
func TestStaging_DisprovedEntryIsDropped(t *testing.T) {
	s := NewStaging()
	wrong := held(3)
	wrong.Collected = true
	wrong.Hash = [32]byte{0xBA, 0xD}
	right := held(3)
	right.Collected = true

	// Held first, disproved by a later proof
	tx := s.Begin()
	tx.Hold(testStream, 3, wrong)
	tx.Commit()
	tx = s.Begin()
	require.NoError(t, tx.Prove(testStream, proofList(2, right.Hash)))
	tx.Commit()
	_, ok := s.Begin().IDOf(testStream, 3)
	require.False(t, ok, "dropped at commit")

	// Proof first, the wrong entry refused, the right one held
	tx = s.Begin()
	tx.Hold(testStream, 3, wrong)
	_, ok = tx.IDOf(testStream, 3)
	require.False(t, ok, "refused")
	tx.Hold(testStream, 3, right)
	h, ok := tx.IDOf(testStream, 3)
	require.True(t, ok)
	require.Same(t, right, h)
	require.True(t, tx.IsValidated(testStream, 3, right.Hash))
	tx.Commit()

	// An own-proof entry is not touched by a disagreeing list: the list
	// conflicts with nothing validated, so it is recorded, and the entry
	// stays runnable on its own proof.
	own := held(5)
	tx = s.Begin()
	tx.Hold(testStream, 5, own)
	require.NoError(t, tx.Prove(testStream, proofList(4, [32]byte{7})))
	tx.Commit()
	h, ok = s.Begin().IDOf(testStream, 5)
	require.True(t, ok)
	require.Same(t, own, h)
}

// Numbers at or below Delivered and beyond the span are neither held nor
// validated; the lists start at Delivered + 1.
func TestStaging_Bounds(t *testing.T) {
	s := NewStaging()
	tx := s.Begin()
	tx.Hold(testStream, 1, held(1))
	tx.Release(testStream, 1)
	tx.Commit()

	tx = s.Begin()
	tx.Hold(testStream, 1, held(1))
	require.NoError(t, tx.Prove(testStream, proofList(0, [32]byte{1}, [32]byte{2})))
	tx.Hold(testStream, MaxStageSpan+10, held(MaxStageSpan+10))
	tx.Commit()
	tx = s.Begin()
	_, ok := tx.IDOf(testStream, 1)
	require.False(t, ok, "at Delivered")
	_, ok = tx.Validated(testStream, 1)
	require.False(t, ok, "at Delivered")
	require.True(t, tx.IsValidated(testStream, 2, [32]byte{2}))
	_, ok = tx.IDOf(testStream, MaxStageSpan+10)
	require.False(t, ok, "beyond the span")
	require.Equal(t, uint64(1), tx.Sighted(testStream))
}

// proofList is a collection proof whose first element sits at chain index
// start: numbers start+1 onwards.
func proofList(start int64, hashes ...[32]byte) *merkle.ReceiptList {
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

// A drained backlog does not stay pinned: releasing everything held returns
// the lists' capacity and drops every reference.
func TestStaging_ReleaseReturnsTheBacklog(t *testing.T) {
	s := NewStaging()
	tx := s.Begin()
	const backlog = 5000
	for n := uint64(1); n <= backlog; n++ {
		tx.Hold(testStream, n, held(n))
	}
	tx.Commit()
	st := s.streams[testStream.key()]
	require.GreaterOrEqual(t, cap(st.entries), backlog)

	tx = s.Begin()
	tx.Release(testStream, backlog)
	tx.Commit()
	require.Len(t, st.entries, 0)
	require.Less(t, cap(st.entries), 2048, "the backing array of a drained backlog is returned")
	require.Less(t, cap(st.validated), 2048)
	require.Empty(t, s.byID, "no released entry stays indexed")

	// A partial release clears what it drops
	tx = s.Begin()
	for n := uint64(backlog + 1); n <= backlog+100; n++ {
		tx.Hold(testStream, n, held(n))
	}
	tx.Release(testStream, backlog+50)
	tx.Commit()
	require.Len(t, st.entries, 50)
	require.Len(t, s.byID, 50)
}

// The same proof arriving twice under one anchor block is held once: every
// copy of a package's members carries the package's proof, and under
// Directory-anchor lag the copies keep coming (review 2026-09-06, finding 28).
// A different span under the same block is a different proof.
func TestStaging_DuplicateProofsAreNotStacked(t *testing.T) {
	s := NewStaging()
	proofOver := func(start int64, elements int) *protocol.AnnotatedReceipt {
		list := merkle.NewReceiptList()
		list.MerkleState = &merkle.State{Count: start}
		for i := 0; i < elements; i++ {
			list.Elements = append(list.Elements, held(uint64(start) + uint64(i) + 1).Hash[:])
		}
		return &protocol.AnnotatedReceipt{Anchor: &protocol.AnchorMetadata{SourceBlock: 7}, ReceiptList: list}
	}
	tx := s.Begin()
	require.True(t, tx.StageProof(testSource, 7, proofOver(0, 3)))
	require.False(t, tx.StageProof(testSource, 7, proofOver(0, 3)), "the same span, the same block: not held again")
	require.True(t, tx.StageProof(testSource, 7, proofOver(3, 2)), "a different span is a different proof")
	require.Len(t, tx.Proofs(testSource, 7), 2)
	tx.Commit()

	tx = s.Begin()
	require.False(t, tx.StageProof(testSource, 7, proofOver(0, 3)), "nor across blocks: the committed one stands")
	require.Len(t, tx.Proofs(testSource, 7), 2)
	require.ElementsMatch(t, [][2]uint64{{1, 3}, {4, 5}}, tx.StagedProofSpans(testSource), "the numbers the waiting proofs cover")
	tx.DropProofs(testSource, 7)
	require.Empty(t, tx.StagedProofSpans(testSource), "dropped proofs cover nothing")
}

// A block reads where every stream stands, to log it: delivered, sighted,
// held, and the first number it is waiting on. This is the record a stall
// is debugged from (executor spec, "What a stream logs").
func TestStaging_Streams(t *testing.T) {
	s := NewStaging()
	tx := s.Begin()
	tx.Hold(testStream, 1, held(1))
	tx.Hold(testStream, 2, held(2))
	tx.Hold(testStream, 3, held(3))
	tx.Hold(testStream, 5, held(5))
	tx.Release(testStream, 1)
	tx.Commit()

	all := s.Begin().Streams()
	require.Len(t, all, 1)
	st := all[0]
	require.Equal(t, testStream, st.ID)
	require.EqualValues(t, 1, st.Delivered)
	require.EqualValues(t, 5, st.Sighted)
	require.Equal(t, 3, st.Held, "2, 3 and 5")
	require.EqualValues(t, 4, st.Waiting, "the first hole above Delivered")
	require.True(t, st.Behind())

	// A block that releases sees its own release before it commits
	tx = s.Begin()
	tx.Release(testStream, 3)
	st = tx.Status(testStream)
	require.EqualValues(t, 3, st.Delivered)
	require.EqualValues(t, 4, st.Waiting)
	tx.Discard()

	// A stream nothing has touched has nothing to say
	other := StreamID{Ledger: testLedger, Source: protocol.PartitionUrl("BVN9")}
	require.Equal(t, StreamStatus{ID: other}, s.Begin().Status(other))
}

// Status walks for the first hole, and that walk is bounded: a stage may
// legitimately hold an hour of a source's production, and Status runs once
// per stream per block to log it. Waiting is the first hole WITHIN the
// window; zero means there is none in it, which with Held says backlog
// rather than gap.
func TestStaging_StatusScanIsBounded(t *testing.T) {
	s := NewStaging()
	tx := s.Begin()
	// A contiguous run well past the window, then a hole beyond it.
	for n := uint64(1); n <= StatusScan+10; n++ {
		tx.Hold(testStream, n, held(n))
	}
	tx.Hold(testStream, StatusScan+12, held(StatusScan+12)) // hole at +11
	tx.Commit()

	st := s.Begin().Status(testStream)
	require.EqualValues(t, StatusScan+12, st.Sighted)
	require.Zero(t, st.Waiting, "the hole is past the window: a backlog, not a gap")
	require.Equal(t, int(StatusScan+11), st.Held)

	// A hole inside the window is found.
	s2 := NewStaging()
	tx = s2.Begin()
	tx.Hold(testStream, 2, held(2))
	tx.Commit()
	require.EqualValues(t, 1, s2.Begin().Status(testStream).Waiting)
}
