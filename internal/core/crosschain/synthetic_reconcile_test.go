// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package crosschain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/network"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A source that cannot prove a sequence cannot prove any later one either --
// sequence N+1 was produced after N, so if N's block is not anchored, N+1's
// certainly is not. The reconcile used to ask anyway, once per sequence in the
// gap, and log an ERROR for each.
//
// That is not hypothetical tidiness. reconcileGraceBlocks is a fixed block count
// meant to keep the reconcile behind anchoring, but anchoring latency is not
// bounded: on a 10 tps soak a partition fell ~11,800 blocks behind its own
// height, and the reconcile produced 1,492 identical failures in ten minutes --
// 2.5 per second, burying every other log line on the node (#4086).

// stubSequencer fails every pull with the supplied error, recording which
// sequences were asked for. Distinct sequences are what matters, not call
// count: requestSyntheticFrom retries the same sequence three times internally
// before giving up, which is a transport concern and not the scan behaviour
// under test.
type stubSequencer struct {
	err  error
	seen map[uint64]bool
}

func (s *stubSequencer) Sequence(ctx context.Context, src, dst *url.URL, num uint64, opts private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	if s.seen == nil {
		s.seen = map[uint64]bool{}
	}
	s.seen[num] = true
	return nil, s.err
}

func (s *stubSequencer) distinct() int { return len(s.seen) }

// stubQuerier answers the "what have you produced for me" query with a ledger
// claiming `produced` messages for the destination.
type stubQuerier struct {
	dest     *url.URL
	produced uint64
}

func (q *stubQuerier) Query(ctx context.Context, scope *url.URL, _ api.Query) (api.Record, error) {
	ledger := new(protocol.SyntheticLedger)
	ledger.Url = scope
	ledger.Sequence = []*protocol.PartitionSyntheticLedger{{
		Url:      q.dest,
		Produced: q.produced,
	}}
	return &api.AccountRecord{Account: ledger}, nil
}

// newReconcileTest builds a conductor for BVN1 that has received `received`
// messages from BVN2, while BVN2 claims to have produced `produced`.
func newReconcileTest(t *testing.T, received, produced uint64, pullErr error) (*Conductor, *stubSequencer) {
	t.Helper()

	const self, peer = "BVN1", "BVN2"
	selfUrl := protocol.PartitionUrl(self)
	peerUrl := protocol.PartitionUrl(peer)

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	defer batch.Discard()

	ledger := new(protocol.SyntheticLedger)
	ledger.Url = selfUrl.JoinPath(protocol.Synthetic)
	ledger.Sequence = []*protocol.PartitionSyntheticLedger{{
		Url:      peerUrl,
		Received: received,
	}}
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))
	require.NoError(t, batch.Commit())

	seq := &stubSequencer{err: pullErr}
	c := &Conductor{
		Partition: &protocol.PartitionInfo{ID: self, Type: protocol.PartitionTypeBlockValidator},
		Database:  db,
		Sequencer: seq,
		Querier:   api.Querier2{Querier: &stubQuerier{dest: selfUrl, produced: produced}},
	}
	g := new(network.GlobalValues)
	g.Network = &protocol.NetworkDefinition{Partitions: []*protocol.PartitionInfo{
		{ID: self, Type: protocol.PartitionTypeBlockValidator},
		{ID: peer, Type: protocol.PartitionTypeBlockValidator},
	}}
	c.Globals.Store(g)
	return c, seq
}

// runReconcile runs the reconcile twice: once to record the gap, then past the
// grace period so the gap counts as overdue and pulls are attempted.
func runReconcile(t *testing.T, c *Conductor) {
	t.Helper()
	ctx := context.Background()
	for _, block := range []uint64{1, 1 + reconcileGraceBlocks + 1} {
		batch := c.Database.Begin(false)
		require.NoError(t, c.reconcileInboundStreams(ctx, batch, block))
		batch.Discard()
	}
}

func TestReconcileStopsWhenSourceCannotProve(t *testing.T) {
	// A 50-message gap, every pull rejected as notFound — the shape of "the
	// producing block is not anchored yet".
	c, seq := newReconcileTest(t, 10, 60, errors.NotFound.With("reached the end of the chain"))
	runReconcile(t, c)

	require.Equal(t, 1, seq.distinct(),
		"reconcile asked for %d distinct sequences the source cannot prove; it "+
			"must stop at the first, because sequences are ordered and every later "+
			"one is even less likely to be anchored", seq.distinct())
}

func TestReconcileKeepsScanningOnOtherErrors(t *testing.T) {
	// The stop is specific to "cannot prove yet". A transport error on one
	// sequence says nothing about the next, so the scan must continue —
	// otherwise one flaky peer response stalls an entire window's recovery.
	// A small gap keeps the test quick: each sequence costs three internal
	// retries at 250ms.
	c, seq := newReconcileTest(t, 10, 13, errors.InternalError.With("connection reset"))
	runReconcile(t, c)

	require.Equal(t, 3, seq.distinct(),
		"a non-notFound failure must not end the scan: it carries no information "+
			"about later sequences")
}
