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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// The 20260824T024122Z wedge left 63 holes scattered across ~20 separate runs
// (498-506, 514, 518, ...). One held anchor proves all of them — it postdates
// the newest hole — so a single scan must attempt every run, not one run per
// heal window (#4163).

func txid(b byte) *url.TxID { return protocol.PartitionUrl("BVN2").WithTxID([32]byte{b}) }

// gapFixture builds a conductor and the ledger entry for one inbound stream:
// delivered to `delivered`, with staging holding `hold`.
//
// The held set is STAGING's now, not the record's (#4189), so a fixture says
// what the node holds rather than laying out a pending array. That rules out a
// shape the old fixtures could write and the executor could never produce: a
// trailing hole above everything held. A number above the highest thing we hold
// is not known to exist — nothing sighted it — and finding it is reconcile's
// job, from what the SOURCE says it produced.
func gapFixture(t *testing.T, delivered uint64, hold ...uint64) (*Conductor, *database.Batch, *protocol.PartitionSyntheticLedger) {
	t.Helper()
	c := &Conductor{
		Partition: &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
	}
	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)

	source := protocol.PartitionUrl("BVN2")
	part := &protocol.PartitionSyntheticLedger{Url: source, Delivered: delivered}
	stageHeld(t, batch, c, source, hold...)
	return c, batch, part
}

// stageHeld and stageHeldAnchors put `hold` into a conductor's staging for the
// stream inbound from source. The held set lives there now, not in the ledger
// record (#4189), so a fixture seeds it here rather than laying out Pending.
func stageHeld(t *testing.T, batch *database.Batch, c *Conductor, source *url.URL, hold ...uint64) {
	t.Helper()
	for _, n := range hold {
		require.NoError(t, execute.Hold(batch, c.syntheticStream(source), n,
			source.WithTxID([32]byte{byte(n), byte(n >> 8)})))
	}
}

func stageHeldAnchors(t *testing.T, batch *database.Batch, c *Conductor, source *url.URL, hold ...uint64) {
	t.Helper()
	for _, n := range hold {
		require.NoError(t, execute.Hold(batch, c.anchorStream(source), n,
			source.WithTxID([32]byte{byte(n), byte(n >> 8)})))
	}
}

func TestMissingRuns_EnumeratesScatteredHolesOldestFirst(t *testing.T) {
	// Delivered=497; holding 507, 509, 510 and 513 — so 498-506, 508 and
	// 511-512 are holes, and 513 is what proves the last of them exists.
	c, batch, part := gapFixture(t, 497, 507, 509, 510, 513)
	assert.Equal(t, [][2]uint64{{498, 506}, {508, 508}, {511, 512}}, c.missingRuns(batch, part))
}

func TestMissingRuns_EmptyAndFullyKnownWindows(t *testing.T) {
	c, batch, part := gapFixture(t, 5)
	assert.Empty(t, c.missingRuns(batch, part), "nothing sighted above the watermark is nothing missing")

	c, batch, part = gapFixture(t, 5, 6, 7)
	assert.Empty(t, c.missingRuns(batch, part), "a contiguous held run has no holes")
}

// rangeRecorder records every SequenceRange attempt and refuses to serve, so
// the scan's loop structure is observable without proof machinery.
type rangeRecorder struct {
	ranges [][2]uint64
	anchor []uint64
}

func (s *rangeRecorder) Sequence(ctx context.Context, _, _ *url.URL, num uint64, _ private.SequenceOptions) (*api.MessageRecord[messaging.Message], error) {
	return nil, errors.NotFound.With("per-message path not under test")
}

func (s *rangeRecorder) SequenceRange(ctx context.Context, _, _ *url.URL, start, end uint64, opts private.SequenceOptions) ([]*api.MessageRecord[messaging.Message], error) {
	s.ranges = append(s.ranges, [2]uint64{start, end})
	s.anchor = append(s.anchor, opts.ProveAgainstAnchor)
	return nil, errors.NotReady.With("not serving; this test only records the attempts")
}

// One scan must attempt EVERY missing run, each under the SAME held anchor.
func TestRequestMissingSynthetics_PullsEveryRunUnderOneHeldAnchor(t *testing.T) {
	self := protocol.PartitionUrl("BVN1")
	source := protocol.PartitionUrl("BVN2")

	ledger := new(protocol.SyntheticLedger)
	ledger.Url = self.JoinPath(protocol.Synthetic)
	part := ledger.Partition(source)
	part.Delivered = 497

	anchors := new(protocol.AnchorLedger)
	anchors.Url = self.JoinPath(protocol.AnchorPool)
	anchors.Partition(source).Delivered = 1454

	db := database.OpenInMemory(nil)
	batch := db.Begin(true)
	t.Cleanup(batch.Discard)
	require.NoError(t, batch.Account(ledger.Url).Main().Put(ledger))
	require.NoError(t, batch.Account(anchors.Url).Main().Put(anchors))

	seq := new(rangeRecorder)
	c := &Conductor{
		Partition:           &protocol.PartitionInfo{ID: "BVN1", Type: protocol.PartitionTypeBlockValidator},
		Sequencer:           seq,
		SyntheticHealWindow: time.Nanosecond, // claim fires on the second scan
	}
	c.Globals.Store(&core.GlobalValues{ExecutorVersion: protocol.ExecutorVersionV2Kourou})

	// Holding 507, 509, 510 and 513 — so 498-506, 508 and 511-512 are holes.
	// 513 is what makes the last run visible at all: a number above everything
	// staged has not been sighted, and finding THAT is reconcile's job.
	stageHeld(t, batch, c, source, 507, 509, 510, 513)

	// First scan registers the jittered claim; second fires.
	require.NoError(t, c.requestMissingSynthetics(context.Background(), batch))
	time.Sleep(time.Millisecond)
	require.NoError(t, c.requestMissingSynthetics(context.Background(), batch))

	require.Equal(t, [][2]uint64{{498, 506}, {508, 508}, {511, 512}}, seq.ranges,
		"one scan attempts every missing run, oldest first")
	for i, a := range seq.anchor {
		assert.Equal(t, uint64(1454), a, "run %d must be proven against the same held anchor", i)
	}
}
