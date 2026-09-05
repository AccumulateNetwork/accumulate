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

// Two things are gaps and nothing else is: an index not held below a held
// entry, and a held entry no proof has validated. Both are asked for when seen
// and not again within patience; a held, validated entry is never asked.
func TestDecide_TwoKindsOfGap(t *testing.T) {
	// 1 missing, 2 collected+unvalidated, 3 collected+validated, 4 own-proof held
	s := reqStaging(t, map[uint64]bool{2: true, 3: true, 4: false}, 3)
	var r healRequester
	tx := s.Begin()
	defer tx.Discard()

	spans := r.decide(tx, reqStream, 0, 8)
	require.Equal(t, [][2]uint64{{1, 2}}, spans, "the hole and the unvalidated entry, coalesced; 3 and 4 are runnable")

	r.asked(reqStream, spans[0], 8)
	require.Empty(t, r.decide(tx, reqStream, 0, 8+healCadence), "asked: patience")
	require.Empty(t, r.decide(tx, reqStream, 0, 8+(healPatience-1)*healCadence))
	require.Equal(t, [][2]uint64{{1, 2}}, r.decide(tx, reqStream, 0, 8+healPatience*healCadence), "patience over: asked again")

	// Delivered past everything held: nothing above Delivered is known, so
	// the span above it is asked for whole, once per patience.
	probe := r.decide(tx, reqStream, 4, 8+(healPatience+1)*healCadence)
	require.Equal(t, [][2]uint64{{5, 4 + protocol.MaxReceiptListElements}}, probe, "an empty stream asks for the span above Delivered")
	r.asked(reqStream, [2]uint64{5, 5}, 8+(healPatience+1)*healCadence)
	require.Empty(t, r.decide(tx, reqStream, 4, 8+(healPatience+2)*healCadence), "asked: patience")
	require.NotEmpty(t, r.decide(tx, reqStream, 4, 8+(2*healPatience+2)*healCadence), "patience over: asked again")
}

// A validated entry whose proof arrived with it is not a gap even when it
// cannot run yet because of a hole below it; the hole is.
func TestDecide_ValidatedEntryBehindAHole(t *testing.T) {
	s := reqStaging(t, map[uint64]bool{3: true}, 3)
	var r healRequester
	tx := s.Begin()
	defer tx.Discard()
	require.Equal(t, [][2]uint64{{1, 2}}, r.decide(tx, reqStream, 0, 4), "only the hole below it")
}

// Separate holes become separate spans, oldest first, capped.
func TestDecide_Spans(t *testing.T) {
	held := map[uint64]bool{}
	for n := uint64(2); n <= 100; n += 2 {
		held[n] = false
	}
	s := reqStaging(t, held)
	var r healRequester
	tx := s.Begin()
	defer tx.Discard()
	spans := r.decide(tx, reqStream, 0, 4)
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
