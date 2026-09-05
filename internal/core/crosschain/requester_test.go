// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	reqSource = protocol.PartitionUrl("BVN1")
	reqStream = execute.StreamID{Ledger: protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic), Source: reqSource}
)

func reqHeld(n uint64, collected bool) *execute.Held {
	seq := &messaging.SequencedMessage{Number: n, Source: reqSource, Destination: protocol.PartitionUrl("BVN0")}
	return &execute.Held{ID: seq.ID(), Message: seq, Hash: seq.Hash(), Collected: collected}
}

// A staging that holds the given indexes; collected ones are unproven unless
// listed in proven.
func reqStaging(t *testing.T, held map[uint64]bool, proven ...uint64) *execute.Staging {
	s := execute.NewStaging()
	tx := s.Begin()
	hashes := map[uint64][32]byte{}
	for n, collected := range held {
		h := reqHeld(n, collected)
		hashes[n] = h.Hash
		tx.Hold(reqStream, n, h)
	}
	if len(proven) > 0 {
		list := merkle.NewReceiptList()
		st := new(merkle.State)
		list.MerkleState = st
		for _, n := range proven {
			h := hashes[n]
			list.Elements = append(list.Elements, h[:])
		}
		require.NoError(t, tx.Prove(reqStream, list))
	}
	tx.Commit()
	return s
}

// Two senders are drawn from the seed, distinct, and the same for the same
// seed on every validator; a single validator always sends.
func TestPullSenders(t *testing.T) {
	require.Equal(t, []int{0}, pullSenders([]byte("x"), 1))
	seen := map[int]bool{}
	for i := byte(0); i < 50; i++ {
		s := pullSenders([]byte{i}, 4)
		require.Len(t, s, 2)
		require.NotEqual(t, s[0], s[1])
		require.Equal(t, s, pullSenders([]byte{i}, 4), "deterministic")
		seen[s[0]] = true
	}
	require.Len(t, seen, 4, "every position is drawn over enough blocks")
}

// A hole below a sighted entry is asked after healNoticeAge activations, then
// not again until healPatience activations have passed; a held and runnable
// entry is never asked; a collected entry without a proof is.
func TestDecide_NoticeAndPatience(t *testing.T) {
	// 1 missing, 2 collected+unproven, 3 collected+proven, 4 own-proof held
	s := reqStaging(t, map[uint64]bool{2: true, 3: true, 4: false}, 3)
	var r healRequester
	tx := s.Begin()
	defer tx.Discard()

	require.Empty(t, r.decide(tx, reqStream, 0, 0, 8), "first sighting is remembered, not asked")
	require.Empty(t, r.decide(tx, reqStream, 0, 0, 12), "one activation is too soon")
	spans := r.decide(tx, reqStream, 0, 0, 16)
	require.Equal(t, [][2]uint64{{1, 2}}, spans, "the hole and the unproven entry, coalesced; 3 and 4 are runnable")

	r.asked(reqStream, spans[0], 16)
	require.Empty(t, r.decide(tx, reqStream, 0, 0, 20), "asked: patience")
	require.Empty(t, r.decide(tx, reqStream, 0, 0, 24))
	require.Equal(t, [][2]uint64{{1, 2}}, r.decide(tx, reqStream, 0, 0, 28), "patience over: asked again")

	require.Empty(t, r.decide(tx, reqStream, 4, 0, 32), "delivered past everything: nothing, and the memory is pruned")
	require.Empty(t, r.gaps[streamKey(reqStream)])
}

// A collected entry whose proof is staged, waiting for its anchor, is not a
// gap: the anchor is on its way. Once the proof is dropped it is.
func TestDecide_StagedProofIsNotAGap(t *testing.T) {
	s := reqStaging(t, map[uint64]bool{1: true})
	tx := s.Begin()
	h, _ := tx.IDOf(reqStream, 1)
	list := merkle.NewReceiptList()
	list.MerkleState = new(merkle.State)
	list.Elements = [][]byte{h.Hash[:]}
	tx.StageProof(reqSource, 42, &protocol.AnnotatedReceipt{ReceiptList: list, Anchor: &protocol.AnchorMetadata{SourceBlock: 42}})
	tx.Commit()

	var r healRequester
	tx = s.Begin()
	defer tx.Discard()
	r.decide(tx, reqStream, 0, 0, 4)
	require.Empty(t, r.decide(tx, reqStream, 0, 0, 4+healNoticeAge*healCadence), "its proof is staged")
	tx.DropProofs(reqSource, 42)
	at := uint64(4 + healNoticeAge*healCadence)
	require.Empty(t, r.decide(tx, reqStream, 0, 0, at), "the proof is gone: the gap is first seen now")
	require.Equal(t, [][2]uint64{{1, 1}}, r.decide(tx, reqStream, 0, 0, at+healNoticeAge*healCadence), "and asked after the notice")
}

// An unsighted tail the source says it produced is a gap after the longer
// notice; separate holes become separate spans, oldest first, capped.
func TestDecide_ExpectedTailAndSpans(t *testing.T) {
	s := reqStaging(t, map[uint64]bool{})
	var r healRequester
	tx := s.Begin()
	defer tx.Discard()

	require.Empty(t, r.decide(tx, reqStream, 0, 5, 4))
	require.Empty(t, r.decide(tx, reqStream, 0, 5, 4+healNoticeAge*healCadence), "sighted-notice is not enough for an unsighted tail")
	require.Equal(t, [][2]uint64{{1, 5}}, r.decide(tx, reqStream, 0, 5, 4+healExpectedAge*healCadence))

	// Many single holes: at most MaxRequestSpans, the oldest.
	held := map[uint64]bool{}
	for n := uint64(2); n <= 100; n += 2 {
		held[n] = false
	}
	s = reqStaging(t, held)
	r = healRequester{}
	tx2 := s.Begin()
	defer tx2.Discard()
	r.decide(tx2, reqStream, 0, 0, 4)
	spans := r.decide(tx2, reqStream, 0, 0, 4+healNoticeAge*healCadence)
	require.Len(t, spans, MaxRequestSpans)
	require.Equal(t, [2]uint64{1, 1}, spans[0])
	require.Equal(t, [2]uint64{31, 31}, spans[MaxRequestSpans-1])
}

// A source whose requests all failed is left alone for a doubling number of
// activations, up to a cap, and asked again as soon as it answers.
func TestOutcome_Backoff(t *testing.T) {
	var r healRequester
	r.outcome(reqSource, 100, 0, 1)
	require.True(t, r.backedOff(reqSource, 100))
	require.False(t, r.backedOff(reqSource, 100+healCadence))
	r.outcome(reqSource, 104, 0, 1)
	require.True(t, r.backedOff(reqSource, 104+healCadence))
	require.False(t, r.backedOff(reqSource, 104+2*healCadence))
	for i := 0; i < 10; i++ {
		r.outcome(reqSource, 200, 0, 1)
	}
	require.True(t, r.backedOff(reqSource, 200+(maxSourceBackoff-1)*healCadence))
	require.False(t, r.backedOff(reqSource, 200+maxSourceBackoff*healCadence), "capped")
	r.outcome(reqSource, 300, 1, 1)
	require.False(t, r.backedOff(reqSource, 300), "an answer clears the back-off")
}
