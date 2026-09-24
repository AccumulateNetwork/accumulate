// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// Run 20260924T111811Z: every BVN restart logged "The next block has a gap;
// advancing the sync block=158 synced=157" and nothing more, so no one could
// say which stream stopped the handoff or at which number (#4432). The line
// names every stream with a gap and its numbers, and the gauge carries the
// first missing number per stream for the harness until the gap is gone.
func TestJoin_TheGapLineAndGaugeNameTheStreamAndNumber(t *testing.T) {
	const b = 157
	const partition = "TestJoin_TheGapLineAndGaugeNameTheStreamAndNumber"
	stream := execute.StreamID{
		Ledger: protocol.PartitionUrl("BVN1").JoinPath(protocol.Synthetic),
		Source: protocol.PartitionUrl("BVN0"),
	}
	gap := StreamGap{Stream: stream, Delivered: 103, Missing: 104, MissingTo: 104, Held: 105}

	var out bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&out, nil))

	// The gauge is read at the moment the second check is asked for: the
	// first check's gap is still on it then, and the second clears it.
	var during float64
	var present bool
	stage := &fakeStage{gaps: map[uint64][]StreamGap{b + 1: {gap}}}
	buf := &gaugeReadingBuffer{read: func(block uint64) {
		if block == b+2 {
			during, present = testutil.ToFloat64(mGapMissing.WithLabelValues(partition, gap.StreamName())), true
		}
	}}
	state := &gapState{stage: stage, b: b}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: partition, Buffer: buf, Stage: stage, State: state, Peers: peers, Logger: logger})
	require.NoError(t, err)

	require.Contains(t, out.String(), "The next block has a gap; advancing the sync")
	require.Contains(t, out.String(), gap.String(), "the gap line names the stream, its Delivered, the missing run and what is held")

	require.True(t, present)
	require.Equal(t, 104.0, during, "the gauge is the first missing number of the gapped stream")
	require.Equal(t, 0, testutil.CollectAndCount(mGapMissing, "accumulate_join_gap_first_missing"),
		"the check that found no gap clears the partition's series")
}

// gaugeReadingBuffer calls read before each StageThrough: the moment between
// one gap check and the next.
type gaugeReadingBuffer struct {
	fakeBuffer
	read func(block uint64)
}

func (b *gaugeReadingBuffer) StageThrough(block uint64) error {
	b.read(block)
	return b.fakeBuffer.StageThrough(block)
}
