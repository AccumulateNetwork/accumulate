// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	api "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var anchorStream = StreamID{Ledger: protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool), Source: testSource}

func anchorHeld(n uint64) *Held {
	seq := &messaging.SequencedMessage{Number: n, Source: testSource, Destination: protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool)}
	return &Held{ID: seq.ID(), Message: seq, Collected: true, Hash: seq.Hash()}
}

func annotated(block uint64, start int64, elements int) *protocol.AnnotatedReceipt {
	var hashes [][32]byte
	for i := 0; i < elements; i++ {
		var h [32]byte
		h[0] = byte(block)
		h[1] = byte(i)
		hashes = append(hashes, h)
	}
	return &protocol.AnnotatedReceipt{
		Anchor:      &protocol.AnchorMetadata{SourceBlock: block},
		ReceiptList: proofList(start, hashes...),
	}
}

// Staging taken as of a committed block and loaded into an empty staging
// stands where the original stood: the same block applied to both leaves the
// same streams, the same held entries, the same validated hashes and the same
// waiting proofs (executor spec, "Sync" step 2).
func TestStaging_SnapshotAndLoad(t *testing.T) {
	s := NewStaging()

	// Block 1: three entries held on the synthetic stream, two of them
	// validated, one anchor copy held below its quorum, a proof waiting on
	// Directory block 7, and number 1 delivered.
	tx := s.Begin()
	tx.AtBlock(1)
	tx.Hold(testStream, 1, held(1))
	tx.Hold(testStream, 2, held(2))
	tx.Hold(testStream, 3, held(3))
	tx.Hold(anchorStream, 1, anchorHeld(1))
	require.NoError(t, tx.Prove(testStream, proofList(0, held(1).Hash, held(2).Hash)))
	require.True(t, tx.StageProof(testSource, 7, annotated(7, 0, 2)))
	tx.Release(testStream, 1)
	tx.Commit()

	// Block 2: one more entry on each stream, one more proof. Nothing is
	// delivered, so the stage grows.
	tx = s.Begin()
	tx.AtBlock(2)
	tx.Hold(testStream, 4, held(4))
	tx.Hold(anchorStream, 2, anchorHeld(2))
	require.True(t, tx.StageProof(testSource, 8, annotated(8, 2, 2)))
	tx.Commit()

	// The snapshot is taken between blocks 2 and 3, and it is as of block 2.
	snap, _ := s.Snapshot(nil)
	require.Equal(t, uint64(2), snap.Block)
	require.False(t, snap.More, "the whole stage fits in one page")
	require.Len(t, snap.Streams, 2)

	fresh := NewStaging()
	require.NoError(t, fresh.Load(snap))

	// A staging that holds something is not loaded over.
	require.Error(t, fresh.Load(snap))

	// Block 3 is fed to both: the same arrivals, as consensus gives them to
	// every node.
	block3 := held(5)
	for _, st := range []*Staging{s, fresh} {
		tx := st.Begin()
		tx.AtBlock(3)
		tx.Hold(testStream, 5, block3)
		require.NoError(t, tx.Prove(testStream, proofList(2, held(3).Hash, held(4).Hash)))
		tx.Release(testStream, 2)
		tx.DropProofs(testSource, 7)
		tx.Commit()
	}

	a, b := s.Begin(), fresh.Begin()
	defer a.Discard()
	defer b.Discard()

	require.Equal(t, a.Streams(), b.Streams())
	for _, id := range []StreamID{testStream, anchorStream} {
		for n := uint64(1); n <= 6; n++ {
			ha, oka := a.IDOf(id, n)
			hb, okb := b.IDOf(id, n)
			require.Equal(t, oka, okb, "%v holds %d", id.Ledger, n)
			require.Equal(t, ha, hb, "%v at %d", id.Ledger, n)

			va, oka := a.Validated(id, n)
			vb, okb := b.Validated(id, n)
			require.Equal(t, oka, okb, "%v validated %d", id.Ledger, n)
			require.Equal(t, va, vb, "%v validated %d", id.Ledger, n)
		}
	}
	require.NotEmpty(t, a.ProofBlocks(testSource))
	require.Equal(t, a.ProofBlocks(testSource), b.ProofBlocks(testSource))
	for _, blk := range a.ProofBlocks(testSource) {
		require.Equal(t, a.Proofs(testSource, blk), b.Proofs(testSource, blk))
	}
	require.Equal(t, a.ProofSources(), b.ProofSources())
}

