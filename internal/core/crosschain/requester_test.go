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

// A staging that holds the given numbers; collected ones are unvalidated
// unless listed in proven, each validated by a one-element proof at its own
// number.
func reqStaging(t *testing.T, held map[uint64]bool, proven ...uint64) *execute.Staging {
	s := execute.NewStaging()
	tx := s.Begin()
	for n, collected := range held {
		tx.Hold(reqStream, n, reqHeld(n, collected))
	}
	for _, n := range proven {
		require.NoError(t, tx.Prove(reqStream, reqProof(n, reqHeld(n, true).Hash)))
	}
	tx.Commit()
	return s
}

// reqProof is a one-element collection proof validating hash at number n.
func reqProof(n uint64, hash [32]byte) *merkle.ReceiptList {
	list := merkle.NewReceiptList()
	st := new(merkle.State)
	for i := uint64(0); i < n-1; i++ {
		var pad [32]byte
		pad[0] = byte(i + 100)
		st.AddEntry(pad[:])
	}
	list.MerkleState = st
	list.Elements = [][]byte{hash[:]}
	return list
}

// settle runs the activations a stream must sit still through before the
// requester asks its source for anything: healing is for a stream that has
// stopped, and one whose Delivered is still moving is delivering (#4280).
// Tests that are about WHAT is asked for use this to get to the decision.
func settle(r *healRequester, tx *execute.StagingTxn, delivered, block uint64) {
	for i := uint64(0); i < probeAfter-1; i++ {
		r.decide(tx, reqStream, delivered, block+i*healCadence)
	}
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

	settle(&r, tx, 0, 8)
	spans := r.decide(tx, reqStream, 0, 8+(probeAfter-1)*healCadence)
	require.Equal(t, [][2]uint64{{1, 2}}, spans, "the hole and the unvalidated entry, coalesced; 3 and 4 are runnable")

	base := uint64(8 + (probeAfter-1)*healCadence)
	r.asked(reqStream, spans[0], base)
	require.Empty(t, r.decide(tx, reqStream, 0, base+healCadence), "asked: patience")
	require.Empty(t, r.decide(tx, reqStream, 0, base+(healPatience-1)*healCadence))
	require.Equal(t, [][2]uint64{{1, 2}}, r.decide(tx, reqStream, 0, base+healPatience*healCadence), "patience over: asked again")

	// Delivered past everything held: nothing above Delivered is known, so
	// the span above it is asked for whole -- but only once the stream has
	// been empty AND still for probeAfter activations (#4280). The first
	// sightings are silent.
	for i := uint64(0); i < probeAfter-1; i++ {
		require.Empty(t, r.decide(tx, reqStream, 4, base+(healPatience+1+i)*healCadence),
			"Delivered only just arrived there: delivering, not stuck")
	}
	probe := r.decide(tx, reqStream, 4, base+(healPatience+probeAfter)*healCadence)
	require.Equal(t, [][2]uint64{{5, 4 + protocol.MaxReceiptListElements}}, probe, "a stuck empty stream asks for the span above Delivered")
	r.asked(reqStream, [2]uint64{5, 5}, base+(healPatience+probeAfter)*healCadence)
	require.Empty(t, r.decide(tx, reqStream, 4, base+(healPatience+probeAfter+1)*healCadence), "asked: patience")
	require.NotEmpty(t, r.decide(tx, reqStream, 4, base+(2*healPatience+probeAfter+1)*healCadence), "patience over: asked again")
}

// A stream that is merely draining never probes. Delivered moves on every
// activation, which is what "the source is delivering" looks like, and the
// catch-up probe is for the opposite case -- a package lost whole, which
// stops Delivered dead.
//
// Run 20260915T211229Z probed on sight: with no faults induced and nothing
// dropped, healing pulled 743,000 entries against 23,000 requests, 56% of
// all traffic those streams had ever carried, to cover ~1% in flight and
// arriving anyway (#4280).
func TestDecide_ADrainingStreamNeverProbes(t *testing.T) {
	s := reqStaging(t, map[uint64]bool{})
	var r healRequester
	tx := s.Begin()
	defer tx.Discard()

	// Twenty activations, Delivered advancing each time: nothing is asked.
	for i := uint64(0); i < 20; i++ {
		require.Empty(t, r.decide(tx, reqStream, 100+i, 8+i*healCadence),
			"activation %d: Delivered is moving, so the stream is draining", i)
	}

	// It stops. Now the probe is due, and fires once it has been still
	// for probeAfter activations.
	stuck := uint64(120)
	for i := uint64(0); i < probeAfter-1; i++ {
		require.Empty(t, r.decide(tx, reqStream, stuck, 200+i*healCadence), "still settling")
	}
	require.Equal(t, [][2]uint64{{stuck + 1, stuck + protocol.MaxReceiptListElements}},
		r.decide(tx, reqStream, stuck, 200+(probeAfter-1)*healCadence),
		"Delivered has stopped: a lost package looks like this")
}

