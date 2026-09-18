// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package execute

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
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
	snap := s.Snapshot(nil, nil, 0, 0)
	require.Equal(t, uint64(2), snap.Block)
	require.Nil(t, snap.NextLedger, "the whole stage fits in one page")
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
		page := s.Snapshot(ledger, source, number, 4)
		require.Equal(t, uint64(9), page.Block, "every page says which block it is as of")
		streams = append(streams, page.Streams...)
		if page.NextLedger == nil {
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
	whole := s.Snapshot(nil, nil, 0, 0)
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
	require.Zero(t, NewStaging().Snapshot(nil, nil, 0, 0).Block)
}