// A snapshot pages by stream, and every page says which block it is as of.
func TestStaging_SnapshotPages(t *testing.T) {
	s := NewStaging()
	tx := s.Begin()
	tx.AtBlock(9)
	for n := uint64(1); n <= 6; n++ {
		tx.Hold(testStream, n, held(n))
		tx.Hold(anchorStream, n, anchorHeld(n))
	}
	tx.Commit()

	var streams []*private.StagedStream
	var ledger, source *url.URL
	var number uint64
	for i := 0; ; i++ {
		require.Less(t, i, 20, "paging does not terminate")
		page, _ := s.Snapshot(&private.StagingSnapshotRequest{Ledger: ledger, Source: source, Number: number, Limit: 4})
		require.Equal(t, uint64(9), page.Block, "every page says which block it is as of")
		streams = append(streams, page.Streams...)
		if !page.More {
			break
		}
		ledger, source, number = page.NextLedger, page.NextSource, page.NextNumber
	}

	// Twelve entries in pages of four: every entry, once, in order.
	got := map[string][]uint64{}
	for _, st := range streams {
		for _, e := range st.Entries {
			got[st.Ledger.String()] = append(got[st.Ledger.String()], e.Number)
		}
	}
	require.Equal(t, []uint64{1, 2, 3, 4, 5, 6}, got[testStream.Ledger.String()])
	require.Equal(t, []uint64{1, 2, 3, 4, 5, 6}, got[anchorStream.Ledger.String()])

	// The whole of it, in one page, loads.
	whole, _ := s.Snapshot(nil)
	fresh := NewStaging()
	require.NoError(t, fresh.Load(whole))
	a, b := s.Begin(), fresh.Begin()
	defer a.Discard()
	defer b.Discard()
	require.Equal(t, a.Streams(), b.Streams())
}

// Staging that no block has ever committed into is not a snapshot of
// anything: the block index is zero and the caller refuses it.
func TestStaging_SnapshotBeforeAnyBlock(t *testing.T) {
	snap, _ := NewStaging().Snapshot(nil)
	require.Zero(t, snap.Block)
}

// stagingPeer serves a staging the way a validator does, so a test can read
// it the way a joining node does: through the private API's own page loop,
// with the request validated as the server validates it.
type stagingPeer struct {
	s     *Staging
	limit uint64
	pages int
}

func (p *stagingPeer) Sequence(context.Context, *url.URL, *url.URL, uint64, private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	return nil, errors.NotFound
}

func (p *stagingPeer) StagingSnapshot(_ context.Context, req *private.StagingSnapshotRequest) (*private.StagingSnapshot, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	p.pages++
	r := *req
	r.Limit = p.limit
	snap, _ := p.s.Snapshot(&r)
	return snap, nil
}

