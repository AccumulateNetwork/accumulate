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
// say which stream it was or at which number (#4432). The line names every
// stream with a gap and its numbers, and the gauge carries the first missing
// number per stream. A gap no longer holds the handoff (executor spec, "Sync",
// "Two mismatches"); it is still said.
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

	stage := &fakeStage{gaps: map[uint64][]StreamGap{b + 1: {gap}}}
	buf := new(fakeBuffer)
	state := &gapState{stage: stage, b: b}
	peers := &fakePeers{peers: []*api.FindServiceResult{peerResult(1)}}

	_, err := run(t, Options{Partition: partition, Buffer: buf, Stage: stage, State: state, Peers: peers, Logger: logger})
	require.NoError(t, err)

	require.Contains(t, out.String(), "The next block has a gap; executing anyway")
	require.Contains(t, out.String(), gap.String(), "the gap line names the stream, its Delivered, the missing run and what is held")
	require.Equal(t, 104.0, testutil.ToFloat64(mGapMissing.WithLabelValues(partition, gap.StreamName())),
		"the gauge is the first missing number of the gapped stream")
}
