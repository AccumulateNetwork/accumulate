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
	actual, err := client.StagingSnapshot(context.Background(), &private.StagingSnapshotRequest{
		Partition:   "BVN0",
		Source:      protocol.PartitionUrl("BVN1"),
		Number:      5,
		ProofOffset: 2,
		Limit:       8,
	})
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
	require.Equal(t, uint64(2), s.asked[0].ProofOffset, "the proof cursor survives the wire too")
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

// Pages are assembled only while they are as of one block. The peer commits a
// block between the first page and the second, so the reader discards what it
// has and starts over rather than executing a state no node ever held — and
// it starts over a bounded number of times, because the peer commits a block
// every second or so and nothing pins a version of its stage for a reader
// (#4291 review).
func TestPrivateStagingSnapshotRestartsOnABlockChange(t *testing.T) {
	first := stagedPage(99, 5, 6)
	first.More = true
	first.NextLedger = first.Streams[0].Ledger
	first.NextSource = first.Streams[0].Source
	first.NextNumber = 7
	second := stagedPage(100, 7, 8)

	// A peer whose block moves on every second page, forever.
	s := &movingServer{first: first, second: second}
	_, err := private.FetchStagingSnapshot(context.Background(), s, "BVN0")
	require.Error(t, err)
	require.ErrorIs(t, err, errors.NotReady, "the reader gives up on this peer")
	require.Equal(t, 2*(private.MaxSnapshotRestarts+1), s.calls, "after a bounded number of attempts")

	// The same two pages as of the same block assemble, over the wire.
	second.Block = 99
	canned := &stagingServer{pages: []*private.StagingSnapshot{first, second}}
	c := SetupTest(t, &Sequencer{Sequencer: canned})
	client := c.ForAddress(nil).Private().(private.StagingSnapshotter)

	whole, err := private.FetchStagingSnapshot(context.Background(), client, "BVN0")
	require.NoError(t, err)
	require.Equal(t, uint64(99), whole.Block)
	require.Len(t, whole.Streams, 2)
	require.Len(t, canned.asked, 2)
	require.Equal(t, uint64(7), canned.asked[1].Number, "the second page continues where the first stopped")
}

// movingServer answers the first page, then a page as of a later block, over
// and over: a peer that commits faster than this reader can read it.
type movingServer struct {
	private.Sequencer
	first, second *private.StagingSnapshot
	calls         int
}

func (s *movingServer) StagingSnapshot(context.Context, *private.StagingSnapshotRequest) (*private.StagingSnapshot, error) {
	s.calls++
	if s.calls%2 == 1 {
		return s.first, nil
	}
	return s.second, nil
}

// A cursor is a position and every position has a source, so a page that says
// there is more but does not say where is a peer this reader cannot follow.
// It is the mirror of the nil-ledger cursor the server sends for a source
// that holds proofs and no stream (#4291 review).
func TestPrivateStagingSnapshotCursorWithoutASource(t *testing.T) {
	page := stagedPage(99, 5, 6)
	page.More = true
	page.NextLedger = page.Streams[0].Ledger // a ledger, and no source

	s := &stagingServer{pages: []*private.StagingSnapshot{page}}
	_, err := private.FetchStagingSnapshot(context.Background(), s, "BVN0")
	require.Error(t, err)
	require.ErrorIs(t, err, errors.PeerMisbehaved)
	require.Len(t, s.asked, 1, "and the reader stopped asking")
}

// A peer whose cursor crawls forward one number at a time never finishes, and
// the reader must not follow it forever (#4291 review).
func TestPrivateStagingSnapshotBoundsPages(t *testing.T) {
	s := new(crawlingServer)
	_, err := private.FetchStagingSnapshot(context.Background(), s, "BVN0")
	require.Error(t, err)
	require.ErrorIs(t, err, errors.NotReady)
	require.Equal(t, private.MaxSnapshotPages, s.calls)
}

// crawlingServer always has more, one number further on.
type crawlingServer struct {
	private.Sequencer
	calls int
}

func (s *crawlingServer) StagingSnapshot(_ context.Context, req *private.StagingSnapshotRequest) (*private.StagingSnapshot, error) {
	s.calls++
	page := stagedPage(99)
	page.More = true
	page.NextLedger = page.Streams[0].Ledger
	page.NextSource = page.Streams[0].Source
	page.NextNumber = req.Number + 1
	return page, nil
}