// A stream whose Delivered is moving is never asked about, holes above it or
// not: delivery is in order, so nothing below Delivered is missing and a hole
// above it either fills before delivery reaches it or stops Delivered when it
// does. Asking on sight healed every transient reorder (#4280).
func TestDecide_AMovingStreamIsNeverAskedAbout(t *testing.T) {
	// A hole at 1 and 2, entries held at 3 and 4: plenty to ask for.
	s := reqStaging(t, map[uint64]bool{3: true, 4: true})
	var r healRequester
	tx := s.Begin()
	defer tx.Discard()

	for i := uint64(0); i < 20; i++ {
		require.Empty(t, r.decide(tx, reqStream, 100+i, 8+i*healCadence),
			"activation %d: Delivered is moving, so the stream is delivering", i)
	}

	// It stops, below the held entries, and the gaps are asked for.
	settle(&r, tx, 0, 200)
	require.NotEmpty(t, r.decide(tx, reqStream, 0, 200+(probeAfter-1)*healCadence),
		"Delivered has stopped: now the holes matter")
}

// A validated entry whose proof arrived with it is not a gap even when it
// cannot run yet because of a hole below it; the hole is.
func TestDecide_ValidatedEntryBehindAHole(t *testing.T) {
	s := reqStaging(t, map[uint64]bool{3: true}, 3)
	var r healRequester
	tx := s.Begin()
	defer tx.Discard()
	settle(&r, tx, 0, 4)
	require.Equal(t, [][2]uint64{{1, 2}}, r.decide(tx, reqStream, 0, 4+(probeAfter-1)*healCadence), "only the hole below it")
}

// A validated hash with no entry under it is a gap of entries, even above
// everything held: the walk runs to the validated reach.
func TestDecide_ValidatedBeyondHeld(t *testing.T) {
	s := reqStaging(t, map[uint64]bool{1: false, 2: false}, 5)
	var r healRequester
	tx := s.Begin()
	defer tx.Discard()
	settle(&r, tx, 0, 4)
	require.Equal(t, [][2]uint64{{3, 5}}, r.decide(tx, reqStream, 0, 4+(probeAfter-1)*healCadence), "3 and 4 are holes, 5 is validated but not held")
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
	settle(&r, tx, 0, 4)
	spans := r.decide(tx, reqStream, 0, 4+(probeAfter-1)*healCadence)
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

// The stillness gate is for SYNTHETIC streams. Every measurement behind it
// came from one: the 743,000-entry storm, the runs averaging 49 consecutive
// numbers, the 13.7-29.3s in-flight times (#4280). An anchor stream is a
// different shape -- roughly one entry per block per partition, executed
// under a quorum -- and gating it made a lost block validator anchor
// unrecoverable rather than merely slow: TestMissingBlockValidatorAnchorTxn
// went from 1 failure in 20 runs to 13, and stayed broken at a 600-block
// budget. An anchor stream is asked on sight.
func TestDecide_AnAnchorStreamIsNotGatedByStillness(t *testing.T) {
	anchorStream := execute.StreamID{
		Ledger: protocol.PartitionUrl("BVN0").JoinPath(protocol.AnchorPool),
		Source: reqSource,
	}
	s := execute.NewStaging()
	var r healRequester
	tx := s.Begin()
	defer tx.Discard()

	// The very first activation on a stalled anchor stream asks: there is no
	// settling period to wait out.
	require.Equal(t, [][2]uint64{{6, 5 + protocol.MaxReceiptListElements}},
		r.decide(tx, anchorStream, 5, 8),
		"an anchor stream is asked on sight, not after probeAfter activations")

	// A synthetic stream at the same standing is still gated, so the two
	// rules stay distinct.
	require.Empty(t, r.decide(tx, reqStream, 5, 8),
		"a synthetic stream still waits for its Delivered to sit still")
}
