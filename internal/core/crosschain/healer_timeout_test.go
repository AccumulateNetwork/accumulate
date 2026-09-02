// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// stuckSequencer models the failure that wedged run 20260824T024122Z: a peer
// that accepts the request and never answers. The p2p transport unblocks a
// read only when the caller's context is canceled (p2p.go closes the stream
// on ctx.Done), so a sequencer call must hang exactly as long as its context
// allows and no longer.
type stuckSequencer struct {
	called chan [2]uint64
}

func (s *stuckSequencer) Sequence(ctx context.Context, _, _ *url.URL, num uint64, _ private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	// The per-message path bounds itself (requestSyntheticFrom wraps a
	// window timeout); it is not the path under test.
	return nil, errors.NotFound.With("per-message path not under test")
}

// SequenceRange blocks until the caller's context is canceled — the exact
// behaviour of the p2p transport against a peer that accepted the stream and
// never answers. recoverSyntheticsViaRange has no timeout of its own, so the
// caller's context is the ONLY thing that can unblock this.
func (s *stuckSequencer) SequenceRange(ctx context.Context, _, _ *url.URL, start, end uint64, _ private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	select {
	case s.called <- [2]uint64{start, end}:
	default:
	}
	<-ctx.Done()
	return nil, errors.UnknownError.Wrap(ctx.Err())
}

// requestMissingSynthetics must return when its context expires, even when
// every pull hangs on a peer that never answers. Before the fix it was
// invoked with context.Background(): one dead peer parked the pull forever,
// the runExclusive slot was never released, and the only healer that can see
// interior sequence holes was dead on that validator from then on — observed
// live as BVN2→DN wedged at delivered=497 with holes 498+ never requested
// while the reconcile loop thrashed the tail (#4159 follow-on).
func TestRequestMissingSynthetics_ReturnsWhenAPeerNeverAnswers(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.PartitionUrl("BVN2")

	// A stream with interior holes: delivered 497, holding 501 — so 498-500
	// are holes, the exact shape that must trigger pulls.
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = self.JoinPath(protocol.Synthetic)
	part := ledger.Partition(source)
	part.Delivered = 497

	// An anchor ledger with a delivered anchor from the source, so the range
	// path (the one with no timeout of its own) is the one that fires.
	anchors := new(protocol.AnchorLedger)
	anchors.Url = self.JoinPath(protocol.AnchorPool)
	anchors.Partition(source).Delivered = 1454

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))
	require.NoError(t, batch.Account(anchors.Url).Main().Put(anchors))

	seq := &stuckSequencer{called: make(chan [2]uint64, 1)}
	c := &Conductor{
		Partition: &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
		Sequencer: seq,
		// Nanosecond window: the claim's jittered fire time lands in the
		// past immediately, so the second scan fires instead of waiting.
		SyntheticHealWindow: time.Nanosecond,
	}
	c.Globals.Store(&core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Kourou})
	stageHeld(c, source, 501)

	// First scan only registers the claim (jittered check-then-fire) and
	// returns without pulling.
	require.NoError(t, c.requestMissingSynthetics(context.Background(), batch))
	time.Sleep(time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- c.requestMissingSynthetics(ctx, batch) }()

	select {
	case <-done:
		// Returned once the context expired — the exclusive slot would be
		// released and the healer lives to scan again next block.
	case <-time.After(5 * time.Second):
		t.Fatal("requestMissingSynthetics is still blocked long after its context expired — a dead peer wedges the healer permanently")
	}

	// The return must have come from the context unblocking a pull that was
	// genuinely stuck — not from the scan skipping the stream entirely, which
	// would make this test pass without exercising anything.
	select {
	case r := <-seq.called:
		require.Equal(t, [2]uint64{498, 500}, r, "the range pull covers the first run of holes")
	default:
		t.Fatal("the sequencer was never called — the scan never attempted the hole this test exists to pull")
	}
	require.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond,
		"returned before the context expired — the pull cannot have been waited on")
}
