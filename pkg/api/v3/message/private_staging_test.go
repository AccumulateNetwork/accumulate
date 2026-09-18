// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package message_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	. "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/message"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// stagingServer is a sequencer that serves canned staging pages, and records
// what it was asked for.
type stagingServer struct {
	private.Sequencer
	pages []*private.StagingSnapshot
	asked []*private.StagingSnapshotRequest
}

func (s *stagingServer) StagingSnapshot(_ context.Context, req *private.StagingSnapshotRequest) (*private.StagingSnapshot, error) {
	s.asked = append(s.asked, req)
	if len(s.asked) > len(s.pages) {
		return nil, errors.NotReady.With("no more pages")
	}
	return s.pages[len(s.asked)-1], nil
}

func stagedPage(block uint64, numbers ...uint64) *private.StagingSnapshot {
	src := protocol.PartitionUrl("BVN1")
	str := &private.StagedStream{
		Ledger:    protocol.PartitionUrl("BVN0").JoinPath(protocol.Synthetic),
		Source:    src,
		Delivered: 4,
		Sighted:   40,
	}
	for _, n := range numbers {
		seq := &messaging.SequencedMessage{Number: n, Source: src, Destination: protocol.PartitionUrl("BVN0")}
		str.Entries = append(str.Entries, &private.StagedEntry{
			Number:    n,
			Message:   seq,
			Collected: true,
			Hash:      seq.Hash(),
		})
		str.Validated = append(str.Validated, &private.StagedHash{Number: n, Hash: seq.Hash()})
	}
	str.Proofs = append(str.Proofs, &private.StagedProof{
		AnchorBlock: 17,
		Proof:       &protocol.AnnotatedReceipt{Anchor: &protocol.AnchorMetadata{SourceBlock: 17}},
	})
	return &private.StagingSnapshot{Block: block, Streams: []*private.StagedStream{str}}
}

// A page of staging survives the wire whole: the block it is as of, what the
// stream holds, the hashes proofs have validated, and the proofs waiting for
// their anchor.
func TestPrivateStagingSnapshot(t *testing.T) {
	expect := stagedPage(99, 5, 6, 7)
	s := &stagingServer{pages: []*private.StagingSnapshot{expect}}
	c := SetupTest(t, &Sequencer{Sequencer: s})

	client := c.ForAddress(nil).Private().(private.StagingSnapshotter)
	actual, err := client.StagingSnapshot(context.Background(), &private.StagingSnapshotRequest{Partition: "BVN0", Number: 5, Limit: 8})
	require.NoError(t, err)
	require.True(t, expect.Equal(actual), "the snapshot came back as it was sent")
	require.Len(t, actual.Streams, 1)
	require.Len(t, actual.Streams[0].Entries, 3)
	require.Len(t, actual.Streams[0].Validated, 3)
	require.Len(t, actual.Streams[0].Proofs, 1)
	require.IsType(t, new(messaging.SequencedMessage), actual.Streams[0].Entries[0].Message)
	require.Equal(t, uint64(4), actual.Streams[0].Delivered)

	require.Len(t, s.asked, 1)
	require.Equal(t, "BVN0", s.asked[0].Partition)
	require.Equal(t, uint64(5), s.asked[0].Number)
	require.Equal(t, uint64(8), s.asked[0].Limit)
}

// A sequencer that does not serve staging says so rather than failing
// obscurely.
func TestPrivateStagingSnapshotUnsupported(t *testing.T) {
	c := SetupTest(t, &Sequencer{Sequencer: plainSequencer{}})
	client := c.ForAddress(nil).Private().(private.StagingSnapshotter)
	_, err := client.StagingSnapshot(context.Background(), &private.StagingSnapshotRequest{Partition: "BVN0"})
	require.Error(t, err)
	require.ErrorIs(t, err, errors.NotAllowed)
}

type plainSequencer struct{}

func (plainSequencer) Sequence(context.Context, *url.URL, *url.URL, uint64, private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	return nil, errors.NotFound
}

// Pages are assembled only while they are as of one block. The peer commits
// a block between the first page and the second, and the reader refuses what
// it has rather than executing a state no node ever held.
func TestPrivateStagingSnapshotPagingRefusesABlockChange(t *testing.T) {
	first := stagedPage(99, 5, 6)
	first.NextLedger = first.Streams[0].Ledger
	first.NextSource = first.Streams[0].Source
	first.NextNumber = 7
	second := stagedPage(100, 7, 8)

	s := &stagingServer{pages: []*private.StagingSnapshot{first, second}}
	c := SetupTest(t, &Sequencer{Sequencer: s})
	client := c.ForAddress(nil).Private().(private.StagingSnapshotter)

	_, err := private.FetchStagingSnapshot(context.Background(), client, "BVN0")
	require.Error(t, err)
	require.ErrorIs(t, err, errors.Conflict)

	// The same two pages as of the same block assemble.
	second.Block = 99
	s.asked = nil
	whole, err := private.FetchStagingSnapshot(context.Background(), client, "BVN0")
	require.NoError(t, err)
	require.Equal(t, uint64(99), whole.Block)
	require.Len(t, whole.Streams, 2)
	require.Len(t, s.asked, 2)
	require.Equal(t, uint64(7), s.asked[1].Number, "the second page continues where the first stopped")
}