// A source that holds proofs and no stream is carried by a page of its own,
// whose Ledger is nil. A page that ends just before it must still say there
// is more, and the reader must be able to ask for it: a nil Ledger is a
// position, not an ending. Reading it as an ending dropped the carrier and
// everything after it, so the proof never arrived and the entries it proves
// stayed collected and unvalidated for good (#4291 review).
func TestStaging_SnapshotPagesToAProofOnlySource(t *testing.T) {
	other := protocol.PartitionUrl("BVN2")

	s := NewStaging()
	tx := s.Begin()
	tx.AtBlock(11)
	for n := uint64(1); n <= 6; n++ {
		tx.Hold(testStream, n, held(n))
	}
	require.True(t, tx.StageProof(other, 21, annotated(21, 0, 2)))
	tx.Commit()

	// Six entries in a page of six: the page ends with the stream, and the
	// carrier for BVN2 has not been served.
	first, _ := s.Snapshot(&private.StagingSnapshotRequest{Limit: 6})
	require.True(t, first.More, "the carrier has not been served")
	require.Nil(t, first.NextLedger, "a source with no stream has no ledger")
	require.NotNil(t, first.NextSource, "but it does have a source")
	require.True(t, other.Equal(first.NextSource))
	for _, st := range first.Streams {
		require.Empty(t, st.Proofs, "the carrier's proofs are not in this page")
	}

	// Read whole, the way a joining node reads it, the proof arrives.
	peer := &stagingPeer{s: s, limit: 6}
	whole, err := private.FetchStagingSnapshot(context.Background(), peer, "BVN0")
	require.NoError(t, err)
	require.Equal(t, 2, peer.pages, "the carrier took a page of its own")

	fresh := NewStaging()
	require.NoError(t, fresh.Load(whole))
	b := fresh.Begin()
	defer b.Discard()
	require.Equal(t, []uint64{21}, b.ProofBlocks(other), "the waiting proof was delivered")
	for n := uint64(1); n <= 6; n++ {
		_, ok := b.IDOf(testStream, n)
		require.True(t, ok, "and so was entry %d", n)
	}
}

// A start number past the end of a stream carries nothing from it and costs
// nothing. Charging the empty range wrapped the page budget, so every stream
// after it was served in full and the limit meant nothing — one field, from
// any peer, and the answer is the whole stage (#4291 review).
func TestStaging_SnapshotStartPastTheEndOfAStream(t *testing.T) {
	s := NewStaging()
	tx := s.Begin()
	tx.AtBlock(7)
	for n := uint64(1); n <= 3; n++ {
		tx.Hold(anchorStream, n, anchorHeld(n))
	}
	for n := uint64(1); n <= 10; n++ {
		tx.Hold(testStream, n, held(n))
	}
	tx.Commit()

	// The anchor stream sorts first, and the page starts past the end of it.
	page, _ := s.Snapshot(&private.StagingSnapshotRequest{
		Ledger: anchorStream.Ledger,
		Source: anchorStream.Source,
		Number: 99,
		Limit:  2,
	})

	entries := 0
	for _, st := range page.Streams {
		entries += len(st.Entries)
	}
	require.Equal(t, 2, entries, "the limit still bounds the page")
	require.True(t, page.More, "and the rest is still to come")
	require.Equal(t, uint64(3), page.NextNumber)
}

// A source may hold up to MaxStagedProofBytes of waiting proofs, and a proof
// stands at no sequence number, so the span limit does not bound them. They
// are charged bytes and paged (#4291 review).
func TestStaging_SnapshotPagesProofs(t *testing.T) {
	restore := MaxSnapshotBytes
	defer func() { MaxSnapshotBytes = restore }()

	s := NewStaging()
	tx := s.Begin()
	tx.AtBlock(5)
	tx.Hold(testStream, 1, held(1))
	for b := uint64(20); b < 26; b++ {
		require.True(t, tx.StageProof(testSource, b, annotated(b, 0, 4)))
	}
	tx.Commit()

	whole, size := s.Snapshot(nil)
	require.False(t, whole.More, "the whole of it fits in one page at the real budget")
	require.Len(t, whole.Streams, 1)
	require.Len(t, whole.Streams[0].Proofs, 6)
	require.Greater(t, size, 0, "and the page says what it cost")

	// At a budget that holds two proofs, the source's proofs are paged.
	MaxSnapshotBytes = 2 * proofSize(annotated(20, 0, 4))
	peer := &stagingPeer{s: s}
	read, err := private.FetchStagingSnapshot(context.Background(), peer, "BVN0")
	require.NoError(t, err)
	require.Greater(t, peer.pages, 2, "the proofs took more than one page")

	proofs := 0
	for _, st := range read.Streams {
		proofs += len(st.Proofs)
	}
	require.Equal(t, 6, proofs, "every proof arrived, once")

	fresh := NewStaging()
	require.NoError(t, fresh.Load(read))
	b := fresh.Begin()
	defer b.Discard()
	require.Equal(t, []uint64{20, 21, 22, 23, 24, 25}, b.ProofBlocks(testSource))
	_, ok := b.IDOf(testStream, 1)
	require.True(t, ok, "and the entry after them")
}

// A cursor with a source and no ledger is a carrier, and one with neither is
// the beginning. Neither may panic: the call is reachable over the wire, and
// #4294's join makes it in process, where a panic is fatal (#4291 review).
func TestStaging_SnapshotCursorWithoutALedger(t *testing.T) {
	s := NewStaging()
	tx := s.Begin()
	tx.AtBlock(2)
	tx.Hold(testStream, 1, held(1))
	tx.Commit()

	require.NotPanics(t, func() {
		page, _ := s.Snapshot(&private.StagingSnapshotRequest{Source: testSource})
		require.False(t, page.More, "there is no carrier for this source")
		require.Empty(t, page.Streams)
	})

	// And the server refuses what names no stream at all.
	require.ErrorIs(t, (&private.StagingSnapshotRequest{}).Validate(), errors.BadRequest)
	require.ErrorIs(t, (&private.StagingSnapshotRequest{Partition: "BVN0", Ledger: testLedger}).Validate(), errors.BadRequest)
	require.ErrorIs(t, (&private.StagingSnapshotRequest{Partition: "BVN0", Number: 4}).Validate(), errors.BadRequest)
	require.NoError(t, (&private.StagingSnapshotRequest{Partition: "BVN0"}).Validate())
	require.NoError(t, (&private.StagingSnapshotRequest{Partition: "BVN0", Source: testSource}).Validate())
}

// Load publishes or it does nothing. A snapshot with one bad entry in it
// leaves staging empty and the load can be tried again — against the same
// peer or another one. A load that wrote as it went left a staging that was
// neither loaded nor empty, and Load refuses one that is not empty, so one
// malformed entry wedged a joining node for good (#4291 review).
func TestStaging_LoadIsAllOrNothing(t *testing.T) {
	s := NewStaging()
	tx := s.Begin()
	tx.AtBlock(4)
	tx.Hold(testStream, 1, held(1))
	tx.Hold(testStream, 2, held(2))
	require.True(t, tx.StageProof(testSource, 9, annotated(9, 0, 2)))
	tx.Commit()

	// Two snapshots of the same stage: the first is spoiled, the second is
	// what a second attempt would fetch.
	bad, _ := s.Snapshot(nil)
	good, _ := s.Snapshot(nil)
	entries := bad.Streams[0].Entries
	require.NotEmpty(t, bad.Streams[0].Proofs, "the proof is read before the bad entry")
	entries[len(entries)-1].Message = nil

	fresh := NewStaging()
	require.Error(t, fresh.Load(bad))

	empty := fresh.Begin()
	require.Empty(t, empty.Streams(), "a failed load leaves staging empty")
	require.Empty(t, empty.ProofSources(), "including its proofs")
	empty.Discard()

	require.NoError(t, fresh.Load(good), "so the load can be tried again")
	b := fresh.Begin()
	defer b.Discard()
	require.Len(t, b.Streams(), 1)
	require.Equal(t, []uint64{9}, b.ProofBlocks(testSource))
}

// What a peer's snapshot may bring in is bounded as what a source may stage
// here is bounded, in the same currency and at the same figure (#4291
// review).
func TestStaging_LoadBoundsProofBytes(t *testing.T) {
	restore := MaxStagedProofBytes
	defer func() { MaxStagedProofBytes = restore }()

	s := NewStaging()
	tx := s.Begin()
	tx.AtBlock(6)
	tx.Hold(testStream, 1, held(1))
	require.True(t, tx.StageProof(testSource, 12, annotated(12, 0, 4)))
	tx.Commit()
	snap, _ := s.Snapshot(nil)

	MaxStagedProofBytes = 1 // any proof exceeds it
	fresh := NewStaging()
	require.ErrorIs(t, fresh.Load(snap), errors.BadRequest)

	b := fresh.Begin()
	defer b.Discard()
	require.Empty(t, b.Streams(), "and the refused load left nothing behind")
}
